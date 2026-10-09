package ai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

type rubricCriterion struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	MaxScore float64 `json:"max_score"`
}

var defaultCriteria = []rubricCriterion{
	{ID: "scratches", Label: "划痕", MaxScore: 25},
	{ID: "artifacts", Label: "假象", MaxScore: 25},
	{ID: "etching", Label: "侵蚀程度", MaxScore: 25},
	{ID: "clarity", Label: "组织清晰与正确度", MaxScore: 25},
}

func requestCriteria(req Request) ([]rubricCriterion, error) {
	var input struct {
		Rubric struct {
			Criteria []rubricCriterion `json:"criteria"`
		} `json:"rubric"`
	}
	if len(req.Input) > 0 && json.Unmarshal(req.Input, &input) != nil {
		return nil, ErrInvalidRequest
	}
	if len(input.Rubric.Criteria) == 0 {
		return append([]rubricCriterion(nil), defaultCriteria...), nil
	}
	criteria := input.Rubric.Criteria
	if len(criteria) != 4 {
		return nil, ErrInvalidRequest
	}
	seen := map[string]bool{}
	for _, item := range criteria {
		if item.ID == "" || seen[item.ID] || !validText(item.Label, 150) || item.MaxScore != 25 {
			return nil, ErrInvalidRequest
		}
		seen[item.ID] = true
	}
	return criteria, nil
}

// Validate validates and normalizes provider output. Scores, labels and totals
// returned to consumers are derived from the server's rule snapshot, never
// trusted from the provider. It is also useful for controlled-provider tests.
func Validate(req Request, raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) > maxResponseBytes {
		return nil, invalid("$", "response_too_large")
	}
	if rejectDuplicateKeys(raw) != nil {
		return nil, invalid("$", "invalid_json_or_duplicate_key")
	}
	switch req.Kind {
	case "assessment":
		return validateAssessment(req, raw)
	case "understanding", "identify":
		return validateUnderstanding(req, raw)
	case "coaching":
		return validateCoaching(req, raw)
	default:
		return nil, ErrInvalidRequest
	}
}

func validateAssessment(req Request, raw json.RawMessage) (json.RawMessage, error) {
	var report Assessment
	if err := decodeStrict(raw, &report); err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			return nil, ve
		}
		return nil, invalid("$", "schema_invalid")
	}
	if require(raw, "summary", "criteria", "defects", "limitations", "citations") != nil {
		return nil, invalid("$", "required_field_missing")
	}
	if !validText(report.Summary, 6000) {
		return nil, invalid("summary", "nonempty_text_required")
	}
	if !validStrings(report.Limitations, 20, 2000) {
		return nil, invalid("limitations", "invalid_array")
	}
	if len(report.Defects) > 30 || report.Defects == nil {
		return nil, invalid("defects", "invalid_array")
	}
	if !validCitations(req, report.Citations) {
		return nil, invalid("citations", "unknown_or_duplicate_source")
	}
	criteria, err := requestCriteria(req)
	if err != nil {
		return nil, err
	}
	if len(report.Criteria) != len(criteria) {
		return nil, invalid("criteria", "must_cover_four_rules")
	}
	var rawFields struct {
		Criteria []json.RawMessage            `json:"criteria"`
		Defects  []map[string]json.RawMessage `json:"defects"`
	}
	_ = json.Unmarshal(raw, &rawFields)
	byID := map[string]Criterion{}
	for i, item := range report.Criteria {
		if require(rawFields.Criteria[i], "id", "label", "score", "max_score", "evidence") != nil || !validText(item.Evidence, 6000) || byID[item.ID].ID != "" {
			return nil, invalid(fmt.Sprintf("criteria[%d]", i), "required_field_invalid_or_duplicate")
		}
		byID[item.ID] = item
	}
	total, observable := 0.0, 0.0
	maxScore := 0.0
	report.Criteria = nil
	for i, criterion := range criteria {
		item, ok := byID[criterion.ID]
		if !ok || item.MaxScore != criterion.MaxScore {
			return nil, invalid(fmt.Sprintf("criteria[%d]", i), "rule_id_or_max_score_mismatch")
		}
		item.Label = criterion.Label
		if item.Score != nil {
			if math.IsNaN(*item.Score) || math.IsInf(*item.Score, 0) || *item.Score < 0 || *item.Score > criterion.MaxScore {
				return nil, invalid(fmt.Sprintf("criteria[%d].score", i), "score_out_of_range")
			}
			total += *item.Score
			observable += criterion.MaxScore
		}
		maxScore += criterion.MaxScore
		report.Criteria = append(report.Criteria, item)
	}
	for i, defect := range report.Defects {
		if value, exists := rawFields.Defects[i]["bbox"]; exists && string(value) == "null" {
			return nil, invalid(fmt.Sprintf("defects[%d].bbox", i), "omit_or_supply_four_numbers")
		}
		if !validText(defect.Name, 200) || !validText(defect.Evidence, 6000) || !validText(defect.Suggestion, 3000) {
			return nil, invalid(fmt.Sprintf("defects[%d]", i), "nonempty_text_required")
		}
		if defect.Severity != "low" && defect.Severity != "medium" && defect.Severity != "high" && defect.Severity != "uncertain" {
			return nil, invalid(fmt.Sprintf("defects[%d].severity", i), "unknown_severity")
		}
		if b := defect.BBox; b != nil {
			if b.X < 0 || b.Y < 0 || b.Width <= 0 || b.Height <= 0 || b.X+b.Width > 1 || b.Y+b.Height > 1 {
				return nil, invalid(fmt.Sprintf("defects[%d].bbox", i), "outside_image_or_empty")
			}
		}
	}
	report.MaxScore = maxScore
	report.Coverage = observable / maxScore
	// Partial evidence has no comparable overall grade. The per-item scores
	// and coverage are preserved so an unobservable item is never a zero.
	if observable == maxScore {
		total = math.Round(total*100) / 100
		report.TotalScore = &total
	} else {
		report.TotalScore = nil
		report.Limitations = appendUnique(report.Limitations, "部分评分项无法从当前图像判断，不生成可比较总分；请补充图像或由教师复核。")
	}
	report.Limitations = sourceLimitations(req, report.Limitations)
	return marshalReport(report)
}

func validateUnderstanding(req Request, raw json.RawMessage) (json.RawMessage, error) {
	var report Understanding
	if decodeStrict(raw, &report) != nil || require(raw, "summary", "candidates", "dimensions", "questions", "limitations", "citations") != nil {
		return nil, invalid("$", "schema_invalid_or_required_field_missing")
	}
	if !validText(report.Summary, 6000) {
		return nil, invalid("summary", "nonempty_text_required")
	}
	if len(report.Candidates) < 1 || len(report.Candidates) > 5 {
		return nil, invalid("candidates", "one_to_five_required")
	}
	if !validStrings(report.Questions, 10, 2000) {
		return nil, invalid("questions", "invalid_array")
	}
	if !validStrings(report.Limitations, 20, 2000) {
		return nil, invalid("limitations", "invalid_array")
	}
	if !validCitations(req, report.Citations) {
		return nil, invalid("citations", "unknown_or_duplicate_source")
	}
	for i, candidate := range report.Candidates {
		if !validText(candidate.Name, 200) || !validText(candidate.Evidence, 6000) || !validText(candidate.Uncertainty, 3000) {
			return nil, invalid(fmt.Sprintf("candidates[%d]", i), "nonempty_text_required")
		}
	}
	names := map[string]bool{"成分": false, "工艺": false, "组织": false, "性能": false, "应用": false}
	if len(report.Dimensions) != len(names) {
		return nil, invalid("dimensions", "five_dimensions_required")
	}
	for i, dimension := range report.Dimensions {
		seen, ok := names[dimension.Name]
		if !ok || seen || !validText(dimension.Content, 6000) {
			return nil, invalid(fmt.Sprintf("dimensions[%d]", i), "name_or_content_invalid")
		}
		names[dimension.Name] = true
		if dimension.Basis != "known" && dimension.Basis != "observed" && dimension.Basis != "inferred" {
			return nil, invalid(fmt.Sprintf("dimensions[%d].basis", i), "unknown_basis")
		}
	}
	report.Limitations = sourceLimitations(req, report.Limitations)
	return marshalReport(report)
}

func validateCoaching(req Request, raw json.RawMessage) (json.RawMessage, error) {
	var report Coaching
	if decodeStrict(raw, &report) != nil || require(raw, "summary", "questions", "suggestions", "verification_plan", "limitations", "citations") != nil {
		return nil, invalid("$", "schema_invalid_or_required_field_missing")
	}
	if !validText(report.Summary, 6000) {
		return nil, invalid("summary", "nonempty_text_required")
	}
	if !validStrings(report.Questions, 10, 2000) || len(report.Questions) == 0 {
		return nil, invalid("questions", "nonempty_array_required")
	}
	if !validStrings(report.Suggestions, 15, 3000) {
		return nil, invalid("suggestions", "invalid_array")
	}
	if !validStrings(report.VerificationPlan, 15, 3000) || len(report.VerificationPlan) == 0 {
		return nil, invalid("verification_plan", "nonempty_array_required")
	}
	if !validStrings(report.Limitations, 20, 2000) {
		return nil, invalid("limitations", "invalid_array")
	}
	if !validCitations(req, report.Citations) {
		return nil, invalid("citations", "unknown_or_duplicate_source")
	}
	report.Limitations = sourceLimitations(req, report.Limitations)
	return marshalReport(report)
}

func sourceLimitations(req Request, limitations []string) []string {
	if len(req.Sources) == 0 {
		return appendUnique(limitations, "没有检索到匹配的已发布教学资料；本报告为模型学习参考，请核对教材或教师意见。")
	}
	return limitations
}

func validCitations(req Request, citations []string) bool {
	if citations == nil || len(citations) > len(req.Sources) {
		return false
	}
	allowed := map[string]bool{}
	for _, source := range req.Sources {
		allowed[source.ID] = true
	}
	seen := map[string]bool{}
	for _, citation := range citations {
		if !allowed[citation] || seen[citation] {
			return false
		}
		seen[citation] = true
	}
	return true
}

func appendUnique(list []string, item string) []string {
	for _, existing := range list {
		if existing == item {
			return list
		}
	}
	return append(list, item)
}

func validText(s string, max int) bool {
	return strings.TrimSpace(s) != "" && len([]rune(s)) <= max
}

func validStrings(ss []string, count, size int) bool {
	if ss == nil || len(ss) > count {
		return false
	}
	for _, s := range ss {
		if !validText(s, size) {
			return false
		}
	}
	return true
}

func marshalReport(v any) (json.RawMessage, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, ErrInvalidResponse
	}
	return data, nil
}

func decodeStrict(raw []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return ErrInvalidResponse
	}
	return nil
}

func require(raw []byte, keys ...string) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return ErrInvalidResponse
	}
	for _, key := range keys {
		if _, ok := obj[key]; !ok {
			return fmt.Errorf("%w: missing %s", ErrInvalidResponse, key)
		}
	}
	return nil
}

// JSON duplicate keys would otherwise be silently overwritten by encoding/json.
func rejectDuplicateKeys(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	var consume func(int) error
	consume = func(depth int) error {
		if depth > 32 {
			return ErrInvalidResponse
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				if e != nil {
					return e
				}
				s, ok := key.(string)
				if !ok || seen[s] {
					return ErrInvalidResponse
				}
				seen[s] = true
				if e := consume(depth + 1); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e := consume(depth + 1); e != nil {
					return e
				}
			}
		default:
			return ErrInvalidResponse
		}
		_, err = d.Token()
		return err
	}
	if err := consume(0); err != nil {
		return err
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return ErrInvalidResponse
	}
	return nil
}
