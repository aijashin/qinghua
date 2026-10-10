package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"zhiwei/backend/internal/ai"
	"zhiwei/backend/internal/auth"
	"zhiwei/backend/internal/jobs"
	"zhiwei/backend/internal/knowledge"
	"zhiwei/backend/internal/observability"
	"zhiwei/backend/internal/store"
)

func defaultRubric() store.Entity {
	return builtinRubric()
}
func (a *App) rubric(ctx context.Context) store.Entity {
	all, _ := a.Store.List(ctx, "rubrics")
	var best, fallback store.Entity
	for _, v := range all {
		if v.String("status") != "published" || demoRecord(v) {
			continue
		}
		if v["is_builtin"] == true {
			fallback = v
			continue
		}
		stamp := v.String("published_at")
		if stamp == "" {
			stamp = v.String("created_at")
		}
		prior := ""
		if best != nil {
			prior = best.String("published_at")
			if prior == "" {
				prior = best.String("created_at")
			}
		}
		stampTime, _ := time.Parse(time.RFC3339Nano, stamp)
		priorTime, _ := time.Parse(time.RFC3339Nano, prior)
		if best == nil || stampTime.After(priorTime) {
			best = v
		}
	}
	if best != nil {
		return best
	}
	return fallback
}
func scoreData(data map[string]any, rubric map[string]any) error {
	criteria, ok := data["criteria"].([]any)
	if !ok || len(criteria) == 0 {
		return errors.New("评分稿缺少评分项目")
	}
	rules, _ := rubric["criteria"].([]any)
	if len(rules) == 0 {
		rules = defaultRubric()["criteria"].([]any)
	}
	allowed := map[string]map[string]any{}
	for _, x := range rules {
		r, ok := x.(map[string]any)
		if ok {
			allowed[str(r, "id")] = r
		}
	}
	if len(criteria) != len(allowed) {
		return errors.New("评分项目必须与本报告规则一致")
	}
	total, max := 0.0, 0.0
	observed := 0
	seen := map[string]bool{}
	for _, x := range criteria {
		c, ok := x.(map[string]any)
		if !ok || strings.TrimSpace(str(c, "evidence")) == "" {
			return errors.New("每项评分需要观察依据")
		}
		id := str(c, "id")
		rule, ok := allowed[id]
		if !ok || seen[id] {
			return errors.New("评分项目ID不正确")
		}
		seen[id] = true
		score, ok := c["score"].(float64)
		ceiling := num(rule, "max_score")
		if c["score"] != nil && (!ok || score < 0 || score > ceiling) {
			return errors.New("分值超出评分规则范围")
		}
		c["max_score"] = ceiling
		c["label"] = rule["label"]
		if _, exists := c["rule_text"]; !exists {
			if _, description := c["description"]; !description {
				c["rule_text"] = effectiveRuleText(rule, nil)
			}
		}
		if ok {
			total += score
			observed++
		}
		max += ceiling
	}
	data["total_score"] = nil
	if observed == len(criteria) {
		data["total_score"] = total
	}
	data["coverage"] = float64(observed) / float64(len(criteria))
	data["max_score"] = max
	data["rubric_id"] = str(rubric, "id")
	return nil
}
func (a *App) createJob(w http.ResponseWriter, r *http.Request) {
	if !a.AI.Configured() {
		fail(w, 503, "ai_not_configured", "尚未配置AI模型，请联系管理员")
		return
	}
	v, ok := body(w, r)
	if !ok {
		return
	}
	kind := v.String("kind")
	if kind != "assessment" && kind != "understanding" && kind != "coaching" && kind != "identify" {
		fail(w, 400, "invalid_kind", "未知分析类型")
		return
	}
	sample, e := a.Store.Get(r.Context(), "samples", v.String("sample_id"))
	if e != nil || sample.String("owner_id") != user(r).ID {
		fail(w, 404, "not_found", "样本不存在")
		return
	}
	if sessionID := v.String("learning_session_id"); sessionID != "" {
		s, e := a.Store.Get(r.Context(), "learning-sessions", sessionID)
		if e != nil || s.String("owner_id") != user(r).ID || s.String("sample_id") != sample.String("id") {
			fail(w, 400, "invalid_session", "学习轮次与样本不匹配")
			return
		}
		snapshot, ok := s["sample_snapshot"].(map[string]any)
		if !ok {
			fail(w, 409, "invalid_snapshot", "学习轮次缺少样本快照")
			return
		}
		sample = store.Entity(snapshot)
	}
	key := v.String("idempotency_key")
	if len(key) < 8 || len(key) > 128 {
		fail(w, 400, "idempotency_required", "请提供8至128字符幂等键")
		return
	}
	input, ok := v["input"].(map[string]any)
	if !ok {
		input = map[string]any{}
	}
	input["sample_snapshot"] = sample
	if demoRecord(sample) {
		fail(w, 409, "legacy_demo", "历史示例仅可回顾，请上传真实照片开始新的学习轮次")
		return
	}
	rubric := a.rubric(r.Context())
	if kind == "assessment" && rubric == nil {
		fail(w, 409, "rubric_required", "请管理员先创建并发布正式教学评分规则，再开始评图")
		return
	}
	input["rubric"] = rubric
	if kind == "coaching" {
		history, goal, err := a.coachingContext(r.Context(), user(r).ID, sample, v.String("learning_session_id"))
		if err != nil {
			dbError(w, err)
			return
		}
		input["teaching_history"] = history
		input["practice_goal"] = goal
	}

	b, _ := json.Marshal(input)
	j, e := a.Jobs.Create(r.Context(), jobs.Job{OwnerID: user(r).ID, ClassID: sample.String("class_id"), Kind: kind, SampleID: sample.String("id"), LearningSessionID: v.String("learning_session_id"), Input: b}, key)
	if e != nil {
		if errors.Is(e, jobs.ErrIdempotencyConflict) {
			fail(w, 409, "idempotency_conflict", "该幂等键已用于不同请求")
			return
		}
		dbError(w, e)
		return
	}
	respond(w, 202, j)
}
func (a *App) jobAllowed(ctx context.Context, u auth.User, j jobs.Job) bool {
	return a.canRead(ctx, u, store.Entity{"owner_id": j.OwnerID, "class_id": j.ClassID, "learning_session_id": j.LearningSessionID})
}
func (a *App) loadJob(w http.ResponseWriter, r *http.Request) (jobs.Job, bool) {
	j, e := a.Jobs.Get(r.Context(), r.PathValue("id"))
	if e != nil || !a.jobAllowed(r.Context(), user(r), j) {
		fail(w, 404, "not_found", "任务不存在")
		return j, false
	}
	return j, true
}
func (a *App) getJob(w http.ResponseWriter, r *http.Request) {
	j, ok := a.loadJob(w, r)
	if ok {
		respond(w, 200, j)
	}
}
func (a *App) listJobs(w http.ResponseWriter, r *http.Request) {
	all, e := a.Jobs.List(r.Context())
	if e != nil {
		dbError(w, e)
		return
	}
	out := []jobs.Job{}
	for _, j := range all {
		if a.jobAllowed(r.Context(), user(r), j) {
			out = append(out, j)
		}
	}
	total := len(out)
	limit := 100
	offset := 0
	if x, e := strconv.Atoi(r.URL.Query().Get("limit")); e == nil && x > 0 && x <= 500 {
		limit = x
	}
	if x, e := strconv.Atoi(r.URL.Query().Get("offset")); e == nil && x > 0 {
		offset = x
	}
	if offset > len(out) {
		offset = len(out)
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	respond(w, 200, map[string]any{"items": out[offset:end], "total": total, "limit": limit, "offset": offset})
}
func (a *App) cancelJob(w http.ResponseWriter, r *http.Request) {
	j, ok := a.loadJob(w, r)
	if !ok {
		return
	}
	if j.OwnerID != user(r).ID && user(r).Role != "admin" {
		fail(w, 403, "forbidden", "无权取消他人任务")
		return
	}
	if e := a.Jobs.Cancel(r.Context(), j.ID); e != nil {
		dbError(w, e)
		return
	}
	j, _ = a.Jobs.Get(r.Context(), j.ID)
	respond(w, 200, j)
}
func (a *App) jobEvents(w http.ResponseWriter, r *http.Request) {
	j, ok := a.loadJob(w, r)
	if !ok {
		return
	}
	cursor := r.Header.Get("Last-Event-ID")
	if cursor == "" {
		cursor = r.URL.Query().Get("after")
	}
	after := 0
	if cursor != "" {
		v, e := strconv.Atoi(cursor)
		if e != nil || v < 0 {
			fail(w, 400, "invalid_cursor", "事件游标无效")
			return
		}
		after = v
	}
	f, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, "stream_unavailable", "流暂不可用")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, ": connected\n\n")
	f.Flush()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		cookie, err := r.Cookie("zhiwei_session")
		if err != nil {
			return
		}
		currentUser, _, err := a.Auth.Session(r.Context(), cookie.Value)
		if err != nil || !a.jobAllowed(r.Context(), currentUser, j) {
			return
		}
		events, e := a.Jobs.Events(r.Context(), j.ID, after)
		if e != nil {
			return
		}
		for _, ev := range events {
			if _, e = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Event, ev.Data); e != nil {
				return
			}
			after = ev.Seq
		}
		if len(events) > 0 {
			f.Flush()
		}
		fresh, e := a.Jobs.Get(r.Context(), j.ID)
		if e != nil {
			return
		}
		if jobs.Terminal(fresh.Status) && len(events) == 0 {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		case <-heartbeat.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			f.Flush()
		}
	}
}
func (a *App) runJob(ctx context.Context, j jobs.Job, emit func(string) error) (jobs.Result, error) {
	var input map[string]any
	if e := json.Unmarshal(j.Input, &input); e != nil {
		return jobs.Result{}, e
	}
	var disabled bool
	var ownerRole string
	if e := a.Store.DB.QueryRowContext(ctx, `SELECT disabled,role FROM users WHERE id=?`, j.OwnerID).Scan(&disabled, &ownerRole); e != nil || disabled {
		return jobs.Result{}, errors.New("账号不可用，任务已停止")
	}
	if j.Kind == "document_import" {
		return a.runImport(ctx, j, input, emit)
	}
	snapshot, ok := input["sample_snapshot"].(map[string]any)
	if !ok {
		return jobs.Result{}, errors.New("样本快照不存在")
	}
	if demoRecord(snapshot) {
		return jobs.Result{}, observability.Coded("legacy_demo", errors.New("历史示例已停用，请上传真实照片重新分析"))
	}
	if j.Kind == "assessment" {
		rubric, _ := input["rubric"].(map[string]any)
		if rubric == nil || demoRecord(rubric) {
			return jobs.Result{}, observability.Coded("rubric_required", errors.New("本任务缺少正式教学规则，请管理员发布规则后重新发起评图"))
		}
	}
	fileIDs := stringsArray(snapshot["file_ids"])
	if len(fileIDs) == 0 && str(snapshot, "file_id") != "" {
		fileIDs = []string{str(snapshot, "file_id")}
	}
	if len(fileIDs) == 0 || len(fileIDs) > 8 {
		return jobs.Result{}, errors.New("样本图片数量应为1至8张")
	}
	if j.ClassID != "" && ownerRole != "admin" && !a.member(ctx, j.ClassID, j.OwnerID) {
		return jobs.Result{}, observability.Coded("class_access_revoked", errors.New("本轮班级访问权限已变更"))
	}
	images := make([]ai.Image, 0, len(fileIDs)+4)
	imageInfos := make([]observability.ImageInfo, 0, len(fileIDs)+4)
	for i, fileID := range fileIDs {
		var mime, key, owner string
		if e := a.Store.DB.QueryRowContext(ctx, `SELECT mime,path,owner_id FROM files WHERE id=?`, fileID).Scan(&mime, &key, &owner); e != nil || owner != j.OwnerID {
			return jobs.Result{}, errors.New("图片无访问权限")
		}
		b, readErr := os.ReadFile(filepath.Join(a.Store.Dir, "files", key))
		if readErr != nil {
			return jobs.Result{}, errors.New("图片读取失败")
		}
		cfg, _, imageErr := image.DecodeConfig(bytes.NewReader(b))
		if imageErr != nil {
			return jobs.Result{}, errors.New("样本图片无法解码")
		}
		role := fmt.Sprintf("当前样本图%d", i+1)
		imageInfos = append(imageInfos, observability.ImageInfo{Role: role, MIME: mime, Bytes: len(b), Width: cfg.Width, Height: cfg.Height})
		images = append(images, ai.Image{MIME: mime, Data: b, Label: fmt.Sprintf("当前待分析图片%d（同一样本，共%d张）", i+1, len(fileIDs))})
	}
	if e := observability.Update(ctx, observability.Details{Images: imageInfos}, true); e != nil {
		return jobs.Result{}, e
	}
	terms := []string{}
	for _, text := range []string{str(snapshot, "material"), str(snapshot, "process"), str(input, "student_judgment"), str(input, "question")} {
		if text != "未知" && text != "" {
			terms = append(terms, text)
		}
	}
	query := strings.Join(terms, " ")
	if e := observability.Stage(ctx, "retrieving", 0); e != nil {
		return jobs.Result{}, e
	}
	sources, e := a.retrieve(ctx, query)
	if e != nil {
		return jobs.Result{}, e
	}
	references, err := a.retrieveReferences(ctx, query, j.ClassID)
	if err != nil {
		return jobs.Result{}, err
	}
	unavailable := 0
	docCount := len(sources)
	referenceCount := 0
	for _, source := range references {
		if len(images) >= 12 {
			unavailable++
			continue
		}
		img, err := a.referenceBytes(ctx, source.ID)
		if err != nil || len(img.Data) > 10<<20 {
			unavailable++
			continue
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(img.Data))
		if err != nil {
			unavailable++
			continue
		}
		images = append(images, img)
		sources = append(sources, source)
		referenceCount++
		imageInfos = append(imageInfos, observability.ImageInfo{Role: "参考图片", MIME: img.MIME, Bytes: len(img.Data), Width: cfg.Width, Height: cfg.Height})
	}
	sourceInfos := []observability.SourceInfo{}
	for _, source := range sources {
		sourceInfos = append(sourceInfos, observability.SourceInfo{Title: source.Title, Kind: source.Kind, Version: source.Version})
	}
	if e = observability.Update(ctx, observability.Details{DocumentCount: &docCount, ReferenceCount: &referenceCount, UnavailableImages: &unavailable, Sources: sourceInfos, Images: imageInfos}, true); e != nil {
		return jobs.Result{}, e
	}
	observability.Logger(ctx).Info("knowledge.retrieved", "document_count", docCount, "reference_count", referenceCount, "unavailable_images", unavailable)
	safeSample := map[string]any{}
	for _, key := range []string{"material", "process", "magnification", "etchant", "demo"} {
		value := snapshot[key]
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			value = "未提供"
		}
		safeSample[key] = value
	}
	safeSample["image_count"] = len(fileIDs)
	safe := map[string]any{"sample": safeSample, "rubric": input["rubric"]}
	for _, key := range []string{"question", "answer", "student_judgment", "target", "reason"} {
		if v, yes := input[key]; yes {
			safe[key] = v
		}
	}
	if j.Kind == "coaching" {
		safe["history"] = input["teaching_history"]
		safe["practice_goal"] = input["practice_goal"]
	}
	payload, _ := json.Marshal(safe)
	out, e := a.AI.Generate(ctx, ai.Request{Kind: j.Kind, Input: payload, Images: images, Sources: sources}, emit)
	if e != nil {
		return jobs.Result{}, e
	}
	var data map[string]any
	if e = json.Unmarshal(out, &data); e != nil {
		return jobs.Result{}, errors.New("模型结果结构无效")
	}
	rubric, _ := input["rubric"].(map[string]any)
	if j.Kind == "assessment" {
		if e = scoreData(data, rubric); e != nil {
			return jobs.Result{}, e
		}
	}
	return jobs.Result{Kind: "reports", Data: store.Entity{"demo": demoRecord(snapshot), "source_atlas_id": snapshot["source_atlas_id"], "kind": j.Kind, "sample_id": j.SampleID, "sample_version": snapshot["version"], "sample_snapshot": snapshot, "learning_session_id": j.LearningSessionID, "job_id": j.ID, "data": data, "original_data": data, "revision_no": 0, "latest_revision": nil, "sources": citedSources(data, sources), "retrieved_sources": sources, "rubric": rubric, "model": a.Config.AIModel, "prompt_version": ai.PromptVersion, "status": "pending_review"}}, nil
}
func (a *App) runImport(ctx context.Context, j jobs.Job, in map[string]any, emit func(string) error) (jobs.Result, error) {
	var name, key, owner string
	if e := a.Store.DB.QueryRowContext(ctx, `SELECT name,path,owner_id FROM files WHERE id=?`, str(in, "file_id")).Scan(&name, &key, &owner); e != nil {
		return jobs.Result{}, errors.New("资料文件不存在")
	}
	if e := observability.Stage(ctx, "extracting", 0); e != nil {
		return jobs.Result{}, e
	}
	if e := emit("正在提取资料正文…"); e != nil {
		return jobs.Result{}, e
	}
	b, e := os.ReadFile(filepath.Join(a.Store.Dir, "files", key))
	if e != nil {
		return jobs.Result{}, e
	}
	text, e := knowledge.Extract(name, b)
	if e != nil {
		return jobs.Result{}, e
	}
	parts := knowledge.Chunk(text)
	chunks := []jobs.Chunk{}
	completed, total := 0, len(parts)
	if e = observability.CountStage(ctx, "chunking", 0, &completed, &total); e != nil {
		return jobs.Result{}, e
	}
	for i, part := range parts {
		chunks = append(chunks, jobs.Chunk{ID: store.ID(), Title: str(in, "title"), Content: part, Tokens: knowledge.Tokenize(part + " " + str(in, "title") + " " + knowledgeTagText(in)), Position: i})
	}
	completed = len(chunks)
	if e = observability.CountStage(ctx, "chunking", 0, &completed, &total); e != nil {
		return jobs.Result{}, e
	}
	if len(chunks) == 0 {
		return jobs.Result{}, errors.New("文档没有可索引正文")
	}
	document := store.Entity{"title": str(in, "title"), "file_id": str(in, "file_id"), "content": text, "chunk_count": len(chunks), "demo": false, "status": "draft", "replaces_document_id": in["replaces_document_id"], "document_version": in["document_version"], "series_id": in["series_id"]}
	for _, key := range knowledgeTagKeys {
		document[key] = stringsArray(in[key])
	}
	return jobs.Result{Kind: "documents", Data: document, Chunks: chunks}, nil
}

// coachingContext freezes server-owned report revisions and the assigned goal at
// task creation; browser-provided history is never treated as teaching evidence.
func (a *App) coachingContext(ctx context.Context, owner string, sample store.Entity, sessionID string) ([]map[string]any, map[string]any, error) {
	parentID := ""
	goal := map[string]any{}
	if sessionID != "" {
		session, e := a.Store.Get(ctx, "learning-sessions", sessionID)
		if e != nil || session.String("owner_id") != owner {
			return nil, nil, errors.New("学习轮次不可访问")
		}
		parentID = session.String("parent_session_id")
		if taskID := session.String("task_id"); taskID != "" {
			task, e := a.Store.Get(ctx, "practice-tasks", taskID)
			if e != nil || !assigned(task, owner) {
				return nil, nil, errors.New("关联练习任务不可访问")
			}
			goal = map[string]any{"title": task["title"], "instruction": task["instruction"]}
		}
	}
	reports, e := a.Store.List(ctx, "reports")
	if e != nil {
		return nil, nil, e
	}
	out := []map[string]any{}
	seen := map[string]bool{}
	for _, rep := range reports {
		kind := rep.String("kind")
		if rep.String("owner_id") != owner || (kind != "assessment" && kind != "understanding") {
			continue
		}
		phase := ""
		if sessionID != "" {
			if rep.String("learning_session_id") == sessionID && rep["sample_version"] == sample["version"] {
				phase = "current"
			} else if parentID != "" && rep.String("learning_session_id") == parentID {
				phase = "previous"
			}
		} else if rep.String("sample_id") == sample.String("id") && rep["sample_version"] == sample["version"] {
			phase = "current"
		}
		if phase == "" || seen[phase+kind] {
			continue
		}
		seen[phase+kind] = true
		data := rep["data"]
		if revision, ok := rep["latest_revision"].(map[string]any); ok && revision["data"] != nil {
			data = revision["data"]
		}
		if historical, ok := data.(map[string]any); ok {
			clean := map[string]any{}
			for key, value := range historical {
				if key != "citations" {
					clean[key] = value
				}
			}
			data = clean
		}
		out = append(out, map[string]any{"phase": phase, "kind": kind, "data": data, "revision_no": rep["revision_no"]})
	}
	return out, goal, nil
}

func citedSources(data map[string]any, retrieved []ai.Source) []ai.Source {
	ids := map[string]bool{}
	for _, id := range stringsArray(data["citations"]) {
		ids[id] = true
	}
	out := []ai.Source{}
	for _, source := range retrieved {
		if ids[source.ID] {
			out = append(out, source)
		}
	}
	return out
}
