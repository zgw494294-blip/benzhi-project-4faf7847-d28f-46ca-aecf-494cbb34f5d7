package domain

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func validCase(t *testing.T) (RelocationCase, time.Time) {
	t.Helper()
	now := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	c, err := NewCase("case-1", "GQ-20260820-0001", now)
	if err != nil {
		t.Fatal(err)
	}
	tree := TreeProfile{TreeProfileID: "tree-1", SpeciesName: "香樟", ProtectionGrade: "二级", TrunkDiameterCM: 60, CrownRadiusM: 4, HealthGrade: "一般", RootSurvey: "主根完整，东侧侧根分段保护", AssessedAt: now.Add(-24 * time.Hour), Assessor: "甲"}
	site := DestinationAssessment{DestinationAssessmentID: "site-1", SiteName: "植物园东区", AvailableRadiusM: 6, SoilType: "壤土", SoilPH: 6.8, DrainageGrade: "可改良", RouteClearanceM: 3.2, AssessedAt: now.Add(-24 * time.Hour), Assessor: "乙"}
	if err := c.RecordAssessments(tree, site, now); err != nil {
		t.Fatal(err)
	}
	return c, now
}

func validRevision(c RelocationCase, now time.Time, controls map[string]string) MethodRevision {
	return MethodRevision{RevisionID: "rev-1", CaseID: c.CaseID, RevisionNumber: 1, RootBallDiameterCM: 500, ExcavationMeasures: "分区断根并保湿", PackingMeasures: "钢丝网分层包装", TransportMeasures: "低速运输专人指挥", PlantingMeasures: "分层回填设置透气管", AftercareMeasures: "两年水肥树势监测", RiskControls: controls, CreatedAt: now}
}

func TestInvalidRevisionDoesNotMutateCase(t *testing.T) {
	c, now := validCase(t)
	before := c
	revision := validRevision(c, now, map[string]string{})
	if err := c.AddRevision(revision, now); err == nil {
		t.Fatal("缺少高风险控制措施应失败")
	}
	if !reflect.DeepEqual(c, before) {
		t.Fatalf("规则失败后档案发生改变: %#v", c)
	}
}

func TestViolationsAreSorted(t *testing.T) {
	now := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	err := ValidateAssessments(TreeProfile{}, DestinationAssessment{}, now)
	var ruleErr *RuleError
	if !errors.As(err, &ruleErr) {
		t.Fatalf("期望 RuleError，实际 %v", err)
	}
	for i := 1; i < len(ruleErr.Violations); i++ {
		previous, current := ruleErr.Violations[i-1], ruleErr.Violations[i]
		if previous.Code > current.Code || previous.Code == current.Code && previous.Field > current.Field {
			t.Fatalf("违反项未稳定排序: %#v", ruleErr.Violations)
		}
	}
}

func TestPermitDigestIsIndependentOfMapIteration(t *testing.T) {
	c, now := validCase(t)
	one := validRevision(c, now, map[string]string{"tree_health": "树冠喷雾并缩短离土时间", "drainage_improvement": "增设盲沟并换填种植土"})
	two := validRevision(c, now, map[string]string{"drainage_improvement": "增设盲沟并换填种植土", "tree_health": "树冠喷雾并缩短离土时间"})
	check := PrecheckRecord{PrecheckID: "check-1", RevisionID: "rev-1", Items: []ChecklistItem{{Code: "weather", Passed: true}, {Code: "equipment", Passed: true}}}
	first, err := PermitDigest(c, one, check)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PermitDigest(c, two, check)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("同一内容摘要不稳定: %s != %s", first, second)
	}
}

func TestReassessmentInvalidatesActiveRevision(t *testing.T) {
	c, now := validCase(t)
	revision := validRevision(c, now, map[string]string{"tree_health": "树冠喷雾并缩短离土时间", "drainage_improvement": "增设盲沟并换填种植土"})
	if err := c.AddRevision(revision, now); err != nil {
		t.Fatal(err)
	}
	if c.ActiveRevisionID == "" {
		t.Fatal("首次方案应成为当前修订")
	}
	tree := *c.TreeProfile
	site := *c.Destination
	tree.RootSurvey = "复测后确认主根完整，侧根需要保护"
	if err := c.RecordAssessments(tree, site, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if c.ActiveRevisionID != "" {
		t.Fatalf("重新评估后旧方案不应保持当前资格: %s", c.ActiveRevisionID)
	}
	if len(c.Revisions) != 1 {
		t.Fatal("重新评估不应删除历史修订")
	}
}
