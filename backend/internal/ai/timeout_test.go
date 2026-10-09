package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRepairHasIndependentCallBudget(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		time.Sleep(160 * time.Millisecond)
		content := validUnderstanding()
		if n == 1 {
			content = `{"summary":"需要格式修复"}`
		}
		writeStream(w, content, "stop", true)
	}))
	defer server.Close()
	c, _ := New(Config{BaseURL: server.URL, Model: "fixture", Timeout: 280 * time.Millisecond})
	if _, err := c.Generate(context.Background(), sampleRequest("understanding"), nil); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("repair request count", calls.Load())
	}
}

func TestTimeoutIsReadableAndDoesNotRetryTransport(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	c, _ := New(Config{BaseURL: server.URL, Model: "fixture", Timeout: 60 * time.Millisecond})
	_, err := c.Generate(context.Background(), sampleRequest("understanding"), nil)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "未生成新报告") || strings.Contains(err.Error(), "context deadline") {
		t.Fatal(err)
	}
	if errorCode(err) != "timeout" || calls.Load() != 1 {
		t.Fatal("timeout retried or mislabeled", calls.Load(), err)
	}
}

func TestResponseHeaderBudgetMatchesConfiguredTimeout(t *testing.T) {
	c, _ := New(Config{Timeout: 5 * time.Minute})
	if c.http.Transport.(*http.Transport).ResponseHeaderTimeout != 5*time.Minute {
		t.Fatal("hidden 60s deadline remains")
	}
}
