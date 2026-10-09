package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func validAssessment() string {
	return `{"summary":"可见少量划痕，建议下一轮观察同一区域。","criteria":[{"id":"scratches","label":"划痕","score":20,"max_score":25,"evidence":"可见线性痕迹"},{"id":"artifacts","label":"假象","score":23,"max_score":25,"evidence":"未观察到明显伪影"},{"id":"etching","label":"侵蚀程度","score":21,"max_score":25,"evidence":"局部对比度较低"},{"id":"clarity","label":"组织清晰与正确度","score":22,"max_score":25,"evidence":"边界基本可辨"}],"defects":[{"name":"划痕","severity":"low","evidence":"上部可见线性痕迹","suggestion":"复查抛光步骤","bbox":{"x":0.1,"y":0.1,"width":0.3,"height":0.3}}],"limitations":[],"citations":["source-1"]}`
}

func validUnderstanding() string {
	return `{"summary":"形貌支持候选但还需要工艺背景。","candidates":[{"name":"珠光体候选","evidence":"局部可见层片状形貌","uncertainty":"需要更高倍率图像确认"}],"dimensions":[{"name":"成分","content":"用户提供45钢","basis":"known"},{"name":"工艺","content":"工艺未知，不能据图确定","basis":"inferred"},{"name":"组织","content":"可見局部片层","basis":"observed"},{"name":"性能","content":"需要硬度实验","basis":"inferred"},{"name":"应用","content":"结合设计需求判断","basis":"inferred"}],"questions":["哪些图像特征支持判断？"],"limitations":["单图不能测量硬度"],"citations":["source-1"]}`
}

func sampleRequest(kind string) Request {
	return Request{
		Kind: kind, Input: json.RawMessage(`{"material":"45钢"}`),
		Images:  []Image{{MIME: "image/png", Data: []byte("real-image-content")}},
		Sources: []Source{{ID: "source-1", Title: "教学参考", Content: "观察形貌后结合工艺判断。"}},
	}
}

func writeStream(w http.ResponseWriter, content string, finish string, done bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	// Multibyte strings are transmitted as JSON string chunks. Split between
	// characters, not inside the wire representation.
	runes := []rune(content)
	for start := 0; start < len(runes); start += 31 {
		text := string(runes[start:min(start+31, len(runes))])
		b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": text}, "finish_reason": nil}}})
		fmt.Fprintf(w, "data: %s\n\n", b)
	}
	if finish != "" {
		fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":%q}]}\n\n", finish)
	}
	if done {
		fmt.Fprint(w, "data: [DONE]\n\n")
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	c, err := New(Config{BaseURL: server.URL + "/v1", APIKey: "test-only-key", Model: "test-vision", Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestGenerateSendsActualImageAndValidatedReport(t *testing.T) {
	req := sampleRequest("assessment")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-only-key" {
			t.Error("wrong endpoint or authorization")
		}
		body, _ := io.ReadAll(r.Body)
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string
				Content json.RawMessage
			} `json:"messages"`
			Stream bool `json:"stream"`
		}
		if json.Unmarshal(body, &request) != nil || !request.Stream || request.Model != "test-vision" {
			t.Error("invalid provider request")
		}
		if !strings.Contains(string(body), "task_kind: assessment") || !strings.Contains(string(body), "source-1") || !strings.Contains(string(body), "data:image/png;base64,"+base64.StdEncoding.EncodeToString(req.Images[0].Data)) {
			t.Error("missing actual image, kind or retrieved evidence")
		}
		writeStream(w, validAssessment(), "stop", true)
	}))
	defer server.Close()
	c := newTestClient(t, server)
	var updates []string
	data, err := c.Generate(context.Background(), req, func(s string) error { updates = append(updates, s); return nil })
	if err != nil {
		t.Fatal(err)
	}
	var report Assessment
	_ = json.Unmarshal(data, &report)
	if report.TotalScore == nil || *report.TotalScore != 86 || report.Coverage != 1 || report.MaxScore != 100 || calls.Load() != 1 {
		t.Fatalf("incorrect normalized assessment: %s", data)
	}
	if len(updates) < 2 || strings.Contains(strings.Join(updates, ""), "real-image-content") {
		t.Fatal("progress missing or raw image leaked")
	}
}

func TestValidationRejectsUntrustedClaimsAndSchema(t *testing.T) {
	tests := map[string]string{
		"score-over-max":          strings.Replace(validAssessment(), `"score":20`, `"score":26`, 1),
		"negative-score":          strings.Replace(validAssessment(), `"score":20`, `"score":-1`, 1),
		"wrong-max":               strings.Replace(validAssessment(), `"max_score":25`, `"max_score":30`, 1),
		"unknown-citation":        strings.Replace(validAssessment(), "source-1", "invented-source", 1),
		"duplicate-citation":      strings.Replace(validAssessment(), `["source-1"]`, `["source-1","source-1"]`, 1),
		"bbox-outside":            strings.Replace(validAssessment(), `"x":0.1`, `"x":0.9`, 1),
		"bbox-negative":           strings.Replace(validAssessment(), `"height":0.3`, `"height":-0.3`, 1),
		"bad-severity":            strings.Replace(validAssessment(), `"severity":"low"`, `"severity":"perfect"`, 1),
		"duplicate-criterion":     strings.Replace(validAssessment(), `"id":"etching"`, `"id":"scratches"`, 1),
		"missing-score":           strings.Replace(validAssessment(), `"score":20,`, ``, 1),
		"missing-citations":       strings.Replace(validAssessment(), `,"citations":["source-1"]`, ``, 1),
		"unknown-property":        strings.Replace(validAssessment(), `"summary":`, `"trust_me":true,"summary":`, 1),
		"nested-unknown-property": strings.Replace(validAssessment(), `"x":0.1`, `"pixels":123,"x":0.1`, 1),
		"duplicate-json-key":      strings.Replace(validAssessment(), `"score":20`, `"score":0,"score":20`, 1),
		"trailing-json":           validAssessment() + `{}`,
		"fenced-json":             "```json\n" + validAssessment() + "\n```",
		"nil-array":               strings.Replace(validAssessment(), `"limitations":[]`, `"limitations":null`, 1),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Validate(sampleRequest("assessment"), json.RawMessage(raw)); !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("wanted invalid response, got %v", err)
			}
		})
	}
}

func TestUnknownScoreHasNoFabricatedTotalAndNoSourceWarning(t *testing.T) {
	raw := strings.Replace(validAssessment(), `"score":20`, `"score":null`, 1)
	raw = strings.Replace(raw, `"citations":["source-1"]`, `"citations":[]`, 1)
	raw = strings.Replace(raw, `"summary":`, `"total_score":100,"max_score":999,"coverage":1,"summary":`, 1)
	req := sampleRequest("assessment")
	req.Sources = nil
	data, err := Validate(req, json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	var report Assessment
	_ = json.Unmarshal(data, &report)
	if report.TotalScore != nil || report.Criteria[0].Score != nil || report.MaxScore != 100 || report.Coverage != 0.75 || len(report.Limitations) != 2 {
		t.Fatalf("must preserve uncertainty: %s", data)
	}
}

func TestFiveDimensionsAndCoachingContract(t *testing.T) {
	for _, kind := range []string{"understanding", "identify"} {
		if _, err := Validate(sampleRequest(kind), json.RawMessage(validUnderstanding())); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{
		strings.Replace(validUnderstanding(), `"name":"应用"`, `"name":"组织"`, 1),
		strings.Replace(validUnderstanding(), `"basis":"known"`, `"basis":"measured"`, 1),
		strings.Replace(validUnderstanding(), `,"basis":"known"`, ``, 1),
		strings.Replace(validUnderstanding(), `"uncertainty":"需要更高倍率图像确认"`, `"uncertainty":""`, 1),
	} {
		if _, err := Validate(sampleRequest("understanding"), json.RawMessage(raw)); !errors.Is(err, ErrInvalidResponse) {
			t.Fatalf("invalid five-dimension report accepted: %s", raw)
		}
	}
	coaching := `{"summary":"复查磨抛","questions":["如何识别划痕？"],"suggestions":["保留前后图像"],"verification_plan":["同倍率复拍"],"limitations":["需教师指导"],"citations":[]}`
	if _, err := Validate(sampleRequest("coaching"), json.RawMessage(coaching)); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateRepairsAtMostOnce(t *testing.T) {
	for _, repairWorks := range []bool{true, false} {
		t.Run(fmt.Sprint(repairWorks), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if n == 1 || !repairWorks {
					writeStream(w, strings.Replace(validAssessment(), `"score":20`, `"score":30`, 1), "stop", true)
				} else {
					writeStream(w, validAssessment(), "stop", true)
				}
			}))
			defer server.Close()
			_, err := newTestClient(t, server).Generate(context.Background(), sampleRequest("assessment"), nil)
			if calls.Load() != 2 || (repairWorks && err != nil) || (!repairWorks && !errors.Is(err, ErrInvalidResponse)) {
				t.Fatalf("repair contract violated: calls %d, error %v", calls.Load(), err)
			}
		})
	}
}

func TestGenerateDoesNotInventResultsForProviderFailures(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    error
	}{
		{"eof", func(w http.ResponseWriter, _ *http.Request) { writeStream(w, validAssessment(), "", false) }, ErrTruncated},
		{"done-without-stop", func(w http.ResponseWriter, _ *http.Request) { writeStream(w, validAssessment(), "", true) }, ErrTruncated},
		{"length", func(w http.ResponseWriter, _ *http.Request) { writeStream(w, validAssessment(), "length", true) }, ErrTruncated},
		{"refusal", func(w http.ResponseWriter, _ *http.Request) { writeStream(w, "", "content_filter", true) }, ErrRefused},
		{"empty", func(w http.ResponseWriter, _ *http.Request) { writeStream(w, "", "stop", true) }, ErrInvalidResponse},
		{"unauthorized", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"error":"secret-token raw-private-image"}`)
		}, ErrProvider},
		{"upstream-error", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"error\":{\"message\":\"secret-token\"}}\n\n")
		}, ErrProvider},
		{"invalid-envelope", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "private prompt") }, ErrProvider},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); test.handler(w, r) }))
			defer server.Close()
			result, err := newTestClient(t, server).Generate(context.Background(), sampleRequest("assessment"), nil)
			if !errors.Is(err, test.want) || result != nil || calls.Load() != 1 {
				t.Fatalf("unexpected result: %s, %v, calls %d", result, err, calls.Load())
			}
			if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "private") {
				t.Fatal("provider content leaked")
			}
		})
	}
}

func TestNonStreamingCompatibilityAndRefusal(t *testing.T) {
	for _, refused := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			message := map[string]any{"content": validAssessment()}
			if refused {
				message["refusal"] = "cannot analyze"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": message, "finish_reason": "stop"}}})
		}))
		_, err := newTestClient(t, server).Generate(context.Background(), sampleRequest("assessment"), nil)
		server.Close()
		if (!refused && err != nil) || (refused && !errors.Is(err, ErrRefused)) {
			t.Fatal(err)
		}
	}
}

func TestCancellationTimeoutAndProgressFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()
	c := newTestClient(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Generate(ctx, sampleRequest("assessment"), nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	c.config.Timeout = 15 * time.Millisecond
	if _, err := c.Generate(context.Background(), sampleRequest("assessment"), nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	errSentinel := errors.New("cannot persist progress")
	if _, err := c.Generate(context.Background(), sampleRequest("assessment"), func(string) error { return errSentinel }); !errors.Is(err, errSentinel) {
		t.Fatal(err)
	}
}

func TestMissingConfigurationAndRedirectProtection(t *testing.T) {
	c, err := New(Config{})
	if err != nil || c.Configured() {
		t.Fatal("unconfigured state not preserved")
	}
	if _, err := c.Generate(context.Background(), sampleRequest("assessment"), nil); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
	if _, err := New(Config{BaseURL: "https://secret@example.test/v1", Model: "m"}); err == nil {
		t.Fatal("credential-in-url accepted")
	}
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	if _, err := newTestClient(t, redirect).Generate(context.Background(), sampleRequest("assessment"), nil); !errors.Is(err, ErrProvider) || targetCalls.Load() != 0 {
		t.Fatal("redirect followed", err)
	}
}
