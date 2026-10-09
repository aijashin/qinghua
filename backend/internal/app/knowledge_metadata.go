package app

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"unicode"
	"zhiwei/backend/internal/ai"
	"zhiwei/backend/internal/knowledge"
	"zhiwei/backend/internal/store"
)

var knowledgeTagKeys = []string{"material_tags", "phase_tags", "process_tags"}

// normalizeKnowledgeTags validates administrator-supplied metadata. Tags are
// descriptive retrieval aids; they do not replace document text or citations.
func normalizeKnowledgeTags(input store.Entity) (store.Entity, error) {
	out := store.Entity{}
	for _, key := range knowledgeTagKeys {
		values := []string{}
		switch raw := input[key].(type) {
		case nil:
		case []string:
			values = raw
		case []any:
			for _, item := range raw {
				value, ok := item.(string)
				if !ok {
					return nil, errors.New("材料、组织、工艺标签必须是字符串数组")
				}
				values = append(values, value)
			}
		default:
			return nil, errors.New("材料、组织、工艺标签必须是字符串数组")
		}
		if len(values) > 12 {
			return nil, errors.New("每类标签最多12项")
		}
		cleaned := []string{}
		seen := map[string]bool{}
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" || len([]rune(value)) > 64 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
				return nil, errors.New("每个标签须为1至64字且不含控制字符")
			}
			folded := strings.ToLower(value)
			if !seen[folded] {
				cleaned = append(cleaned, value)
				seen[folded] = true
			}
		}
		out[key] = cleaned
	}
	return out, nil
}

func knowledgeTagText(input map[string]any) string {
	parts := []string{}
	for _, key := range knowledgeTagKeys {
		parts = append(parts, stringsArray(input[key])...)
	}
	return strings.Join(parts, " ")
}

func knowledgeTagFilters(values url.Values) (map[string]string, error) {
	out := map[string]string{}
	for _, key := range []string{"material_tag", "phase_tag", "process_tag"} {
		value := strings.TrimSpace(values.Get(key))
		if len([]rune(value)) > 64 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return nil, errors.New("标签筛选值最多64字且不含控制字符")
		}
		if value != "" {
			out[key] = value
		}
	}
	return out, nil
}

// retrieveWithTags uses only server-selected SQL field names and bound values.
// Different tag categories are ANDed. Each category is one exact tag, using
// case-insensitive comparison for ASCII text. Empty q permits tag-only lookup.
func (a *App) retrieveWithTags(ctx context.Context, text string, filters map[string]string) ([]ai.Source, error) {
	query := knowledge.FTSQuery(text)
	out := []ai.Source{}
	if query == "" && len(filters) == 0 {
		return out, nil
	}
	sqlText := `SELECT c.id,c.title,c.content FROM knowledge_chunks c JOIN entities d ON d.id=c.document_id`
	if query != "" {
		sqlText += ` JOIN knowledge_fts f ON f.chunk_id=c.id`
	}
	sqlText += ` WHERE d.kind='documents' AND d.status='published'`
	args := []any{}
	if query != "" {
		sqlText += ` AND knowledge_fts MATCH ?`
		args = append(args, query)
	}
	for _, pair := range [][2]string{{"material_tag", "material_tags"}, {"phase_tag", "phase_tags"}, {"process_tag", "process_tags"}} {
		if value := filters[pair[0]]; value != "" {
			sqlText += ` AND EXISTS(SELECT 1 FROM json_each(d.data,'$.` + pair[1] + `') tag WHERE lower(trim(tag.value))=lower(?))`
			args = append(args, value)
		}
	}
	if query != "" {
		sqlText += ` ORDER BY bm25(knowledge_fts),c.id LIMIT 6`
	} else {
		sqlText += ` ORDER BY d.created_at DESC,c.position,c.id LIMIT 6`
	}
	rows, err := a.Store.DB.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var source ai.Source
		if err := rows.Scan(&source.ID, &source.Title, &source.Content); err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	return out, rows.Err()
}
