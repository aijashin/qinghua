package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"time"
	"zhiwei/backend/internal/observability"
	"zhiwei/backend/internal/store"
)

type Diagnostics struct {
	OriginRequestID string                   `json:"origin_request_id,omitempty"`
	StartedAt       string                   `json:"started_at,omitempty"`
	FinishedAt      string                   `json:"finished_at,omitempty"`
	ErrorCode       string                   `json:"error_code,omitempty"`
	Progress        *observability.Progress  `json:"progress,omitempty"`
	ProgressHistory []observability.Progress `json:"progress_history,omitempty"`
	Timings         *observability.Timings   `json:"timings,omitempty"`
}
type Job struct {
	Diagnostics
	ID                string          `json:"id"`
	OwnerID           string          `json:"owner_id"`
	ClassID           string          `json:"class_id"`
	Kind              string          `json:"kind"`
	SampleID          string          `json:"sample_id"`
	LearningSessionID string          `json:"learning_session_id"`
	Input             json.RawMessage `json:"-"`
	Status            string          `json:"status"`
	Stage             string          `json:"stage"`
	Error             string          `json:"error,omitempty"`
	ResultID          string          `json:"result_id,omitempty"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
	Lease             string          `json:"-"`
}
type Chunk struct {
	ID, Title, Content, Tokens string
	Position                   int
}
type Result struct {
	Kind   string
	Data   store.Entity
	Chunks []Chunk
}
type Runner func(context.Context, Job, func(string) error) (Result, error)
type Pool struct {
	s      *store.Store
	run    Runner
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
	active map[string]context.CancelFunc
	runs   map[string]*observability.Run
}

func New(s *store.Store, r Runner) *Pool {
	return &Pool{s: s, run: r, active: map[string]context.CancelFunc{}, runs: map[string]*observability.Run{}}
}

const cols = `id,owner_id,class_id,kind,sample_id,learning_session_id,input,status,stage,error,result_id,created_at,updated_at,lease,telemetry`

var ErrIdempotencyConflict = errors.New("幂等键已用于不同请求，请刷新后重新提交")

func scan(row interface{ Scan(...any) error }) (Job, error) {
	var j Job
	var input, telemetry string
	e := row.Scan(&j.ID, &j.OwnerID, &j.ClassID, &j.Kind, &j.SampleID, &j.LearningSessionID, &input, &j.Status, &j.Stage, &j.Error, &j.ResultID, &j.CreatedAt, &j.UpdatedAt, &j.Lease, &telemetry)
	j.Input = json.RawMessage(input)
	if e == nil {
		e = json.Unmarshal([]byte(telemetry), &j.Diagnostics)
	}
	return j, e
}
func (p *Pool) Get(ctx context.Context, id string) (Job, error) {
	return scan(p.s.DB.QueryRowContext(ctx, `SELECT `+cols+` FROM jobs WHERE id=?`, id))
}
func (p *Pool) List(ctx context.Context) ([]Job, error) {
	rows, e := p.s.DB.QueryContext(ctx, `SELECT `+cols+` FROM jobs ORDER BY created_at DESC`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		j, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
func (p *Pool) Create(ctx context.Context, j Job, key string) (Job, error) {
	if !json.Valid(j.Input) {
		return Job{}, errors.New("任务输入不是有效JSON")
	}
	j.ID = store.ID()
	j.CreatedAt = store.Now()
	j.UpdatedAt = j.CreatedAt
	j.Status = "queued"
	j.Stage = "等待处理"
	progress := observability.NewProgress("queued")
	j.Diagnostics = Diagnostics{OriginRequestID: observability.RequestID(ctx), Progress: &progress, ProgressHistory: []observability.Progress{progress}}
	telemetry, _ := json.Marshal(j.Diagnostics)
	tx, e := p.s.DB.BeginTx(ctx, nil)
	if e != nil {
		return Job{}, e
	}
	defer tx.Rollback()
	result, e := tx.ExecContext(ctx, `INSERT INTO jobs(id,owner_id,class_id,kind,sample_id,learning_session_id,input,status,stage,idempotency_key,created_at,updated_at,telemetry) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(owner_id,idempotency_key) DO NOTHING`, j.ID, j.OwnerID, j.ClassID, j.Kind, j.SampleID, j.LearningSessionID, string(j.Input), j.Status, j.Stage, key, j.CreatedAt, j.UpdatedAt, string(telemetry))
	if e != nil {
		return j, e
	}
	existing, e := scan(tx.QueryRowContext(ctx, `SELECT `+cols+` FROM jobs WHERE owner_id=? AND idempotency_key=?`, j.OwnerID, key))
	if e != nil {
		return Job{}, e
	}
	var original, candidate any
	if json.Unmarshal(existing.Input, &original) != nil || json.Unmarshal(j.Input, &candidate) != nil ||
		existing.Kind != j.Kind || existing.ClassID != j.ClassID || existing.SampleID != j.SampleID ||
		existing.LearningSessionID != j.LearningSessionID || !reflect.DeepEqual(original, candidate) {
		return Job{}, ErrIdempotencyConflict
	}
	inserted, e := result.RowsAffected()
	if e != nil {
		return Job{}, e
	}
	if inserted > 0 {
		if e = eventTx(ctx, tx, j.ID, "progress", progress); e != nil {
			return Job{}, e
		}
		if e = eventTx(ctx, tx, j.ID, "status", j); e != nil {
			return Job{}, e
		}
	}
	e = tx.Commit()
	if e == nil && inserted > 0 {
		observability.Logger(ctx).Info("job.queued", "job_id", j.ID, "kind", j.Kind)
	}
	return existing, e
}
func eventTx(ctx context.Context, tx *sql.Tx, id, event string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	var seq int
	if e = tx.QueryRowContext(ctx, `UPDATE jobs SET next_seq=next_seq+1,updated_at=? WHERE id=? RETURNING next_seq`, store.Now(), id).Scan(&seq); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO job_events VALUES(?,?,?,?,?)`, id, seq, event, string(b), store.Now())
	return e
}
func (p *Pool) Emit(ctx context.Context, id, event string, v any) error {
	tx, e := p.s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var status string
	if e = tx.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id=?`, id).Scan(&status); e != nil {
		return e
	}
	if status != "running" && status != "queued" {
		return context.Canceled
	}
	if e = eventTx(ctx, tx, id, event, v); e != nil {
		return e
	}
	return tx.Commit()
}
func (p *Pool) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	p.cancel = cancel
	// A fresh process cannot safely replay paid calls left in running state.
	rows, _ := p.s.DB.QueryContext(ctx, `SELECT id FROM jobs WHERE status='running'`)
	ids := []string{}
	if rows != nil {
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			ids = append(ids, id)
		}
		rows.Close()
	}
	for _, id := range ids {
		_ = p.failDetails(ctx, id, "上次服务中断，请手动重试", "failed", "interrupted", nil)
	}
	for i := 0; i < 2; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			ticker := time.NewTicker(150 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					j, e := p.claim(ctx)
					if e == nil {
						p.execute(ctx, j)
					}
				}
			}
		}()
	}
}
func (p *Pool) Stop() {
	if p.cancel != nil {
		p.cancel()
		p.wg.Wait()
	}
}
func (p *Pool) claim(ctx context.Context) (Job, error) {
	return scan(p.s.DB.QueryRowContext(ctx, `UPDATE jobs SET status='running',stage='准备资料',lease=?,lease_until=?,updated_at=?,telemetry=json_set(telemetry,'$.started_at',?) WHERE id=(SELECT id FROM jobs WHERE status='queued' ORDER BY created_at LIMIT 1) AND status='queued' RETURNING `+cols, store.ID(), time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano), store.Now(), store.Now()))
}
func (p *Pool) execute(parent context.Context, j Job) {
	var run *observability.Run
	timing := func() *observability.Timings {
		if run == nil {
			return nil
		}
		t := run.Timings()
		return &t
	}
	defer func() {
		if recover() != nil {
			// Panic values may contain provider input; log only the job ID.
			slog.Error("job runner panicked", "job_id", j.ID)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = p.failDetails(ctx, j.ID, "任务处理异常，请联系管理员或重试", "failed", "internal_error", timing())
		}
	}()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	queued, _ := time.Parse(time.RFC3339Nano, j.CreatedAt)
	ctx, run = observability.WithRun(ctx, j.OriginRequestID, j.ID, queued, func(v observability.Progress) error { return p.progress(ctx, j.ID, v) })
	p.mu.Lock()
	p.active[j.ID] = cancel
	p.runs[j.ID] = run
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.active, j.ID); delete(p.runs, j.ID); p.mu.Unlock() }()
	progressFailure := func(err error) {
		code, message := observability.ErrorCode(err), "保存任务进度失败，请重试"
		if errors.Is(err, context.Canceled) {
			message = "任务已中断"
		} else if code == "processing_failed" {
			code = "persistence_failed"
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.failDetails(cleanup, j.ID, message, "failed", code, timing())
	}
	// Cancellation can happen between claim and registration in active.
	current, err := p.Get(ctx, j.ID)
	if err != nil || current.Status != "running" || current.Lease != j.Lease {
		return
	}
	if err = p.Emit(ctx, j.ID, "status", j); err != nil {
		progressFailure(err)
		return
	}
	if err = ctx.Err(); err != nil {
		progressFailure(err)
		return
	}
	if err = observability.Stage(ctx, "preparing", 0); err != nil {
		progressFailure(err)
		return
	}
	observability.Logger(ctx).Info("job.started", "kind", j.Kind)
	done := make(chan struct{})
	defer close(done)
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				_, _ = p.s.DB.ExecContext(ctx, `UPDATE jobs SET lease_until=? WHERE id=? AND lease=? AND status='running'`, time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano), j.ID, j.Lease)
			}
		}
	}()
	// Coalesce token deltas before SQLite writes; keep durable replay bounded.
	var pending strings.Builder
	lastFlush := time.Now()
	flush := func() error {
		if pending.Len() == 0 {
			return ctx.Err()
		}
		text := pending.String()
		pending.Reset()
		lastFlush = time.Now()
		return p.Emit(ctx, j.ID, "delta", map[string]string{"text": text})
	}
	result, e := p.run(ctx, j, func(text string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if pending.Len() > 0 {
			pending.WriteString("\n")
		}
		pending.WriteString(text)
		if time.Since(lastFlush) >= 200*time.Millisecond || pending.Len() >= 4096 {
			return flush()
		}
		return nil
	})
	if e == nil {
		e = flush()
	}
	if e != nil {
		status := "failed"
		msg := e.Error()
		if errors.Is(e, context.Canceled) {
			msg = "任务已中断"
		}
		_ = p.failDetails(context.Background(), j.ID, msg, status, observability.ErrorCode(e), timing())
		return
	}
	if e = observability.Stage(ctx, "persisting", 0); e != nil {
		progressFailure(e)
		return
	}
	j.Timings = timing()
	if e = p.finish(ctx, j, result); e != nil && !errors.Is(e, context.Canceled) {
		_ = p.failDetails(context.Background(), j.ID, "保存结果失败", "failed", "persistence_failed", timing())
	}
}
func (p *Pool) finish(ctx context.Context, j Job, r Result) error {
	tx, e := p.s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var state, lease, telemetry string
	if e = tx.QueryRowContext(ctx, `SELECT status,lease,telemetry FROM jobs WHERE id=?`, j.ID).Scan(&state, &lease, &telemetry); e != nil {
		return e
	}
	if state != "running" || lease != j.Lease {
		return context.Canceled
	}
	id, now := store.ID(), store.Now()
	var d Diagnostics
	if e = json.Unmarshal([]byte(telemetry), &d); e != nil {
		return e
	}
	d.FinishedAt = now
	d.Timings = j.Timings
	terminal := observability.NewProgress("succeeded")
	terminal.Details = &observability.Details{Saved: true}
	if d.Timings != nil {
		terminal.ElapsedMS = d.Timings.TotalMS
	}
	d.Progress = &terminal
	d.ProgressHistory = append(d.ProgressHistory, terminal)
	telemetryBytes, _ := json.Marshal(d)
	v := r.Data
	if v == nil {
		v = store.Entity{}
	}
	if d.StartedAt != "" {
		v["started_at"] = d.StartedAt
	}
	v["finished_at"] = now
	if d.Timings != nil {
		v["timings"] = d.Timings
	}
	v["id"] = id
	v["owner_id"] = j.OwnerID
	v["class_id"] = j.ClassID
	v["version"] = 1
	v["created_at"] = now
	v["updated_at"] = now
	status := v.String("status")
	if status == "" {
		status = "pending_review"
		v["status"] = status
	}
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO entities(id,kind,owner_id,class_id,status,data,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, id, r.Kind, j.OwnerID, j.ClassID, status, string(b), now, now); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO entity_versions VALUES(?,?,?,?)`, id, 1, string(b), now); e != nil {
		return e
	}
	for _, c := range r.Chunks {
		if _, e = tx.ExecContext(ctx, `INSERT INTO knowledge_chunks VALUES(?,?,?,?,?)`, c.ID, id, c.Title, c.Content, c.Position); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO knowledge_fts(chunk_id,tokens) VALUES(?,?)`, c.ID, c.Tokens); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `UPDATE jobs SET status='succeeded',stage='已完成',result_id=?,updated_at=?,telemetry=? WHERE id=?`, id, now, string(telemetryBytes), j.ID); e != nil {
		return e
	}
	if e = eventTx(ctx, tx, j.ID, "progress", terminal); e != nil {
		return e
	}
	if e = eventTx(ctx, tx, j.ID, "completed", map[string]any{"job_id": j.ID, "report_id": id, "result_id": id, "result_kind": r.Kind, "finished_at": now, "timings": d.Timings}); e != nil {
		return e
	}
	e = tx.Commit()
	if e == nil {
		observability.Logger(ctx).Info("job.finished", "status", "succeeded", "result_id", id, "timings", d.Timings)
	}
	return e
}
func (p *Pool) progress(ctx context.Context, id string, v observability.Progress) error {
	tx, e := p.s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var raw, status string
	if e = tx.QueryRowContext(ctx, `SELECT telemetry,status FROM jobs WHERE id=?`, id).Scan(&raw, &status); e != nil {
		return e
	}
	if Terminal(status) {
		return context.Canceled
	}
	var d Diagnostics
	if e = json.Unmarshal([]byte(raw), &d); e != nil {
		return e
	}
	d.Progress = &v
	found := false
	for i, previous := range d.ProgressHistory {
		if v.StageID != "" && previous.StageID == v.StageID {
			d.ProgressHistory[i] = v
			found = true
			break
		}
	}
	if !found {
		d.ProgressHistory = append(d.ProgressHistory, v)
	}
	b, _ := json.Marshal(d)
	if _, e = tx.ExecContext(ctx, `UPDATE jobs SET telemetry=?,stage=? WHERE id=?`, string(b), v.Label, id); e != nil {
		return e
	}
	if e = eventTx(ctx, tx, id, "progress", v); e != nil {
		return e
	}
	return tx.Commit()
}
func (p *Pool) fail(ctx context.Context, id, msg, status string) error {
	return p.failDetails(ctx, id, msg, status, status, nil)
}
func (p *Pool) failDetails(ctx context.Context, id, msg, status, code string, timings *observability.Timings) error {
	tx, e := p.s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var raw, current string
	if e = tx.QueryRowContext(ctx, `SELECT telemetry,status FROM jobs WHERE id=?`, id).Scan(&raw, &current); e != nil {
		return e
	}
	if Terminal(current) {
		return nil
	}
	var d Diagnostics
	if e = json.Unmarshal([]byte(raw), &d); e != nil {
		return e
	}
	d.FinishedAt = store.Now()
	d.ErrorCode = code
	d.Timings = timings
	terminal := observability.NewProgress(status)
	if timings != nil {
		terminal.ElapsedMS = timings.TotalMS
	}
	d.Progress = &terminal
	d.ProgressHistory = append(d.ProgressHistory, terminal)
	b, _ := json.Marshal(d)
	if _, e = tx.ExecContext(ctx, `UPDATE jobs SET status=?,error=?,stage=?,updated_at=?,telemetry=? WHERE id=?`, status, msg, terminal.Label, d.FinishedAt, string(b), id); e != nil {
		return e
	}
	if e = eventTx(ctx, tx, id, "progress", terminal); e != nil {
		return e
	}
	if e = eventTx(ctx, tx, id, "error", map[string]any{"code": code, "message": msg, "finished_at": d.FinishedAt, "timings": timings}); e != nil {
		return e
	}
	e = tx.Commit()
	if e == nil {
		slog.Info("job.finished", "job_id", id, "request_id", d.OriginRequestID, "status", status, "error_code", code)
	}
	return e
}
func (p *Pool) Cancel(ctx context.Context, id string) error {
	p.mu.Lock()
	run := p.runs[id]
	p.mu.Unlock()
	var timing *observability.Timings
	if run != nil {
		t := run.Timings()
		timing = &t
	}
	if e := p.failDetails(ctx, id, "任务已取消", "cancelled", "cancelled", timing); e != nil {
		return e
	}
	p.mu.Lock()
	if c := p.active[id]; c != nil {
		c()
	}
	p.mu.Unlock()
	return nil
}

type Event struct {
	Seq         int
	Event, Data string
}

func (p *Pool) Events(ctx context.Context, id string, after int) ([]Event, error) {
	rows, e := p.s.DB.QueryContext(ctx, `SELECT seq,event,data FROM job_events WHERE job_id=? AND seq>? ORDER BY seq LIMIT 200`, id, after)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var v Event
		if e = rows.Scan(&v.Seq, &v.Event, &v.Data); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func Terminal(s string) bool { return s == "succeeded" || s == "failed" || s == "cancelled" }
