package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"zhiwei/backend/internal/store"
)

func testPool(t *testing.T, runner Runner) *Pool {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := New(s, runner)
	t.Cleanup(func() { p.Stop(); s.DB.Close() })
	return p
}

func create(t *testing.T, p *Pool, key string) Job {
	t.Helper()
	j, err := p.Create(t.Context(), Job{OwnerID: "owner", Kind: "assessment", SampleID: "sample", Input: json.RawMessage(`{"sample":"snapshot","rubric":1}`)}, key)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestIdempotencyRejectsChangedRequest(t *testing.T) {
	p := testPool(t, nil)
	first := create(t, p, "request-1")
	reordered := first
	reordered.Input = json.RawMessage(`{"rubric":1,"sample":"snapshot"}`)
	same, err := p.Create(t.Context(), reordered, "request-1")
	if err != nil || same.ID != first.ID {
		t.Fatalf("idempotent repeat = %v, %v", same, err)
	}
	changed := first
	changed.Input = json.RawMessage(`{"sample":"different","rubric":1}`)
	if _, err := p.Create(t.Context(), changed, "request-1"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed input must conflict: %v", err)
	}
	changed = first
	changed.Kind = "understanding"
	if _, err := p.Create(t.Context(), changed, "request-1"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed kind must conflict: %v", err)
	}
	all, _ := p.List(t.Context())
	if len(all) != 1 {
		t.Fatalf("created %d jobs", len(all))
	}
}

func TestCancelledClaimNeverCallsRunner(t *testing.T) {
	called := false
	p := testPool(t, func(context.Context, Job, func(string) error) (Result, error) {
		called = true
		return Result{}, nil
	})
	j := create(t, p, "cancel-before-run")
	claimed, err := p.claim(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Cancel(t.Context(), j.ID); err != nil {
		t.Fatal(err)
	}
	p.execute(t.Context(), claimed)
	if called {
		t.Fatal("cancelled task reached paid runner")
	}
	stored, _ := p.Get(t.Context(), j.ID)
	if stored.Status != "cancelled" || stored.ResultID != "" {
		t.Fatalf("cancelled result: %+v", stored)
	}
}

func TestCancelledRunnerCannotCommitLateSuccess(t *testing.T) {
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	p := testPool(t, func(context.Context, Job, func(string) error) (Result, error) {
		close(started)
		<-release // Simulates a provider ignoring cancellation and returning late.
		return Result{Kind: "reports", Data: store.Entity{"data": "late"}}, nil
	})
	j := create(t, p, "cancel-during-run")
	claimed, _ := p.claim(t.Context())
	go func() { p.execute(t.Context(), claimed); close(done) }()
	<-started
	if err := p.Cancel(t.Context(), j.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	<-done
	stored, _ := p.Get(t.Context(), j.ID)
	if stored.Status != "cancelled" || stored.ResultID != "" {
		t.Fatalf("late success changed cancelled task: %+v", stored)
	}
	items, _ := p.s.List(t.Context(), "reports")
	if len(items) != 0 {
		t.Fatal("cancelled task left a report")
	}
}

func TestRunnerPanicBecomesFailure(t *testing.T) {
	p := testPool(t, func(context.Context, Job, func(string) error) (Result, error) { panic("private provider payload") })
	j := create(t, p, "panic")
	claimed, _ := p.claim(t.Context())
	p.execute(t.Context(), claimed)
	stored, _ := p.Get(t.Context(), j.ID)
	if stored.Status != "failed" || stored.ResultID != "" || stored.Error == "private provider payload" {
		t.Fatalf("panic was not safely contained: %+v", stored)
	}
}

func TestDurableEventsReplayBeyondPageAndAtomicCompletion(t *testing.T) {
	p := testPool(t, nil)
	j := create(t, p, "long-event-replay")
	claimed, _ := p.claim(t.Context())
	for i := 0; i < 430; i++ {
		if err := p.Emit(t.Context(), j.ID, "delta", map[string]string{"text": "a"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.finish(t.Context(), claimed, Result{Kind: "reports", Data: store.Entity{"summary": "verified"}}); err != nil {
		t.Fatal(err)
	}
	after, count, completed := 0, 0, 0
	for {
		page, err := p.Events(t.Context(), j.ID, after)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		for _, event := range page {
			if event.Seq != after+1 {
				t.Fatalf("gap or duplicate event: %d after %d", event.Seq, after)
			}
			after = event.Seq
			count++
			if event.Event == "completed" {
				completed++
			}
		}
	}
	if count != 434 || completed != 1 {
		t.Fatalf("got %d events and %d completed", count, completed)
	}
	stored, _ := p.Get(t.Context(), j.ID)
	if stored.Status != "succeeded" || stored.ResultID == "" {
		t.Fatalf("missing atomic result: %+v", stored)
	}
	if _, err := p.s.Get(t.Context(), "reports", stored.ResultID); err != nil {
		t.Fatalf("success has no report: %v", err)
	}
}

func TestRecoveryFailsInterruptedButRunsQueued(t *testing.T) {
	p := testPool(t, func(context.Context, Job, func(string) error) (Result, error) {
		return Result{Kind: "reports", Data: store.Entity{"summary": "queued continued"}}, nil
	})
	interrupted := create(t, p, "interrupted")
	if _, err := p.claim(t.Context()); err != nil {
		t.Fatal(err)
	}
	queued := create(t, p, "queued")
	p.Start(t.Context())
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		item, _ := p.Get(t.Context(), queued.ID)
		if Terminal(item.Status) {
			if item.Status != "succeeded" {
				t.Fatalf("queued recovery failed: %+v", item)
			}
			old, _ := p.Get(t.Context(), interrupted.ID)
			if old.Status != "failed" || old.ResultID != "" {
				t.Fatalf("paid request was replayed: %+v", old)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("queued job did not resume")
}
