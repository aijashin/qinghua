package app

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"zhiwei/backend/internal/auth"
	"zhiwei/backend/internal/store"
)

// SeedSyntheticLearningData creates an explicitly labelled demo class and
// reports for UI walkthroughs. It is idempotent and never masquerades as real
// student evidence.
func SeedSyntheticLearningData(s *store.Store) error {
	ctx := context.Background()
	const classID = "synthetic-demo-class"
	const password = "DemoLearning2026!"
	if _, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO classes(id,name,created_at) VALUES(?,?,?)`, classID, "演示班级（合成数据）", store.Now()); err != nil {
		return err
	}
	teacher, err := ensureDemoUser(ctx, s, "demo_teacher", password, "演示教师", "teacher")
	if err != nil {
		return err
	}
	if _, err = s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO class_members VALUES(?,?)`, classID, teacher.ID); err != nil {
		return err
	}
	students := []struct {
		Username string
		Name     string
		Score    float64
	}{
		{"demo_2026001", "陈思远（演示）", 92},
		{"demo_2026002", "林若晴（演示）", 86},
		{"demo_2026003", "周启航（演示）", 79},
		{"demo_2026004", "许安然（演示）", 73},
		{"demo_2026005", "赵明哲（演示）", 68},
		{"demo_2026006", "王知微（演示）", 61},
		{"demo_2026007", "孙景行（演示）", 55},
		{"demo_2026008", "李沐阳（演示）", 47},
	}
	for index, row := range students {
		student, err := ensureDemoUser(ctx, s, row.Username, password, row.Name, "student")
		if err != nil {
			return err
		}
		if _, err = s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO class_members VALUES(?,?)`, classID, student.ID); err != nil {
			return err
		}
		var exists int
		if err = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM entities WHERE kind='reports' AND owner_id=? AND json_extract(data,'$.demo_seed_key')='teacher-score-demo-v1'`, student.ID).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		fileID, err := syntheticStudentImage(s, student.ID, index)
		if err != nil {
			return err
		}
		snapshot := store.Entity{
			"title": "工业纯铁多视场评图（合成演示）", "material": "工业纯铁", "process": "退火",
			"magnification": "500×", "etchant": "硝酸酒精溶液", "file_id": fileID,
			"file_ids": []string{fileID}, "image_url": "/api/v1/files/" + fileID,
			"image_urls": []string{"/api/v1/files/" + fileID}, "demo": true,
		}
		sample, err := s.Create(ctx, "samples", student.ID, classID, "active", snapshot)
		if err != nil {
			return err
		}
		data := syntheticAssessment(row.Score)
		_, err = s.Create(ctx, "reports", student.ID, classID, "pending_review", store.Entity{
			"demo": true, "demo_seed_key": "teacher-score-demo-v1", "kind": "assessment",
			"sample_id": sample["id"], "sample_version": 1, "sample_snapshot": sample,
			"job_id": "synthetic-demo", "data": data, "original_data": data,
			"sources": []any{}, "model": "synthetic/demo", "prompt_version": "synthetic-demo-v1",
			"revision_no": 0, "latest_revision": nil,
		})
		if err != nil {
			return err
		}
	}
	fmt.Println("已创建演示班级、1名演示教师、8名演示学生及合成评分记录。")
	fmt.Println("演示教师账号: demo_teacher；演示账号统一密码:", password)
	return nil
}

func ensureDemoUser(ctx context.Context, s *store.Store, username, password, name, role string) (auth.User, error) {
	var user auth.User
	err := s.DB.QueryRowContext(ctx, `SELECT id,username,display_name,role,disabled FROM users WHERE username=?`, username).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role, &user.Disabled)
	if err == nil {
		return user, nil
	}
	if err != sql.ErrNoRows {
		return user, err
	}
	return (auth.Service{DB: s.DB}).Create(ctx, username, password, name, role)
}

func syntheticStudentImage(s *store.Store, owner string, seed int) (string, error) {
	img := image.NewGray(image.Rect(0, 0, 640, 420))
	for y := 0; y < 420; y++ {
		for x := 0; x < 640; x++ {
			value := uint8(120 + (x*3+y*5+seed*17)%105)
			if ((x+seed*11)/70+(y+seed*7)/55)%3 == 0 {
				value = uint8(55 + (x+y+seed*13)%70)
			}
			img.SetGray(x, y, color.Gray{Y: value})
		}
	}
	var payload bytes.Buffer
	if err := png.Encode(&payload, img); err != nil {
		return "", err
	}
	id := store.ID()
	if err := os.WriteFile(filepath.Join(s.Dir, "files", id), payload.Bytes(), 0600); err != nil {
		return "", err
	}
	_, err := s.DB.Exec(`INSERT INTO files VALUES(?,?,?,?,?,?,?)`, id, owner, "synthetic-metallography-demo.png", "image/png", payload.Len(), id, store.Now())
	return id, err
}

func syntheticAssessment(score float64) map[string]any {
	labels := []string{"划痕", "假象", "侵蚀程度", "组织清晰与正确度"}
	ids := []string{"scratches", "artifacts", "etching", "clarity"}
	weights := []float64{0.25, 0.25, 0.25, 0.25}
	criteria := make([]any, 0, 4)
	for i := range ids {
		value := float64(int(score*weights[i]*10+0.5)) / 10
		criteria = append(criteria, map[string]any{
			"id": ids[i], "label": labels[i], "score": value, "max_score": 25.0,
			"evidence":  "合成演示记录：用于展示教师端排序与记录查看，不代表真实学生表现。",
			"rule_text": "按可见证据给出学习性反馈。",
		})
	}
	return map[string]any{
		"summary":  "合成教学演示报告，仅用于界面和排序功能展示。",
		"criteria": criteria, "defects": []any{}, "limitations": []any{"非真实学生、非真实实验结果"},
		"citations": []any{}, "total_score": score, "max_score": 100.0, "coverage": 1.0,
	}
}
