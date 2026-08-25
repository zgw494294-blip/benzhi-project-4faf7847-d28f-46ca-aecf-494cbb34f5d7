package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Violation struct {
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

type RuleError struct {
	Violations []Violation `json:"violations"`
}

func (e *RuleError) Error() string {
	parts := make([]string, len(e.Violations))
	for i, violation := range e.Violations {
		parts[i] = violation.Message
	}
	return strings.Join(parts, "；")
}

func NewRuleError(items []Violation) error {
	if len(items) == 0 {
		return nil
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Code == items[j].Code {
			return items[i].Field < items[j].Field
		}
		return items[i].Code < items[j].Code
	})
	return &RuleError{Violations: items}
}

func ValidateAssessments(tree TreeProfile, destination DestinationAssessment, now time.Time) error {
	var problems []Violation
	add := func(code, field, message string) { problems = append(problems, Violation{code, field, message}) }
	if strings.TrimSpace(tree.SpeciesName) == "" {
		add("TREE_SPECIES_REQUIRED", "speciesName", "树种名称不能为空")
	}
	if tree.ProtectionGrade != "一级" && tree.ProtectionGrade != "二级" && tree.ProtectionGrade != "三级" {
		add("TREE_GRADE_INVALID", "protectionGrade", "保护等级必须为一级、二级或三级")
	}
	if tree.TrunkDiameterCM < 20 {
		add("TREE_DIAMETER_INVALID", "trunkDiameterCm", "树干胸径不得小于 20 厘米")
	}
	if tree.CrownRadiusM <= 0 {
		add("TREE_CROWN_INVALID", "crownRadiusM", "冠幅半径必须大于零")
	}
	if tree.HealthGrade != "良好" && tree.HealthGrade != "一般" && tree.HealthGrade != "衰弱" {
		add("TREE_HEALTH_INVALID", "healthGrade", "健康等级必须为良好、一般或衰弱")
	}
	if strings.TrimSpace(tree.RootSurvey) == "" {
		add("TREE_ROOT_SURVEY_REQUIRED", "rootSurvey", "必须填写根系调查结论")
	}
	if strings.TrimSpace(tree.Assessor) == "" {
		add("TREE_ASSESSOR_REQUIRED", "assessor", "必须填写树体评估人员")
	}
	if tree.AssessedAt.IsZero() || tree.AssessedAt.After(now.Add(5*time.Minute)) || now.Sub(tree.AssessedAt) > 180*24*time.Hour {
		add("TREE_ASSESSMENT_STALE", "assessedAt", "树体评估日期必须在最近 180 天内")
	}
	if strings.TrimSpace(destination.SiteName) == "" {
		add("DESTINATION_SITE_REQUIRED", "siteName", "迁入地名称不能为空")
	}
	if destination.AvailableRadiusM < tree.CrownRadiusM*1.2 {
		add("DESTINATION_SPACE_INSUFFICIENT", "availableRadiusM", fmt.Sprintf("迁入地可用半径应至少为 %.1f 米", tree.CrownRadiusM*1.2))
	}
	if strings.TrimSpace(destination.SoilType) == "" {
		add("DESTINATION_SOIL_REQUIRED", "soilType", "必须填写迁入地土壤类型")
	}
	if destination.SoilPH < 5.0 || destination.SoilPH > 8.5 {
		add("DESTINATION_PH_INVALID", "soilPH", "土壤 pH 必须在 5.0 至 8.5 之间")
	}
	if destination.DrainageGrade != "良好" && destination.DrainageGrade != "可改良" {
		add("DESTINATION_DRAINAGE_INVALID", "drainageGrade", "排水等级必须为良好或可改良")
	}
	minimumClearance := tree.TrunkDiameterCM/100 + 1.5
	if destination.RouteClearanceM < minimumClearance {
		add("ROUTE_CLEARANCE_INSUFFICIENT", "routeClearanceM", fmt.Sprintf("运输通道净宽应至少为 %.1f 米", minimumClearance))
	}
	if strings.TrimSpace(destination.Assessor) == "" {
		add("DESTINATION_ASSESSOR_REQUIRED", "assessor", "必须填写迁入地评估人员")
	}
	if destination.AssessedAt.IsZero() || destination.AssessedAt.After(now.Add(5*time.Minute)) || now.Sub(destination.AssessedAt) > 180*24*time.Hour {
		add("DESTINATION_ASSESSMENT_STALE", "assessedAt", "迁入地评估日期必须在最近 180 天内")
	}
	return NewRuleError(problems)
}
