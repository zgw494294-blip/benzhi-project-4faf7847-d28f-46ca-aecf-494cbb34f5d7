package permit_snapshot_input_alias_test

import (
	"testing"
	"time"

	"heritage-tree-relocation-permit/internal/domain"
)

func TestIssuedPermitSnapshotIsolatedFromCallerMutation(t *testing.T) {
	now := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
	item, err := domain.NewCase("case-alias", "GQ-20260826-0042", now)
	if err != nil {
		t.Fatal(err)
	}
	tree := domain.TreeProfile{
		TreeProfileID: "tree-alias", SpeciesName: "香樟", ProtectionGrade: "二级",
		TrunkDiameterCM: 60, CrownRadiusM: 4, HealthGrade: "一般",
		RootSurvey: "主根完整且侧根保护范围已定位", AssessedAt: now.Add(-time.Hour), Assessor: "评估员甲",
	}
	destination := domain.DestinationAssessment{
		DestinationAssessmentID: "site-alias", SiteName: "植物园东区", AvailableRadiusM: 6,
		SoilType: "壤土", SoilPH: 6.8, DrainageGrade: "可改良", RouteClearanceM: 3.2,
		AssessedAt: now.Add(-time.Hour), Assessor: "评估员乙",
	}
	if err := item.RecordAssessments(tree, destination, now); err != nil {
		t.Fatal(err)
	}
	revision := domain.MethodRevision{
		RevisionID: "revision-alias", CaseID: item.CaseID, RevisionNumber: 1,
		RootBallDiameterCM: 500, ExcavationMeasures: "分区断根并持续保湿",
		PackingMeasures: "钢丝网分层包装土球", TransportMeasures: "低速运输并由专人指挥",
		PlantingMeasures: "分层回填并设置支撑", AftercareMeasures: "持续开展水肥树势监测",
		RiskControls: map[string]string{
			"tree_health":          "树冠喷雾并缩短离土时间",
			"drainage_improvement": "增设盲沟并换填种植土",
		},
		CreatedAt: now,
	}
	if err := item.AddRevision(revision, now); err != nil {
		t.Fatal(err)
	}
	if err := item.SubmitReview(nil, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	precheckItems := append([]domain.ChecklistItem(nil), item.FrozenChecklist.Items...)
	for index := range precheckItems {
		precheckItems[index].Passed = true
		precheckItems[index].Note = "符合现场条件"
	}
	precheck := domain.PrecheckRecord{
		PrecheckID: "precheck-alias", ChecklistID: item.FrozenChecklist.ChecklistID,
		RevisionID: item.ActiveRevisionID, Inspector: "现场核验员",
		CheckedAt: now.Add(2 * time.Hour), Items: precheckItems,
	}
	if err := item.RecordPrecheck(precheck, now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	passingPrecheck, ok := item.LatestPassingPrecheck()
	if !ok {
		t.Fatal("测试前置条件错误：现场核验应已通过")
	}
	snapshot := domain.NewPermitSnapshot(item, revision, *item.FrozenChecklist, passingPrecheck)
	digest, err := domain.PermitSnapshotDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	permit := domain.WorkPermit{
		PermitID: "permit-alias", PermitNumber: "XK-20260826-0042", CaseID: item.CaseID,
		FrozenRevisionID: revision.RevisionID, PrecheckSnapshot: passingPrecheck,
		Snapshot: snapshot, ContentDigest: digest, IssuedBy: "质量负责人", IssuedAt: now.Add(3 * time.Hour),
	}
	if err := item.IssuePermit(permit, now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}

	permit.Snapshot.Revision.RiskControls["tree_health"] = "调用方复用 map 后写入的污染内容"
	permit.Snapshot.Checklist.Items[0].Note = "调用方复用 checklist slice 后写入的污染内容"
	permit.Snapshot.Precheck.Items[0].Note = "调用方复用 precheck slice 后写入的污染内容"

	verification := domain.VerifyPermitDigest(*item.Permit)
	if verification.Status != "matching" {
		t.Fatalf("签发后的不可变许可不应受原始命令别名污染，摘要复验状态=%s", verification.Status)
	}
}
