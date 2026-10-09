package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUnderstandingSchemaDiagnosticsAndTargetedRepair(t *testing.T) {
	var data map[string]any
	if err := json.Unmarshal([]byte(validUnderstanding()), &data); err != nil {
		t.Fatal(err)
	}
	data["dimensions"] = map[string]any{"组织": "不能使用此类型"}
	raw, _ := json.Marshal(data)
	_, err := Validate(sampleRequest("understanding"), raw)
	path, reason := diagnostic(err)
	if reason != "incorrect_json_type" || path != "dimensions" {
		t.Fatal(path, reason, err)
	}
	repair := repairMessage("understanding", err)
	if !strings.Contains(repair, "dimensions") || !strings.Contains(repair, "incorrect_json_type") || strings.Contains(repair, "criteria") {
		t.Fatal("repair must be specific to the failed report type", repair)
	}
	delete(data, "dimensions")
	data["untrusted_secret_field"] = "never print this value"
	raw, _ = json.Marshal(data)
	_, err = Validate(sampleRequest("understanding"), raw)
	path, reason = diagnostic(err)
	if reason != "unknown_field" || path != "$" || strings.Contains(repairMessage("understanding", err), "untrusted_secret_field") {
		t.Fatal(path, reason, err)
	}
}
