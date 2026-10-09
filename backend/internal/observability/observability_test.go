package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func configureTest(t *testing.T, b *bytes.Buffer, c Config) {
	t.Helper()
	old := slog.Default()
	cfg := current.Load()
	t.Cleanup(func() { slog.SetDefault(old); current.Store(cfg) })
	if err := Configure(b, c); err != nil {
		t.Fatal(err)
	}
}
func TestContentRequiresBothSwitchesAndRedacts(t *testing.T) {
	for _, c := range []Config{{Level: "info", AIContent: true}, {Level: "debug"}} {
		var b bytes.Buffer
		configureTest(t, &b, c)
		Content(context.Background(), "call", 1, "request", "private business text")
		if b.Len() != 0 {
			t.Fatal("content logged without both switches")
		}
	}
	var b bytes.Buffer
	configureTest(t, &b, Config{Level: "debug", Format: "json", AIContent: true, Secrets: []string{"operator-key"}})
	input := `operator-key {"api_key":"other-key","cookie":"session-value"} Bearer private-token data:image/png;base64,` + strings.Repeat("Q", 200) + "\n<think>hidden reasoning</think> final\x1b[31m"
	Content(WithRequest(context.Background(), "request"), "call", 1, "response", input)
	logged := b.String()
	for _, secret := range []string{"operator-key", "other-key", "session-value", "private-token", "hidden reasoning", strings.Repeat("Q", 160), "\\u001b"} {
		if strings.Contains(logged, secret) {
			t.Fatalf("secret/control leaked: %s", secret)
		}
	}
	if !strings.Contains(logged, "final") || !strings.Contains(logged, `"call_id":"call"`) {
		t.Fatal(logged)
	}
}
func TestContentUTF8BoundsAndPerJobBudget(t *testing.T) {
	var b bytes.Buffer
	configureTest(t, &b, Config{Level: "debug", Format: "json", AIContent: true})
	ctx, _ := WithRun(context.Background(), "r", "j", time.Time{}, nil)
	text := strings.Repeat("开头内容 ", 10000) + "END末尾"
	for i := 0; i < 8; i++ {
		Content(ctx, "call", i+1, "response", text)
	}
	sum := 0
	reconstructed := ""
	perCall := map[float64]int{}
	for _, line := range strings.Split(strings.TrimSpace(b.String()), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatal(err)
		}
		if rec["msg"] == "ai.content.omitted" {
			continue
		}
		chunk := rec["text"].(string)
		if !utf8.ValidString(chunk) || len(chunk) > 4<<10 {
			t.Fatal("invalid chunk boundary")
		}
		sum += len(chunk)
		reconstructed += chunk
		perCall[rec["attempt"].(float64)] += len(chunk)
		if rec["omitted_bytes"].(float64) <= 0 {
			t.Fatal("missing truncation metadata")
		}
	}
	if sum > 128<<10 {
		t.Fatal(sum)
	}
	for _, size := range perCall {
		if size > 32<<10 {
			t.Fatal(size)
		}
	}
	if !strings.Contains(reconstructed, "END末尾") {
		t.Fatal("tail was lost")
	}
}
func TestRunStageTimingsAreMeasuredAndUnknownIsRejected(t *testing.T) {
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer slog.SetDefault(old)
	events := []Progress{}
	ctx, run := WithRun(context.Background(), "request", "job", time.Now().Add(-time.Second), func(p Progress) error { events = append(events, p); return nil })
	if err := Stage(ctx, "preparing", 0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Millisecond)
	if err := Stage(ctx, "model_waiting", 1); err != nil {
		t.Fatal(err)
	}
	RecordCall(ctx, CallTiming{Attempt: 1, DurationMS: 2})
	timing := run.Timings()
	if len(events) != 2 || timing.QueueMS == nil || *timing.QueueMS < 1000 || timing.TotalMS < 3 || timing.ModelMS != 2 {
		t.Fatalf("%+v", timing)
	}
	if err := Stage(ctx, "made_up", 0); err == nil {
		t.Fatal("accepted imaginary stage")
	}
}

func TestTerminalContentIsReadableFramedAndBudgetExhaustionExplicit(t *testing.T) {
	var b bytes.Buffer
	configureTest(t, &b, Config{Level: "debug", Format: "text", AIContent: true})
	ctx, _ := WithRun(context.Background(), "request", "job", time.Time{}, nil)
	Content(ctx, "call", 1, "request", "第一行\n第二行 {\"内容\":\"保留可读文本\"}")
	logged := b.String()
	if !strings.Contains(logged, "│ 第一行\n│ 第二行") || !strings.Contains(logged, "┌─ ai.content") || !strings.Contains(logged, "└─ ai.content end") {
		t.Fatal(logged)
	}
	for i := 0; i < 7; i++ {
		Content(ctx, "call", i+1, "response", strings.Repeat("文字 ", 20000))
	}
	if !strings.Contains(b.String(), "ai.content.omitted") || !strings.Contains(b.String(), "job_content_budget_exhausted") {
		t.Fatal("budget omission was silent")
	}
}
