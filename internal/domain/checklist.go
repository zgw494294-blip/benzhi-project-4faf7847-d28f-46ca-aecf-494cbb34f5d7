package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

var baselineChecklist = []ChecklistItem{
	{Code: "personnel", Label: "人员到位", Required: true},
	{Code: "equipment", Label: "设备状态", Required: true},
	{Code: "weather", Label: "天气窗口", Required: true},
	{Code: "materials", Label: "保护物资", Required: true},
}

var riskLabels = map[string]string{
	"tree_health":          "树体健康风险控制落实",
	"grade_one_protection": "一级保护强化措施落实",
	"drainage_improvement": "排水改良措施落实",
	"route_clearance":      "运输净宽风险控制落实",
}

func BuildFrozenChecklist(revision MethodRevision, now time.Time) FrozenChecklist {
	items := append([]ChecklistItem(nil), baselineChecklist...)
	keys := make([]string, 0, len(revision.RiskControls))
	for key, value := range revision.RiskControls {
		if strings.TrimSpace(value) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		label := riskLabels[key]
		if label == "" {
			label = "方案风险控制：" + key
		}
		items = append(items, ChecklistItem{Code: "risk:" + key, Label: label, Required: true})
	}
	return FrozenChecklist{
		ChecklistID: fmt.Sprintf("checklist-%s-v%d", revision.RevisionID, revision.RevisionNumber),
		Version:     revision.RevisionNumber, RevisionID: revision.RevisionID, Items: items, FrozenAt: now.UTC(),
	}
}

func ValidatePrecheckItems(template FrozenChecklist, submitted []ChecklistItem) (bool, error) {
	if len(submitted) != len(template.Items) {
		return false, fmt.Errorf("现场核验项目数量与冻结清单不一致")
	}
	want := make(map[string]ChecklistItem, len(template.Items))
	for _, item := range template.Items {
		want[item.Code] = item
	}
	seen := make(map[string]bool, len(submitted))
	passed := true
	for i, item := range submitted {
		templateItem, ok := want[item.Code]
		if !ok {
			return false, fmt.Errorf("现场核验包含未知项目 %s", item.Code)
		}
		if seen[item.Code] {
			return false, fmt.Errorf("现场核验项目 %s 重复", item.Code)
		}
		seen[item.Code] = true
		if templateItem.Required && !item.Required {
			return false, fmt.Errorf("现场核验必检项 %s 不得降级", item.Code)
		}
		if templateItem.Required && !item.Passed {
			passed = false
			if strings.TrimSpace(item.Note) == "" {
				return false, fmt.Errorf("不合格必检项 %s 必须填写原因（第 %d 项）", item.Code, i+1)
			}
		}
	}
	return passed, nil
}
