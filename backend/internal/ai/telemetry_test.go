package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"zhiwei/backend/internal/observability"
)

func TestBoundingBoxRequiresEveryExplicitFiniteNumber(t *testing.T) {
	original := `{"x":0.1,"y":0.1,"width":0.3,"height":0.3}`
	for _, box := range []string{`{"y":0.1,"width":0.3,"height":0.3}`, `{"x":null,"y":0.1,"width":0.3,"height":0.3}`, `{"x":0,"y":null,"width":0.3,"height":0.3}`, `null`, `{"x":0.7,"y":0.1,"width":0.30000001,"height":0.3}`, `{"x":0,"y":0,"width":1,"height":0}`} {
		raw := strings.Replace(validAssessment(), original, box, 1)
		_, err := Validate(sampleRequest("assessment"), json.RawMessage(raw))
		var detail *ValidationError
		if !errors.Is(err, ErrInvalidResponse) || !errors.As(err, &detail) || !strings.Contains(detail.Path, "bbox") {
			t.Fatalf("bad box %s => %v %+v", box, err, detail)
		}
	}
	for _, raw := range []string{strings.Replace(validAssessment(), original, `{"x":0,"y":0,"width":1,"height":1}`, 1), strings.Replace(validAssessment(), `,"bbox":`+original, "", 1)} {
		if _, err := Validate(sampleRequest("assessment"), json.RawMessage(raw)); err != nil {
			t.Fatal(err)
		}
	}
	messages, _ := buildMessages(sampleRequest("assessment"))
	if !strings.Contains(messageText(messages), "原点在左上角") {
		t.Fatal("coordinate origin absent")
	}
}
func TestMeasuredCallRepairProgressUsageAndPrivateTerminalLogs(t *testing.T) {
	var logs bytes.Buffer
	old := slog.Default()
	t.Cleanup(func() { _ = observability.Configure(io.Discard, observability.Config{}); slog.SetDefault(old) })
	if err := observability.Configure(&logs, observability.Config{Level: "debug", Format: "json", AIContent: true}); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		raw := validAssessment()
		if n == 1 {
			raw = strings.Replace(raw, `"score":20`, `"score":30`, 1)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		time.Sleep(5 * time.Millisecond)
		chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": raw, "reasoning_content": "internal-hidden-reasoning"}, "finish_reason": nil}}})
		fmt.Fprintf(w, "data: %s\n\n", chunk)
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":34,\"total_tokens\":46}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	events := []observability.Progress{}
	ctx, run := observability.WithRun(context.Background(), "request-123", "job-123", time.Now(), func(p observability.Progress) error { events = append(events, p); return nil })
	var public []string
	_, err := newTestClient(t, server).Generate(ctx, sampleRequest("assessment"), func(s string) error { public = append(public, s); return nil })
	if err != nil {
		t.Fatal(err)
	}
	codes := []string{}
	seenStages := map[string]bool{}
	for _, event := range events {
		if !seenStages[event.StageID] {
			codes = append(codes, event.Code)
			seenStages[event.StageID] = true
		}
	}
	if got := strings.Join(codes, ","); got != "model_waiting,model_receiving,validating,repairing,model_waiting,model_receiving,validating" {
		t.Fatal(got)
	}
	timing := run.Timings()
	if len(timing.ModelCalls) != 2 || timing.ModelMS < 10 {
		t.Fatalf("%+v", timing)
	}
	for _, call := range timing.ModelCalls {
		if call.FirstContentMS == nil || *call.FirstContentMS < 5 || call.InputTokens == nil || *call.InputTokens != 12 || call.OutputTokens == nil || *call.OutputTokens != 34 || call.TotalTokens == nil || *call.TotalTokens != 46 {
			t.Fatalf("%+v", call)
		}
	}
	for _, secret := range []string{"test-only-key", "internal-hidden-reasoning", "data:image/png;base64", "real-image-content"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("private transport leaked: %s", secret)
		}
	}
	if strings.Contains(strings.Join(public, ""), "criteria") {
		t.Fatal("raw model content reached progress")
	}
	for _, required := range []string{`"direction":"request"`, `"direction":"response"`, `"path":"criteria[0].score"`, `"request_id":"request-123"`, `"job_id":"job-123"`, `"call_id":`, `"valid":false`, `"valid":true`} {
		if !strings.Contains(logs.String(), required) {
			t.Fatalf("missing log %s", required)
		}
	}
}
