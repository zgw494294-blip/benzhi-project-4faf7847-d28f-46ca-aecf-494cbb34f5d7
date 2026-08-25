package domain

import (
	"fmt"
	"strings"
)

func ValidateRevision(tree TreeProfile, destination DestinationAssessment, revision MethodRevision) error {
	var problems []Violation
	add := func(code, field, message string) { problems = append(problems, Violation{code, field, message}) }
	requiredRootBall := tree.TrunkDiameterCM * 8
	if revision.RootBallDiameterCM < requiredRootBall {
		add("ROOT_BALL_TOO_SMALL", "rootBallDiameterCm", fmt.Sprintf("土球直径应至少为树干胸径的 8 倍，即 %.0f 厘米", requiredRootBall))
	}
	measures := []struct{ field, value, label string }{
		{"excavationMeasures", revision.ExcavationMeasures, "起掘措施"},
		{"packingMeasures", revision.PackingMeasures, "包装措施"},
		{"transportMeasures", revision.TransportMeasures, "运输措施"},
		{"plantingMeasures", revision.PlantingMeasures, "定植措施"},
		{"aftercareMeasures", revision.AftercareMeasures, "养护措施"},
	}
	for _, measure := range measures {
		if len([]rune(strings.TrimSpace(measure.value))) < 4 {
			add("MEASURE_REQUIRED_"+strings.ToUpper(measure.field), measure.field, measure.label+"内容过短")
		}
	}
	requiredControls := RequiredRiskControls(tree, destination)
	for _, risk := range requiredControls {
		if len([]rune(strings.TrimSpace(revision.RiskControls[risk]))) < 4 {
			add("RISK_CONTROL_MISSING_"+strings.ToUpper(risk), "riskControls."+risk, "高风险项 "+risk+" 缺少有效控制措施")
		}
	}
	return NewRuleError(problems)
}

func RequiredRiskControls(tree TreeProfile, destination DestinationAssessment) []string {
	var risks []string
	if tree.HealthGrade == "衰弱" || tree.HealthGrade == "一般" {
		risks = append(risks, "tree_health")
	}
	if tree.ProtectionGrade == "一级" {
		risks = append(risks, "grade_one_protection")
	}
	if destination.DrainageGrade == "可改良" {
		risks = append(risks, "drainage_improvement")
	}
	if destination.RouteClearanceM < tree.TrunkDiameterCM/100+2.5 {
		risks = append(risks, "route_clearance")
	}
	return risks
}
