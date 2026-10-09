package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

// Both ordinary records and multiline content use this shared write lock.
// A content part is one atomic write, with every untrusted line visibly prefixed.
type lockedWriter struct {
	mu  sync.Mutex
	out io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.out.Write(p)
}

type terminalHandler struct {
	base  slog.Handler
	out   *lockedWriter
	attrs []slog.Attr
}

func newTerminalHandler(out io.Writer, options *slog.HandlerOptions) slog.Handler {
	w := &lockedWriter{out: out}
	return &terminalHandler{base: slog.NewTextHandler(w, options), out: w}
}
func (h *terminalHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.base.Enabled(ctx, l)
}
func (h *terminalHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &terminalHandler{base: h.base.WithAttrs(attrs), out: h.out, attrs: append(append([]slog.Attr(nil), h.attrs...), attrs...)}
}
func (h *terminalHandler) WithGroup(name string) slog.Handler {
	return &terminalHandler{base: h.base.WithGroup(name), out: h.out, attrs: append([]slog.Attr(nil), h.attrs...)}
}
func (h *terminalHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Message != "ai.content" {
		return h.base.Handle(ctx, r)
	}
	values := map[string]slog.Value{}
	for _, a := range h.attrs {
		values[a.Key] = a.Value.Resolve()
	}
	r.Attrs(func(a slog.Attr) bool { values[a.Key] = a.Value.Resolve(); return true })
	var b strings.Builder
	fmt.Fprintf(&b, "%s DEBUG ┌─ ai.content", r.Time.Format("2006-01-02T15:04:05.000Z07:00"))
	for _, key := range []string{"request_id", "job_id", "call_id", "attempt", "direction", "part", "original_bytes", "omitted_bytes"} {
		if value, ok := values[key]; ok {
			fmt.Fprintf(&b, " %s=%q", key, value.String())
		}
	}
	b.WriteString("\n")
	for _, line := range strings.Split(values["text"].String(), "\n") {
		b.WriteString("│ ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("└─ ai.content end\n")
	_, err := h.out.Write([]byte(b.String()))
	return err
}
