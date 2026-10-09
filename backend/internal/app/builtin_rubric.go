package app

import (
	"context"
	"encoding/json"
	"zhiwei/backend/internal/store"
)

const builtinRubricID = "metallography-learning-v1"

func builtinRubric() store.Entity {
	texts := []string{
		"观察磨痕和抛光划痕的粗细、数量、方向、延伸及覆盖范围。良好：没有可见划痕或仅有不影响观察的极细痕迹；轻微：局部细划痕但组织仍可辨；明显：较粗、较密或贯穿视场的划痕遮挡部分组织；严重：大量粗深划痕妨碍组织识别。根据可见证据和影响程度在0至25分内建议分数，不能根据推测的磨抛操作扣分；无法观察时留空并说明原因。",
		"观察污物、水迹、拖尾、拉出、边缘形变等制样假象，避免将真实组织当作缺陷。良好：没有可见假象；轻微：少量局部假象且不影响识别；明显：假象遮挡局部组织或干扰判断；严重：假象广泛存在，无法可靠辨识组织。根据可见证据和识别影响在0至25分内建议分数；不能确认是假象时说明不确定性，证据不足则留空。",
		"观察腐蚀对比、均匀性和细节保留。良好：对比适当、腐蚀均匀、组织细节清楚；轻微：局部对比偏弱或偏强，仍可辨识；明显：欠腐蚀、过腐蚀或不均匀腐蚀造成部分细节损失；严重：主要组织细节难以观察。根据可见影响在0至25分内建议分数；未知腐蚀剂不能被自动扣分，也不能仅凭明暗断言腐蚀工艺错误；无法判断时留空。",
		"结合焦点、照明、对比、组织细节及已知材料和工艺判断。良好：细节清楚且观察与已知背景相符；轻微：局部失焦或照明不均但主要组织可辨；明显：细节不清或与背景存在需核实的矛盾；严重：失焦、曝光或信息缺失妨碍可靠识别。根据可见证据在0至25分内建议分数，不以模型未识别出组织作为制样错误；不能评价组织正确度时留空并明确缺失证据。",
	}
	ids := []string{"scratches", "artifacts", "etching", "clarity"}
	labels := []string{"划痕", "假象", "侵蚀程度", "组织清晰与正确度"}
	criteria := []any{}
	for i, id := range ids {
		criteria = append(criteria, map[string]any{"id": id, "label": labels[i], "max_score": 25.0, "rule_text": texts[i]})
	}
	return store.Entity{"id": builtinRubricID, "title": "金相制样学习评价 v1", "description": "系统内置金相制样教学评价，四项各25分。用于学习反馈，不代表国赛官方评分标准；分数须有可见依据，无法判断时留空。", "is_builtin": true, "demo": false, "criteria": criteria}
}

func (a *App) initializeTeaching(ctx context.Context) error {
	rule := builtinRubric()
	now := store.Now()
	rule["owner_id"], rule["class_id"], rule["status"], rule["version"] = "system", "", "published", 1
	rule["created_at"], rule["updated_at"], rule["published_at"] = now, now, now
	raw, _ := json.Marshal(rule)
	tx, err := a.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO entities(id,kind,owner_id,class_id,status,data,created_at,updated_at) VALUES(?,'rubrics','system','','published',?,?,?)`, builtinRubricID, string(raw), now, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO entity_versions SELECT id,version,data,created_at FROM entities WHERE id=?`, builtinRubricID); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	entries, err := a.Store.List(ctx, "atlas")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.String("status") == "published" && !demoRecord(entry) {
			if err = a.indexReference(ctx, entry, "atlas"); err != nil {
				return err
			}
		}
	}
	return nil
}
