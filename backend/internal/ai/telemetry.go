package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"zhiwei/backend/internal/observability"
)

type ValidationError struct{ Path, Reason string }

func (e *ValidationError) Error() string { return ErrInvalidResponse.Error() }
func (e *ValidationError) Unwrap() error { return ErrInvalidResponse }
func (e *ValidationError) Code() string  { return "ai_invalid_response" }
func invalid(path, reason string) error  { return &ValidationError{Path: path, Reason: reason} }
func diagnostic(err error) (string, string) {
	var v *ValidationError
	if errors.As(err, &v) {
		return v.Path, v.Reason
	}
	return "$", "schema_invalid"
}
func errorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.Canceled):
		return "interrupted"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrNotConfigured):
		return "ai_not_configured"
	case errors.Is(err, ErrInvalidRequest):
		return "ai_invalid_request"
	case errors.Is(err, ErrInvalidResponse):
		return "ai_invalid_response"
	case errors.Is(err, ErrTruncated):
		return "ai_truncated"
	case errors.Is(err, ErrRefused):
		return "ai_refused"
	case errors.Is(err, ErrProvider):
		return "ai_provider_error"
	}
	return "processing_failed"
}

type usage struct {
	PromptTokens     *int64 `json:"prompt_tokens"`
	CompletionTokens *int64 `json:"completion_tokens"`
	TotalTokens      *int64 `json:"total_tokens"`
}
type callStats struct {
	receivedBytes, receivedChunks int
	onDelta                       func(int, int, bool) error
	started                       time.Time
	first                         *int64
	usage                         *usage
	finishReason                  string
	onContent                     func() error
}

func (s *callStats) content() error {
	if s == nil || s.first != nil {
		return nil
	}
	ms := time.Since(s.started).Milliseconds()
	s.first = &ms
	if s.onContent != nil {
		return s.onContent()
	}
	return nil
}
func (s *callStats) observe(c completion) {
	if s == nil {
		return
	}
	if c.Usage != nil {
		s.usage = c.Usage
	}
	if len(c.Choices) == 1 && c.Choices[0].FinishReason != nil {
		switch value := *c.Choices[0].FinishReason; value {
		case "stop", "length", "content_filter", "tool_calls", "function_call":
			s.finishReason = value
		default:
			s.finishReason = "unknown"
		}
	}
}
func safeTokens(v *int64) *int64 {
	if v == nil || *v < 0 {
		return nil
	}
	return v
}
func (s *callStats) timing(attempt int) observability.CallTiming {
	v := observability.CallTiming{Attempt: attempt, DurationMS: time.Since(s.started).Milliseconds(), FirstContentMS: s.first}
	if s.usage != nil {
		v.InputTokens = safeTokens(s.usage.PromptTokens)
		v.OutputTokens = safeTokens(s.usage.CompletionTokens)
		v.TotalTokens = safeTokens(s.usage.TotalTokens)
	}
	return v
}
func messageText(messages []message) string {
	var out strings.Builder
	for _, m := range messages {
		fmt.Fprintf(&out, "[%s]\n", m.Role)
		switch content := m.Content.(type) {
		case string:
			out.WriteString(content)
		case []any:
			for _, p := range content {
				part, ok := p.(map[string]any)
				if !ok {
					continue
				}
				if part["type"] == "text" {
					text, _ := part["text"].(string)
					out.WriteString(text)
				} else if part["type"] == "image_url" {
					out.WriteString("\n[图片已隐藏]")
				}
			}
		}
		out.WriteString("\n")
	}
	return out.String()
}

// Bounding boxes are optional, but every supplied box has four explicit numbers.
func (b *BoundingBox) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 4 {
		return invalid("defects[].bbox", "four_numbers_required")
	}
	for _, key := range []string{"x", "y", "width", "height"} {
		var n float64
		v, ok := fields[key]
		if !ok || string(v) == "null" || json.Unmarshal(v, &n) != nil {
			return invalid("defects[].bbox."+key, "number_required")
		}
	}
	type plain BoundingBox
	var v plain
	if decodeStrict(raw, &v) != nil {
		return invalid("defects[].bbox", "object_invalid")
	}
	*b = BoundingBox(v)
	return nil
}

func optionalCount(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func (s *callStats) receive(text string, force bool) error {
	if s == nil {
		return nil
	}
	s.receivedBytes += len(text)
	if text != "" {
		s.receivedChunks++
	}
	if s.onDelta != nil {
		return s.onDelta(s.receivedBytes, s.receivedChunks, force)
	}
	return nil
}
