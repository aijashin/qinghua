package app

import (
	"net/http"
	"time"
	"zhiwei/backend/internal/observability"
)

type responseCapture struct {
	http.ResponseWriter
	status, bytes int
}

func (w *responseCapture) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *responseCapture) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, e := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, e
}
func (w *responseCapture) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *responseCapture) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}
func observeHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		id := observability.ID()
		r = r.WithContext(observability.WithRequest(r.Context(), id))
		w.Header().Set("X-Request-ID", id)
		capture := &responseCapture{ResponseWriter: w}
		defer func() {
			if recover() != nil {
				observability.Logger(r.Context()).Error("http.panic")
				if capture.status == 0 {
					fail(capture, 500, "internal_error", "服务暂时无法处理请求")
				}
			}
			status := capture.status
			if status == 0 {
				status = 200
			}
			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			observability.Logger(r.Context()).Info("http.completed", "method", r.Method, "route", route, "status", status, "duration_ms", time.Since(started).Milliseconds(), "response_bytes", capture.bytes)
		}()
		next.ServeHTTP(capture, r)
	})
}
