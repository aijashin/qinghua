// Package observability separates private terminal diagnostics from public job progress.
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"
)

type Config struct {
	Level, Format string
	AIContent     bool
	Secrets       []string
}
type settings struct {
	content bool
	secrets []string
}

var current atomic.Pointer[settings]
var contentMu sync.Mutex

func Configure(w io.Writer, c Config) error {
	var level slog.Level
	switch strings.ToLower(c.Level) {
	case "", "info":
		level = slog.LevelInfo
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return fmt.Errorf("ZHIWEI_LOG_LEVEL must be debug, info, warn or error")
	}
	options := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	switch c.Format {
	case "", "text":
		h = newTerminalHandler(w, options)
	case "json":
		h = slog.NewJSONHandler(w, options)
	default:
		return fmt.Errorf("ZHIWEI_LOG_FORMAT must be text or json")
	}
	slog.SetDefault(slog.New(h))
	current.Store(&settings{content: c.AIContent, secrets: append([]string(nil), c.Secrets...)})
	return nil
}
func ID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

type contextKey struct{}
type Scope struct {
	RequestID, JobID string
	run              *Run
	budget           *budget
}
type budget struct {
	mu   sync.Mutex
	used int
}

func WithRequest(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, &Scope{RequestID: id, budget: &budget{}})
}
func RequestID(ctx context.Context) string {
	s, _ := ctx.Value(contextKey{}).(*Scope)
	if s == nil {
		return ""
	}
	return s.RequestID
}
func Logger(ctx context.Context) *slog.Logger {
	s, _ := ctx.Value(contextKey{}).(*Scope)
	if s == nil {
		return slog.Default()
	}
	l := slog.Default().With("request_id", s.RequestID)
	if s.JobID != "" {
		l = l.With("job_id", s.JobID)
	}
	return l
}

type ImageInfo struct {
	Role   string `json:"role"`
	MIME   string `json:"mime"`
	Bytes  int    `json:"bytes"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type SourceInfo struct {
	Title   string `json:"title"`
	Kind    string `json:"source_kind"`
	Version int    `json:"source_version"`
}
type CheckInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// Explicit fields keep prompts, provider text and credentials out of public events.
type Details struct {
	Images            []ImageInfo  `json:"images,omitempty"`
	Sources           []SourceInfo `json:"sources,omitempty"`
	DocumentCount     *int         `json:"document_count,omitempty"`
	ReferenceCount    *int         `json:"reference_count,omitempty"`
	UnavailableImages *int         `json:"unavailable_images,omitempty"`
	SentAt            string       `json:"sent_at,omitempty"`
	ResponseAt        string       `json:"response_at,omitempty"`
	FirstContentMS    *int64       `json:"first_content_ms,omitempty"`
	ReceivedBytes     *int         `json:"received_bytes,omitempty"`
	ReceivedChunks    *int         `json:"received_chunks,omitempty"`
	TimeoutMS         int64        `json:"timeout_ms,omitempty"`
	Checks            []CheckInfo  `json:"checks,omitempty"`
	Call              *CallTiming  `json:"call,omitempty"`
	Saved             bool         `json:"saved,omitempty"`
}
type Progress struct {
	StageID   string   `json:"stage_id,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
	Details   *Details `json:"details,omitempty"`
	Code      string   `json:"code"`
	Label     string   `json:"label"`
	At        string   `json:"at"`
	ElapsedMS int64    `json:"elapsed_ms"`
	Attempt   int      `json:"attempt,omitempty"`
	Completed *int     `json:"completed,omitempty"`
	Total     *int     `json:"total,omitempty"`
}
type StageTiming struct {
	Code       string `json:"code"`
	Attempt    int    `json:"attempt,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}
type CallTiming struct {
	Attempt        int    `json:"attempt"`
	DurationMS     int64  `json:"duration_ms"`
	FirstContentMS *int64 `json:"first_content_ms,omitempty"`
	InputTokens    *int64 `json:"input_tokens,omitempty"`
	OutputTokens   *int64 `json:"output_tokens,omitempty"`
	TotalTokens    *int64 `json:"total_tokens,omitempty"`
}
type Timings struct {
	QueueMS    *int64        `json:"queue_ms,omitempty"`
	TotalMS    int64         `json:"total_ms"`
	ModelMS    int64         `json:"model_ms"`
	Stages     []StageTiming `json:"stages,omitempty"`
	ModelCalls []CallTiming  `json:"model_calls,omitempty"`
}

// Avoid pointer-address formatting in human terminal metadata.
func (t *Timings) LogValue() slog.Value {
	if t == nil {
		return slog.AnyValue(nil)
	}
	attrs := []slog.Attr{slog.Int64("total_ms", t.TotalMS), slog.Int64("model_ms", t.ModelMS)}
	if t.QueueMS != nil {
		attrs = append(attrs, slog.Int64("queue_ms", *t.QueueMS))
	}
	return slog.GroupValue(attrs...)
}

var labels = map[string]string{"queued": "等待处理", "preparing": "准备图像与任务资料", "retrieving": "检索已发布教学资料", "model_waiting": "等待模型响应", "model_receiving": "正在接收模型输出", "validating": "校验报告结构与依据", "repairing": "修复报告格式", "persisting": "保存分析结果", "extracting": "提取资料正文", "chunking": "整理资料片段", "succeeded": "已完成", "failed": "处理失败", "cancelled": "已取消"}

func NewProgress(code string) Progress {
	return Progress{StageID: ID(), Code: code, Label: labels[code], At: time.Now().UTC().Format(time.RFC3339Nano)}
}

type Run struct {
	mu            sync.Mutex
	started, last time.Time
	queue         *int64
	history       []Progress
	stages        []StageTiming
	calls         []CallTiming
	activeCalls   map[int]time.Time
	lastDetail    time.Time
	emit          func(Progress) error
}

func WithRun(ctx context.Context, requestID, jobID string, queued time.Time, emit func(Progress) error) (context.Context, *Run) {
	now := time.Now()
	r := &Run{started: now, last: now, emit: emit}
	if !queued.IsZero() {
		q := max(int64(0), now.Sub(queued).Milliseconds())
		r.queue = &q
	}
	return context.WithValue(ctx, contextKey{}, &Scope{RequestID: requestID, JobID: jobID, run: r, budget: &budget{}}), r
}
func Stage(ctx context.Context, code string, attempt int) error {
	return CountStage(ctx, code, attempt, nil, nil)
}
func CountStage(ctx context.Context, code string, attempt int, completed, total *int) error {
	if _, ok := labels[code]; !ok {
		return errors.New("unknown progress stage")
	}
	s, _ := ctx.Value(contextKey{}).(*Scope)
	if s == nil || s.run == nil {
		return ctx.Err()
	}
	r := s.run
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	now := time.Now()
	p := NewProgress(code)
	p.ElapsedMS = now.Sub(r.started).Milliseconds()
	p.Attempt = attempt
	p.Completed = completed
	p.Total = total
	if r.emit != nil {
		if err := r.emit(p); err != nil {
			return err
		}
	}
	if len(r.history) > 0 {
		previous := r.history[len(r.history)-1]
		r.stages = append(r.stages, StageTiming{Code: previous.Code, Attempt: previous.Attempt, DurationMS: now.Sub(r.last).Milliseconds()})
	}
	r.last = now
	r.history = append(r.history, p)
	Logger(ctx).Info("job.stage", "stage", code, "attempt", attempt, "elapsed_ms", p.ElapsedMS)
	return nil
}

// Update changes only the current stage snapshot, never its start time.
func Update(ctx context.Context, patch Details, force bool) error {
	scope, _ := ctx.Value(contextKey{}).(*Scope)
	if scope == nil || scope.run == nil {
		return ctx.Err()
	}
	r := scope.run
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(r.history) == 0 {
		return nil
	}
	now := time.Now()
	if !force && now.Sub(r.lastDetail) < time.Second {
		return nil
	}
	i := len(r.history) - 1
	p := r.history[i]
	merged := map[string]any{}
	if p.Details != nil {
		raw, _ := json.Marshal(p.Details)
		_ = json.Unmarshal(raw, &merged)
	}
	raw, _ := json.Marshal(patch)
	delta := map[string]any{}
	_ = json.Unmarshal(raw, &delta)
	for key, value := range delta {
		merged[key] = value
	}
	var details Details
	raw, _ = json.Marshal(merged)
	_ = json.Unmarshal(raw, &details)
	p.Details = &details
	p.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	if r.emit != nil {
		if err := r.emit(p); err != nil {
			return err
		}
	}
	r.history[i] = p
	r.lastDetail = now
	return nil
}

func (r *Run) Timings() Timings {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := Timings{QueueMS: r.queue, TotalMS: time.Since(r.started).Milliseconds(), Stages: append([]StageTiming(nil), r.stages...), ModelCalls: append([]CallTiming(nil), r.calls...)}
	if len(r.history) > 0 {
		p := r.history[len(r.history)-1]
		t.Stages = append(t.Stages, StageTiming{Code: p.Code, Attempt: p.Attempt, DurationMS: time.Since(r.last).Milliseconds()})
	}
	for i := range t.ModelCalls {
		if started, yes := r.activeCalls[t.ModelCalls[i].Attempt]; yes {
			t.ModelCalls[i].DurationMS = time.Since(started).Milliseconds()
		}
		t.ModelMS += t.ModelCalls[i].DurationMS
	}
	return t
}
func BeginCall(ctx context.Context, attempt int, started time.Time) {
	s, _ := ctx.Value(contextKey{}).(*Scope)
	if s == nil || s.run == nil {
		return
	}
	r := s.run
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activeCalls == nil {
		r.activeCalls = map[int]time.Time{}
	}
	r.activeCalls[attempt] = started
	r.calls = append(r.calls, CallTiming{Attempt: attempt})
}
func FirstContent(ctx context.Context, attempt int, ms int64) {
	s, _ := ctx.Value(contextKey{}).(*Scope)
	if s == nil || s.run == nil {
		return
	}
	r := s.run
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.calls {
		if r.calls[i].Attempt == attempt {
			r.calls[i].FirstContentMS = &ms
			return
		}
	}
}
func RecordCall(ctx context.Context, c CallTiming) {
	s, _ := ctx.Value(contextKey{}).(*Scope)
	if s == nil || s.run == nil {
		return
	}
	r := s.run
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.activeCalls, c.Attempt)
	for i := range r.calls {
		if r.calls[i].Attempt == c.Attempt {
			r.calls[i] = c
			return
		}
	}
	r.calls = append(r.calls, c)
}
func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "interrupted"
	}
	var coded interface{ Code() string }
	if errors.As(err, &coded) {
		return coded.Code()
	}
	return "processing_failed"
}

type codedError struct {
	code string
	err  error
}

func (e codedError) Error() string       { return e.err.Error() }
func (e codedError) Unwrap() error       { return e.err }
func (e codedError) Code() string        { return e.code }
func Coded(code string, err error) error { return codedError{code, err} }

var dataURL = regexp.MustCompile(`(?i)data:[^\s"'<>]{0,100};base64,[A-Za-z0-9+/=_\-\s]+`)
var base64Run = regexp.MustCompile(`[A-Za-z0-9+/_\-]{160,}={0,2}`)
var credential = regexp.MustCompile(`(?i)(["']?(?:api[_-]?key|authorization|cookie|set-cookie|password|secret|access[_-]?token|refresh[_-]?token|csrf|session[_-]?token)["']?\s*[:=]\s*)(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s,;}]+)`)
var bearer = regexp.MustCompile(`(?i)\bBearer\s+[^\s"',;]+`)
var reasoningField = regexp.MustCompile(`(?is)("(?:reasoning_content|analysis|thinking|reasoning)"\s*:\s*)"(?:\\.|[^"\\])*"`)
var thinking = regexp.MustCompile(`(?is)<(?:think|thinking|analysis|reasoning)(?:\s[^>]*)?>.*?(?:</(?:think|thinking|analysis|reasoning)>|$)`)

func Redact(text string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[秘密已隐藏]")
		}
	}
	text = thinking.ReplaceAllString(text, "[内部思考已隐藏]")
	text = reasoningField.ReplaceAllString(text, `${1}"[内部思考已隐藏]"`)
	text = dataURL.ReplaceAllString(text, "[图片Base64已隐藏]")
	text = credential.ReplaceAllString(text, "${1}[秘密已隐藏]")
	text = bearer.ReplaceAllString(text, "Bearer [已隐藏]")
	text = base64Run.ReplaceAllString(text, "[长编码内容已隐藏]")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, strings.ToValidUTF8(text, "�"))
}
func boundary(s string, n int) int {
	n = min(n, len(s))
	for n > 0 && n < len(s) && !utf8.RuneStart(s[n]) {
		n--
	}
	return n
}

// Content is terminal-only. Never pass this output to a progress callback or DB.
func Content(ctx context.Context, callID string, attempt int, direction, text string, secrets ...string) {
	cfg := current.Load()
	if cfg == nil || !cfg.content || !Logger(ctx).Enabled(ctx, slog.LevelDebug) {
		return
	}
	original := len(text)
	text = Redact(text, append(append([]string(nil), cfg.secrets...), secrets...)...)
	omitted := 0
	if len(text) > 32<<10 {
		marker := "\n[中间内容已截断]\n"
		head := boundary(text, (24<<10)-len(marker))
		tailStart := len(text) - (8 << 10)
		for tailStart < len(text) && !utf8.RuneStart(text[tailStart]) {
			tailStart++
		}
		omitted = tailStart - head
		text = text[:head] + marker + text[tailStart:]
	}
	s, _ := ctx.Value(contextKey{}).(*Scope)
	if s != nil && s.budget != nil {
		s.budget.mu.Lock()
		remaining := (128 << 10) - s.budget.used
		if remaining <= 0 {
			s.budget.mu.Unlock()
			Logger(ctx).Debug("ai.content.omitted", "call_id", callID, "attempt", attempt, "direction", direction, "original_bytes", original, "omitted_bytes", len(text), "reason", "job_content_budget_exhausted")
			return
		}
		if len(text) > remaining {
			cut := boundary(text, remaining)
			omitted += len(text) - cut
			text = text[:cut]
		}
		s.budget.used += len(text)
		s.budget.mu.Unlock()
	}
	contentMu.Lock()
	defer contentMu.Unlock()
	part := 1
	for len(text) > 0 {
		n := boundary(text, 4<<10)
		chunk := text[:n]
		text = text[n:]
		Logger(ctx).Debug("ai.content", "call_id", callID, "attempt", attempt, "direction", direction, "part", part, "original_bytes", original, "omitted_bytes", omitted, "text", chunk)
		part++
	}
}
