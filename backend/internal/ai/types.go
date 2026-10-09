// Package ai is an independent, OpenAI-compatible multimodal adapter. It has
// no dependency on the application database or any external agent platform.
package ai

import (
	"encoding/json"
	"errors"
	"time"
)

const PromptVersion = "metallography-v4"

var (
	ErrNotConfigured   = errors.New("AI 尚未配置，请配置独立模型地址、模型名称及所需密钥")
	ErrInvalidRequest  = errors.New("AI 请求缺少有效图像、背景或任务类型")
	ErrInvalidResponse = errors.New("AI 返回的数据不符合教学报告结构，请重试或联系管理员")
	ErrTruncated       = errors.New("AI 响应未完整结束，没有生成报告，请重试")
	ErrRefused         = errors.New("模型未能完成本次分析，请补充清晰图像和背景后重试")
	ErrProvider        = errors.New("模型服务调用失败，请检查服务配置或稍后重试")
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

type Image struct {
	MIME  string `json:"mime"`
	Data  []byte `json:"-"`
	Label string `json:"label,omitempty"`
}

type Source struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Content         string   `json:"content"`
	Kind            string   `json:"source_kind,omitempty"`
	Version         int      `json:"source_version,omitempty"`
	DocumentID      string   `json:"document_id,omitempty"`
	DocumentVersion int      `json:"document_version,omitempty"`
	ImageURL        string   `json:"image_url,omitempty"`
	MaterialTags    []string `json:"material_tags,omitempty"`
	PhaseTags       []string `json:"phase_tags,omitempty"`
	ProcessTags     []string `json:"process_tags,omitempty"`
}

type Request struct {
	Kind    string          `json:"kind"`
	Input   json.RawMessage `json:"input"`
	Images  []Image         `json:"-"`
	Sources []Source        `json:"sources"`
}

type Criterion struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Score    *float64 `json:"score"`
	MaxScore float64  `json:"max_score"`
	Evidence string   `json:"evidence"`
}

type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type Defect struct {
	Name       string       `json:"name"`
	Severity   string       `json:"severity"`
	Evidence   string       `json:"evidence"`
	Suggestion string       `json:"suggestion"`
	BBox       *BoundingBox `json:"bbox,omitempty"`
}

type Assessment struct {
	Summary     string      `json:"summary"`
	Criteria    []Criterion `json:"criteria"`
	Defects     []Defect    `json:"defects"`
	Limitations []string    `json:"limitations"`
	Citations   []string    `json:"citations"`
	TotalScore  *float64    `json:"total_score"`
	MaxScore    float64     `json:"max_score"`
	Coverage    float64     `json:"coverage"`
}

type Candidate struct {
	Name        string `json:"name"`
	Evidence    string `json:"evidence"`
	Uncertainty string `json:"uncertainty"`
}

type Dimension struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Basis   string `json:"basis"`
}

type Understanding struct {
	Summary     string      `json:"summary"`
	Candidates  []Candidate `json:"candidates"`
	Dimensions  []Dimension `json:"dimensions"`
	Questions   []string    `json:"questions"`
	Limitations []string    `json:"limitations"`
	Citations   []string    `json:"citations"`
}

type Coaching struct {
	Summary          string   `json:"summary"`
	Questions        []string `json:"questions"`
	Suggestions      []string `json:"suggestions"`
	VerificationPlan []string `json:"verification_plan"`
	Limitations      []string `json:"limitations"`
	Citations        []string `json:"citations"`
}
