package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"zhiwei/backend/internal/observability"
	"zhiwei/backend/internal/store"
)

func TestProgressTimingPersistenceAndReportSnapshot(t *testing.T) {
	p := testPool(t, func(ctx context.Context, j Job, emit func(string) error) (Result, error) {
		if observability.RequestID(ctx) != "request-origin" {
			t.Fatal("request ID lost across worker")
		}
		for _, code := range []string{"retrieving", "model_waiting", "model_receiving", "validating"} {
			if e := observability.Stage(ctx, code, 1); e != nil {
				return Result{}, e
			}
			time.Sleep(time.Millisecond)
		}
		first := int64(2)
		observability.RecordCall(ctx, observability.CallTiming{Attempt: 1, DurationMS: 4, FirstContentMS: &first})
		return Result{Kind: "reports", Data: store.Entity{"original_data": store.Entity{"summary": "only-validated-output"}}}, nil
	})
	ctx := observability.WithRequest(t.Context(), "request-origin")
	j, e := p.Create(ctx, Job{OwnerID: "owner", Kind: "assessment", Input: json.RawMessage(`{}`)}, "timing-key")
	if e != nil {
		t.Fatal(e)
	}
	claimed, e := p.claim(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	p.execute(t.Context(), claimed)
	got, e := p.Get(t.Context(), j.ID)
	if e != nil {
		t.Fatal(e)
	}
	if got.Status != "succeeded" || got.OriginRequestID != "request-origin" || got.StartedAt == "" || got.FinishedAt == "" || got.Timings == nil || got.Timings.TotalMS < 4 || got.Timings.ModelMS != 4 || got.Timings.QueueMS == nil {
		t.Fatalf("missing actual timing %+v", got)
	}
	codes := []string{}
	for _, progress := range got.ProgressHistory {
		codes = append(codes, progress.Code)
	}
	if strings.Join(codes, ",") != "queued,preparing,retrieving,model_waiting,model_receiving,validating,persisting,succeeded" {
		t.Fatal(codes)
	}
	events, _ := p.Events(t.Context(), j.ID, 0)
	progresses := 0
	for _, event := range events {
		if event.Event == "progress" {
			progresses++
		}
		if strings.Contains(event.Data, "only-validated-output") {
			t.Fatal("report content leaked into SSE")
		}
	}
	if progresses != len(got.ProgressHistory) {
		t.Fatal("history differs from replay")
	}
	report, e := p.s.Get(t.Context(), "reports", got.ResultID)
	if e != nil {
		t.Fatal(e)
	}
	if report["started_at"] != got.StartedAt || report["finished_at"] != got.FinishedAt || report["timings"].(map[string]any)["total_ms"] != float64(got.Timings.TotalMS) {
		t.Fatal("report timing mismatch")
	}
	reopened := New(p.s, nil)
	restored, e := reopened.Get(t.Context(), j.ID)
	if e != nil || len(restored.ProgressHistory) != 8 {
		t.Fatal("progress not durable")
	}
}
func TestFailureCodeAndCancelHaveRealTerminalTelemetry(t *testing.T) {
	p := testPool(t, func(context.Context, Job, func(string) error) (Result, error) {
		return Result{}, observability.Coded("ai_truncated", errors.New("返回未完整结束"))
	})
	j := create(t, p, "failure-code")
	claimed, _ := p.claim(t.Context())
	p.execute(t.Context(), claimed)
	got, _ := p.Get(t.Context(), j.ID)
	if got.ErrorCode != "ai_truncated" || got.Timings == nil || got.FinishedAt == "" || got.Progress.Code != "failed" {
		t.Fatalf("%+v", got)
	}
	started, done := make(chan struct{}), make(chan struct{})
	p.run = func(ctx context.Context, j Job, emit func(string) error) (Result, error) {
		close(started)
		<-ctx.Done()
		return Result{}, ctx.Err()
	}
	j = create(t, p, "cancel-timing")
	claimed, _ = p.claim(t.Context())
	go func() { p.execute(t.Context(), claimed); close(done) }()
	<-started
	time.Sleep(2 * time.Millisecond)
	if err := p.Cancel(t.Context(), j.ID); err != nil {
		t.Fatal(err)
	}
	<-done
	got, _ = p.Get(t.Context(), j.ID)
	if got.ErrorCode != "cancelled" || got.Timings == nil || got.Timings.TotalMS < 2 || got.Progress.Code != "cancelled" {
		t.Fatalf("%+v", got)
	}
}
