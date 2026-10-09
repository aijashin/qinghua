package ai

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"time"
	"zhiwei/backend/internal/observability"
)

const (
	maxResponseBytes = 1 << 20
	maxWireBytes     = 8 << 20
	maxInputBytes    = 128 << 10
	maxImageBytes    = 20 << 20
)

type Client struct {
	config   Config
	endpoint string
	http     *http.Client
}

func New(config Config) (*Client, error) {
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	config.Model = strings.TrimSpace(config.Model)
	config.APIKey = strings.TrimSpace(config.APIKey)
	if config.Timeout <= 0 {
		config.Timeout = 300 * time.Second
	}
	c := &Client{config: config}
	if config.BaseURL != "" {
		u, err := url.Parse(config.BaseURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, errors.New("AI 地址必须是有效的 HTTP(S) API 地址，不得包含用户名、查询参数或片段")
		}
		path := strings.TrimRight(u.Path, "/")
		if path == "" {
			path = "/v1"
		}
		if !strings.HasSuffix(path, "/chat/completions") {
			path += "/chat/completions"
		}
		u.Path = path
		u.RawPath = ""
		c.endpoint = u.String()
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = config.Timeout
	c.http = &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
		// Never forward credentials through redirects to another endpoint.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	return c, nil
}

// Configured permits a keyless local compatible endpoint. Hosted endpoints
// which require a key return an explicit provider error when it is missing.
func (c *Client) Configured() bool {
	return c != nil && c.endpoint != "" && c.config.Model != ""
}

func (c *Client) Model() string {
	if c == nil {
		return ""
	}
	return c.config.Model
}

// Generate emits safe progress text, not unvalidated provider JSON. The final
// return value is the only content suitable for publication as a report.
// Exactly one repair request is allowed for structurally invalid output;
// refusals, cancellation, transport failures and truncation are not retried.
func (c *Client) Generate(ctx context.Context, req Request, progress func(string) error) (output json.RawMessage, resultErr error) {
	defer func() {
		if resultErr != nil {
			var timeout *TimeoutError
			if errors.Is(resultErr, context.DeadlineExceeded) && !errors.As(resultErr, &timeout) {
				resultErr = &TimeoutError{Limit: 2 * c.config.Timeout}
			}
			resultErr = observability.Coded(errorCode(resultErr), resultErr)
		}
	}()
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	if err := validateRequest(req); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*c.config.Timeout)
	defer cancel()
	if err := emit(progress, "正在读取图像与已发布教学资料"); err != nil {
		return nil, err
	}
	imageBytes := 0
	imageMIMEs := []string{}
	for _, image := range req.Images {
		imageBytes += len(image.Data)
		imageMIMEs = append(imageMIMEs, image.MIME)
	}
	observability.Logger(ctx).Info("ai.request.prepared", "kind", req.Kind, "image_count", len(req.Images), "image_bytes", imageBytes, "image_mimes", imageMIMEs, "source_count", len(req.Sources), "prompt_version", PromptVersion)
	messages, err := buildMessages(req)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := observability.Stage(ctx, "model_waiting", attempt+1); err != nil {
			return nil, err
		}
		callID := observability.ID()
		raw, err := c.complete(ctx, messages, progress, attempt+1, callID)
		if err != nil {
			return nil, err
		}
		if err := observability.Stage(ctx, "validating", attempt+1); err != nil {
			return nil, err
		}
		validationStarted := time.Now()
		validated, err := Validate(req, raw)
		checks := []observability.CheckInfo{}
		for _, name := range []string{"报告字段与结构", "分数与评分项目", "标注坐标范围", "引用来源"} {
			status := "passed"
			if err != nil {
				status = "not_checked"
				path, _ := diagnostic(err)
				target := "报告字段与结构"
				if strings.Contains(path, "bbox") {
					target = "标注坐标范围"
				} else if strings.Contains(path, "criteria") {
					target = "分数与评分项目"
				} else if strings.Contains(path, "citations") {
					target = "引用来源"
				}
				if name == target {
					status = "failed"
				}
			}
			if name == "分数与评分项目" && req.Kind != "assessment" {
				status = "not_applicable"
			}
			checks = append(checks, observability.CheckInfo{Name: name, Status: status})
		}
		if updateErr := observability.Update(ctx, observability.Details{Checks: checks}, true); updateErr != nil {
			return nil, updateErr
		}
		if err != nil {
			path, reason := diagnostic(err)
			observability.Logger(ctx).Debug("ai.validation", "call_id", callID, "attempt", attempt+1, "valid", false, "path", path, "reason", reason, "duration_ms", time.Since(validationStarted).Milliseconds())
		} else {
			observability.Logger(ctx).Debug("ai.validation", "call_id", callID, "attempt", attempt+1, "valid", true, "duration_ms", time.Since(validationStarted).Milliseconds())
		}
		if err == nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := emit(progress, "报告结构、评分范围与引用已校验"); err != nil {
				return nil, err
			}
			return validated, nil
		}
		if !errors.Is(err, ErrInvalidResponse) || attempt == 1 {
			return nil, err
		}
		if err := observability.Stage(ctx, "repairing", attempt+1); err != nil {
			return nil, err
		}
		observability.Logger(ctx).Info("ai.repair", "attempt", attempt+1, "next_attempt", attempt+2)
		if err := emit(progress, "报告格式不符合约定，正在进行一次结构修复"); err != nil {
			return nil, err
		}
		// The invalid answer is data, not instructions. It is deliberately
		// separated from the system contract; repair has the same image and
		// source evidence, and no additional tool permissions.
		messages = append(messages,
			message{Role: "assistant", Content: string(raw)},
			message{Role: "user", Content: repairMessage(req.Kind, err)},
		)
	}
	return nil, ErrInvalidResponse
}

func validateRequest(req Request) error {
	switch req.Kind {
	case "assessment", "understanding", "identify", "coaching":
	default:
		return ErrInvalidRequest
	}
	if len(req.Input) > maxInputBytes || (len(req.Input) > 0 && !json.Valid(req.Input)) || len(req.Images) == 0 || len(req.Images) > 4 || len(req.Sources) > 20 {
		return ErrInvalidRequest
	}
	if req.Kind == "assessment" {
		if _, err := requestCriteria(req); err != nil {
			return err
		}
	}
	for _, image := range req.Images {
		if len(image.Data) == 0 || len(image.Data) > maxImageBytes {
			return ErrInvalidRequest
		}
		if image.MIME != "image/jpeg" && image.MIME != "image/png" && image.MIME != "image/webp" {
			return ErrInvalidRequest
		}
	}
	seen := map[string]bool{}
	for _, source := range req.Sources {
		if !validText(source.ID, 200) || seen[source.ID] || !validText(source.Title, 500) || !validText(source.Content, 12000) {
			return ErrInvalidRequest
		}
		seen[source.ID] = true
	}
	return nil
}

type message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

func (c *Client) complete(ctx context.Context, messages []message, progress func(string) error, attempt int, callID string) (output json.RawMessage, resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()
	started := time.Now()
	responseAt := ""
	if err := observability.Update(ctx, observability.Details{SentAt: started.UTC().Format(time.RFC3339Nano), TimeoutMS: c.config.Timeout.Milliseconds()}, true); err != nil {
		return nil, err
	}
	observability.BeginCall(ctx, attempt, started)
	stats := &callStats{started: started, onContent: func() error {
		observability.FirstContent(ctx, attempt, time.Since(started).Milliseconds())
		if err := observability.Stage(ctx, "model_receiving", attempt); err != nil {
			return err
		}
		ms := time.Since(started).Milliseconds()
		return observability.Update(ctx, observability.Details{FirstContentMS: &ms, SentAt: started.UTC().Format(time.RFC3339Nano), ResponseAt: responseAt}, true)
	}}
	stats.onDelta = func(bytes, chunks int, force bool) error {
		return observability.Update(ctx, observability.Details{ReceivedBytes: &bytes, ReceivedChunks: &chunks}, force)
	}
	logger := observability.Logger(ctx).With("call_id", callID, "attempt", attempt)
	endpoint, _ := url.Parse(c.endpoint)
	logger.Info("ai.call.started", "provider_host", observability.Redact(endpoint.Host, c.config.APIKey), "provider_path", observability.Redact(endpoint.Path, c.config.APIKey), "model", observability.Redact(c.config.Model, c.config.APIKey))
	observability.Content(ctx, callID, attempt, "request", messageText(messages), c.config.APIKey)
	defer func() {
		timing := stats.timing(attempt)
		observability.RecordCall(ctx, timing)
		if resultErr == nil {
			if err := observability.Update(ctx, observability.Details{Call: &timing, ReceivedBytes: &stats.receivedBytes, ReceivedChunks: &stats.receivedChunks}, true); err != nil {
				resultErr = err
			}
		}
		logger.Info("ai.call.finished", "duration_ms", timing.DurationMS, "first_content_ms", optionalCount(timing.FirstContentMS), "finish_reason", stats.finishReason, "error_code", errorCode(resultErr), "input_tokens", optionalCount(timing.InputTokens), "output_tokens", optionalCount(timing.OutputTokens), "total_tokens", optionalCount(timing.TotalTokens))
		if len(output) > 0 {
			observability.Content(ctx, callID, attempt, "response", string(output), c.config.APIKey)
		}
	}()
	if logger.Enabled(ctx, slog.LevelDebug) {
		ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
			TLSHandshakeStart: func() { logger.Debug("ai.transport", "phase", "tls_started") }, TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
				logger.Debug("ai.transport", "phase", "tls_finished", "success", err == nil)
			},
			DNSStart: func(httptrace.DNSStartInfo) { logger.Debug("ai.transport", "phase", "dns_started") }, DNSDone: func(v httptrace.DNSDoneInfo) {
				logger.Debug("ai.transport", "phase", "dns_finished", "success", v.Err == nil)
			},
			ConnectStart: func(string, string) { logger.Debug("ai.transport", "phase", "connect_started") }, ConnectDone: func(_, _ string, e error) {
				logger.Debug("ai.transport", "phase", "connect_finished", "success", e == nil)
			},
			GotConn: func(v httptrace.GotConnInfo) {
				logger.Debug("ai.transport", "phase", "connection_ready", "reused", v.Reused)
			}, WroteRequest: func(v httptrace.WroteRequestInfo) {
				logger.Debug("ai.transport", "phase", "request_written", "success", v.Err == nil)
			}, GotFirstResponseByte: func() {
				logger.Debug("ai.transport", "phase", "first_response_byte", "elapsed_ms", time.Since(started).Milliseconds())
			},
		})
	}
	body, err := json.Marshal(struct {
		Model          string            `json:"model"`
		Messages       []message         `json:"messages"`
		Stream         bool              `json:"stream"`
		ResponseFormat map[string]string `json:"response_format"`
	}{Model: c.config.Model, Messages: messages, Stream: true, ResponseFormat: map[string]string{"type": "json_object"}})
	if err != nil {
		return nil, ErrInvalidRequest
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, ErrInvalidRequest
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream, application/json")
	if c.config.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, &TimeoutError{Limit: c.config.Timeout}
			}
			return nil, ctx.Err()
		}
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			return nil, &TimeoutError{Limit: c.config.Timeout, Connection: true}
		}
		return nil, ErrProvider
	}
	defer resp.Body.Close()
	responseAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := observability.Update(ctx, observability.Details{ResponseAt: responseAt}, true); err != nil {
		return nil, err
	}
	logger.Info("ai.http.response", "status", resp.StatusCode, "elapsed_ms", time.Since(started).Milliseconds())
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Provider responses can echo credentials, full prompts and image
		// URLs. Never include an upstream response body in a user error.
		return nil, fmt.Errorf("%w (HTTP %d)", ErrProvider, resp.StatusCode)
	}
	var raw json.RawMessage
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		raw, err = readStream(resp.Body, progress, stats)
	} else {
		raw, err = readCompletion(resp.Body, progress, stats)
	}
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, &TimeoutError{Limit: c.config.Timeout}
		}
		return nil, ctx.Err()
	}
	return raw, err
}

type completion struct {
	Choices []struct {
		Index        int     `json:"index"`
		Delta        part    `json:"delta"`
		Message      part    `json:"message"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Error json.RawMessage `json:"error"`
	Usage *usage          `json:"usage"`
}

type part struct {
	Content   string          `json:"content"`
	Refusal   string          `json:"refusal"`
	ToolCalls json.RawMessage `json:"tool_calls"`
}

func readCompletion(reader io.Reader, progress func(string) error, stats *callStats) (json.RawMessage, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxWireBytes+1))
	if err != nil {
		return nil, ErrProvider
	}
	if len(data) > maxWireBytes {
		return nil, ErrTruncated
	}
	var response completion
	if json.Unmarshal(data, &response) != nil {
		return nil, ErrProvider
	}
	stats.observe(response)
	if nonNull(response.Error) {
		return nil, ErrProvider
	}
	if len(response.Choices) != 1 {
		return nil, ErrInvalidResponse
	}
	choice := response.Choices[0]
	if choice.Message.Refusal != "" || nonEmptyArray(choice.Message.ToolCalls) {
		return nil, ErrRefused
	}
	if err := finishError(choice.FinishReason); err != nil {
		return nil, err
	}
	if strings.TrimSpace(choice.Message.Content) == "" {
		return nil, ErrInvalidResponse
	}
	if len(choice.Message.Content) > maxResponseBytes {
		return nil, ErrTruncated
	}
	if err := stats.content(); err != nil {
		return nil, err
	}
	if err := stats.receive(choice.Message.Content, true); err != nil {
		return nil, err
	}
	if err := emit(progress, "模型已返回图像分析，正在校验"); err != nil {
		return nil, err
	}
	return json.RawMessage(choice.Message.Content), nil
}

func readStream(reader io.Reader, progress func(string) error, stats *callStats) (json.RawMessage, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, maxWireBytes+1))
	scanner.Buffer(make([]byte, 8192), maxResponseBytes+65536)
	var content strings.Builder
	var eventData []string
	finished, done, announced := false, false, false
	process := func() error {
		if len(eventData) == 0 {
			return nil
		}
		data := strings.Join(eventData, "\n")
		eventData = nil
		if strings.TrimSpace(data) == "[DONE]" {
			done = true
			return nil
		}
		var event completion
		if json.Unmarshal([]byte(data), &event) != nil || nonNull(event.Error) {
			return ErrProvider
		}
		stats.observe(event)
		// Usage-only events may have an empty choices array.
		if len(event.Choices) == 0 {
			return nil
		}
		if len(event.Choices) != 1 || event.Choices[0].Index != 0 {
			return ErrInvalidResponse
		}
		choice := event.Choices[0]
		if choice.Delta.Refusal != "" || nonEmptyArray(choice.Delta.ToolCalls) {
			return ErrRefused
		}
		if finished && choice.Delta.Content != "" {
			return ErrInvalidResponse
		}
		if choice.Delta.Content != "" {
			if content.Len()+len(choice.Delta.Content) > maxResponseBytes {
				return ErrTruncated
			}
			content.WriteString(choice.Delta.Content)
			if !announced {
				if err := stats.content(); err != nil {
					return err
				}
				announced = true
				if err := emit(progress, "模型正在分析图像与资料"); err != nil {
					return err
				}
			}
		}
		if choice.Delta.Content != "" {
			if err := stats.receive(choice.Delta.Content, false); err != nil {
				return err
			}
		}
		if choice.FinishReason != nil {
			if err := finishError(choice.FinishReason); err != nil {
				return err
			}
			finished = true
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := process(); err != nil {
				return nil, err
			}
			if done {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			eventData = append(eventData, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if scanner.Err() != nil {
		return nil, ErrTruncated
	}
	if err := process(); err != nil {
		return nil, err
	}
	// [DONE] without a stop finish_reason or bare EOF is not successful.
	if !finished {
		return nil, ErrTruncated
	}
	if strings.TrimSpace(content.String()) == "" {
		return nil, ErrInvalidResponse
	}
	if err := stats.receive("", true); err != nil {
		return nil, err
	}
	return json.RawMessage(content.String()), nil
}

func finishError(reason *string) error {
	if reason == nil {
		return ErrTruncated
	}
	switch *reason {
	case "stop":
		return nil
	case "content_filter", "tool_calls", "function_call":
		return ErrRefused
	default:
		return ErrTruncated
	}
}

func nonNull(raw json.RawMessage) bool {
	return len(raw) != 0 && string(raw) != "null"
}

func nonEmptyArray(raw json.RawMessage) bool {
	return nonNull(raw) && string(raw) != "[]"
}

func emit(progress func(string) error, text string) error {
	if progress == nil {
		return nil
	}
	return progress(text)
}
