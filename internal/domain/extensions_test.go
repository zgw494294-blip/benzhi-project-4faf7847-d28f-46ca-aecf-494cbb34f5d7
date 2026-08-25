package domain

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestExpiredAssessmentBlocksReviewWithoutMutation(t *testing.T) {
	c, now := validCase(t)
	if err := c.AddRevision(validRevision(c, now, map[string]string{
		"tree_health": "树冠喷雾并缩短离土时间", "drainage_improvement": "增设盲沟并换填种植土",
	}), now); err != nil {
		t.Fatal(err)
	}
	before := c
	err := c.SubmitReview(nil, now.Add(181*24*time.Hour))
	var ruleErr *RuleError
	if !errors.As(err, &ruleErr) || len(ruleErr.Violations) != 2 {
		t.Fatalf("期望两项评估过期违反项，实际 %v", err)
	}
	if !reflect.DeepEqual(c, before) {
		t.Fatal("过期评估导致审查失败时不应改变档案")
	}
}

func TestReviewDuplicateDetectionIsAtomic(t *testing.T) {
	c, now := validCase(t)
	if err := c.AddRevision(validRevision(c, now, map[string]string{
		"tree_health": "树冠喷雾并缩短离土时间", "drainage_improvement": "增设盲沟并换填种植土",
	}), now); err != nil {
		t.Fatal(err)
	}
	before := c
	findings := []ReviewFinding{
		{FindingID: "finding-1", CaseID: c.CaseID, RevisionID: c.ActiveRevisionID, Severity: SeverityBlocking, Category: "运输 组织", Description: "职责  需要明确", Status: FindingOpen},
		{FindingID: "finding-2", CaseID: c.CaseID, RevisionID: c.ActiveRevisionID, Severity: SeverityBlocking, Category: "运输 组织", Description: "职责需要明确", Status: FindingOpen},
	}
	if err := c.SubmitReview(findings, now); err == nil {
		t.Fatal("规范化后重复的问题应被拒绝")
	}
	if !reflect.DeepEqual(c, before) {
		t.Fatal("重复问题失败时不应部分保存")
	}
}

func TestEvidenceRoundsAndChecklistRetryArePreserved(t *testing.T) {
	c, now := validCase(t)
	if err := c.AddRevision(validRevision(c, now, map[string]string{
		"tree_health": "树冠喷雾并缩短离土时间", "drainage_improvement": "增设盲沟并换填种植土", "route_clearance": "运输路线专人指挥",
	}), now); err != nil {
		t.Fatal(err)
	}
	finding := ReviewFinding{FindingID: "finding-1", CaseID: c.CaseID, RevisionID: c.ActiveRevisionID, Severity: SeverityBlocking, Category: "资料", Description: "现场签字缺失", Status: FindingOpen}
	if err := c.SubmitReview([]ReviewFinding{finding}, now); err != nil {
		t.Fatal(err)
	}
	if err := c.SubmitEvidenceBy(finding.FindingID, "第一轮现场记录", "编制人甲", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := c.VerifyFindingWithOpinion(finding.FindingID, "审查员乙", "现场签字缺失", false, now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := c.SubmitEvidenceBy(finding.FindingID, "第二轮含签字记录", "编制人甲", now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := c.Findings[0].EvidenceRounds; len(got) != 2 || got[0].Decision != "rejected" || got[1].Round != 2 {
		t.Fatalf("证据轮次未完整保留: %#v", got)
	}
	if err := c.VerifyFindingWithOpinion(finding.FindingID, "审查员乙", "签字完整，同意关闭", true, now.Add(4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusPrecheckReady || c.FrozenChecklist == nil || len(c.FrozenChecklist.Items) != 7 {
		t.Fatalf("最后阻断项关闭后未生成风险清单: %#v", c.FrozenChecklist)
	}
	items := append([]ChecklistItem(nil), c.FrozenChecklist.Items...)
	for i := range items {
		items[i].Passed = true
		items[i].Note = "符合条件"
	}
	items[1].Passed = false
	items[1].Note = "设备证书待补齐"
	first := PrecheckRecord{PrecheckID: "precheck-1", ChecklistID: c.FrozenChecklist.ChecklistID, RevisionID: c.ActiveRevisionID, Inspector: "核验员", CheckedAt: now.Add(5 * time.Hour), Items: items}
	if err := c.RecordPrecheck(first, now.Add(5*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.LatestPassingPrecheck(); ok {
		t.Fatal("最近一次失败核验不应取得签发资格")
	}
	for i := range items {
		items[i].Passed = true
	}
	second := PrecheckRecord{PrecheckID: "precheck-2", ChecklistID: c.FrozenChecklist.ChecklistID, RevisionID: c.ActiveRevisionID, Inspector: "核验员", CheckedAt: now.Add(6 * time.Hour), Items: items}
	if err := c.RecordPrecheck(second, now.Add(6*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(c.Prechecks) != 2 || !c.PermitEligibilityAt(now.Add(6*time.Hour)).Eligible {
		t.Fatal("同一冻结清单复查合格后应保留两次记录并取得资格")
	}
}

func TestRevisionComparisonAndPermitSnapshotVerification(t *testing.T) {
	c, now := validCase(t)
	first := validRevision(c, now, map[string]string{"tree_health": "喷雾保湿", "drainage_improvement": "设置盲沟"})
	if err := c.AddRevision(first, now); err != nil {
		t.Fatal(err)
	}
	second := first
	second.RevisionID, second.RevisionNumber = "rev-2", 2
	second.TransportMeasures = "夜间低速运输并封闭转弯点"
	second.RiskControls = map[string]string{"tree_health": "喷雾保湿", "drainage_improvement": "设置盲沟", "route_clearance": "增加净宽复测"}
	if err := c.AddRevision(second, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	comparison, err := c.CompareRevisions(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Differences[3].Field != "transportMeasures" || comparison.Differences[3].Change != "modified" {
		t.Fatalf("运输措施差异不正确: %#v", comparison.Differences)
	}
	last := comparison.Differences[len(comparison.Differences)-1]
	if last.Field != "riskControls.tree_health" || last.Change != "unchanged" {
		t.Fatalf("风险差异顺序不稳定: %#v", comparison.Differences)
	}
	if err := c.SubmitReview(nil, now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	items := append([]ChecklistItem(nil), c.FrozenChecklist.Items...)
	for i := range items {
		items[i].Passed, items[i].Note = true, "符合条件"
	}
	precheck := PrecheckRecord{PrecheckID: "precheck-1", ChecklistID: c.FrozenChecklist.ChecklistID, RevisionID: c.ActiveRevisionID, Inspector: "核验员", CheckedAt: now.Add(3 * time.Hour), Items: items}
	if err := c.RecordPrecheck(precheck, now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	precheck, _ = c.LatestPassingPrecheck()
	snapshot := NewPermitSnapshot(c, second, *c.FrozenChecklist, precheck)
	digest, err := PermitSnapshotDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	permit := WorkPermit{ContentDigest: digest, Snapshot: snapshot}
	if result := VerifyPermitDigest(permit); result.Status != "matching" {
		t.Fatalf("摘要应匹配: %#v", result)
	}
	permit.Snapshot.Revision.TransportMeasures = "损坏内容"
	if result := VerifyPermitDigest(permit); result.Status != "mismatch" {
		t.Fatalf("损坏内容应摘要不匹配: %#v", result)
	}
	permit.Snapshot.TreeProfile.SpeciesName = ""
	if result := VerifyPermitDigest(permit); result.Status != "missing_fields" {
		t.Fatalf("冻结字段缺失应明确诊断: %#v", result)
	}
}
