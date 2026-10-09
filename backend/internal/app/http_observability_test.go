package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"zhiwei/backend/internal/observability"
	"zhiwei/backend/internal/store"
)

func TestHTTPObservationPreservesStreamingCorrelationAndPanicSafety(t *testing.T) {
	var requestID string
	handler := observeHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID = observability.RequestID(r.Context())
		if r.URL.Path == "/panic" {
			panic("do-not-publish-panic-value")
		}
		if _, ok := w.(http.Flusher); !ok {
			t.Fatal("Flusher lost")
		}
		if _, ok := w.(interface{ Unwrap() http.ResponseWriter }); !ok {
			t.Fatal("ResponseController unwrapping lost")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: progress\ndata: {\"code\":\"preparing\"}\n\n")
		w.(http.Flusher).Flush()
		panic("do-not-append-json-after-SSE")
	}))
	for _, path := range []string{"/events?secret=not-logged", "/panic"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("X-Request-ID", "untrusted-header")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if len(requestID) != 32 || w.Header().Get("X-Request-ID") != requestID {
			t.Fatal("server correlation not propagated")
		}
		body := w.Body.String()
		if strings.Contains(body, "do-not") {
			t.Fatal("panic leaked")
		}
		if strings.HasPrefix(path, "/events") {
			if !w.Flushed || w.Code != 200 || strings.Contains(body, "internal_error") {
				t.Fatalf("stream corrupted: %s", body)
			}
		} else if w.Code != 500 || !strings.Contains(body, "internal_error") {
			t.Fatal(body)
		}
	}
}
func TestRevisionCannotChangeOrInventDefectCoordinates(t *testing.T) {
	a, student, _, _ := setup(t)
	original := assessmentData()
	original["defects"] = []any{map[string]any{"name": "划痕", "bbox": map[string]any{"x": 0.1, "y": 0.2, "width": 0.3, "height": 0.4}}, map[string]any{"name": "假象"}}
	_ = scoreData(original, defaultRubric())
	rep, err := a.Store.Create(context.Background(), "reports", student.u.ID, "", "pending_review", store.Entity{"kind": "assessment", "data": original, "original_data": original, "rubric": defaultRubric(), "revision_no": 0, "sources": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(original)
	var revision map[string]any
	_ = json.Unmarshal(raw, &revision)
	defects := revision["defects"].([]any)
	defects[0].(map[string]any)["bbox"] = map[string]any{"x": 0.8}
	defects[1].(map[string]any)["bbox"] = map[string]any{"x": 0.2}
	code, saved := student.call("POST", "/api/v1/reports/"+rep.String("id")+"/revisions", map[string]any{"data": revision, "reason": "采用并说明", "base_version": 0})
	if code != 201 {
		t.Fatal(code, saved)
	}
	actual := saved["data"].(map[string]any)["defects"].([]any)
	if actual[0].(map[string]any)["bbox"].(map[string]any)["x"] != 0.1 {
		t.Fatal("original box overwritten")
	}
	if _, yes := actual[1].(map[string]any)["bbox"]; yes {
		t.Fatal("new box accepted")
	}
}
