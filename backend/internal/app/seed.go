package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"zhiwei/backend/internal/knowledge"
	"zhiwei/backend/internal/store"
)

func SeedDemo(s *store.Store) error {
	ctx := context.Background()
	all, e := s.List(ctx, "atlas")
	if e != nil {
		return e
	}
	for _, v := range all {
		if d, _ := v["demo"].(bool); d {
			fmt.Println("示例资料已存在，不重复导入")
			return nil
		}
	}
	rub := defaultRubric()
	delete(rub, "id")
	if _, e = s.Create(ctx, "rubrics", "system", "", "published", rub); e != nil {
		return e
	}
	img := image.NewRGBA(image.Rect(0, 0, 512, 384))
	for y := 0; y < 384; y++ {
		for x := 0; x < 512; x++ {
			v := uint8(185 + (x*3+y*7)%45)
			if (x/38+y/45)%3 == 0 {
				v = 90 + uint8((x+y)%60)
			}
			img.Set(x, y, color.RGBA{v, v, v, 255})
		}
	}
	var b bytes.Buffer
	if e = png.Encode(&b, img); e != nil {
		return e
	}
	id := store.ID()
	if e = os.WriteFile(filepath.Join(s.Dir, "files", id), b.Bytes(), 0600); e != nil {
		return e
	}
	if _, e = s.DB.ExecContext(ctx, `INSERT INTO files VALUES(?,?,?,?,?,?,?)`, id, "system", "synthetic-teaching-pattern.png", "image/png", b.Len(), id, store.Now()); e != nil {
		return e
	}
	if _, e = s.Create(ctx, "atlas", "system", "", "published", store.Entity{"title": "教学纹理示意（合成图）", "material": "未指定", "phase": "纹理示意，不能据此定相", "process": "未指定", "description": "程序生成的合成示意图，仅用于页面与操作演示，不是真实金相图像。", "observation": "比较明暗和纹理；不用于评分基准。", "file_id": id, "image_url": "/api/v1/files/" + id, "source": "本项目程序生成；synthetic/demo", "demo": true}); e != nil {
		return e
	}
	text := "教学示例资料：观察金相图像时，应先记录材料、制备工艺、倍率和腐蚀剂。划痕、假象、侵蚀程度及组织清晰度可以作为教学观察维度。信息不足时保留未知，不应凭单幅图片直接断言材料性能。区分珠光体与贝氏体需要综合形貌与材料、热处理背景；这段资料仅示范知识库流程，须由教师审核替换为课程资料。"
	doc, e := s.Create(ctx, "documents", "system", "", "published", store.Entity{"title": "教学观察说明（示例，非官方规范）", "content": text, "demo": true, "chunk_count": 1})
	if e != nil {
		return e
	}
	chunk := store.ID()
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `INSERT INTO knowledge_chunks VALUES(?,?,?,?,?)`, chunk, doc.String("id"), doc.String("title"), text, 0); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO knowledge_fts(chunk_id,tokens) VALUES(?,?)`, chunk, knowledge.Tokenize(text)); e != nil {
		return e
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	fmt.Println("已导入合成示意图、教学示例规则与示例资料；未创建成绩或AI报告。")
	return nil
}
