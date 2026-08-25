package application_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"heritage-tree-relocation-permit/internal/application"
	"heritage-tree-relocation-permit/internal/domain"
	"heritage-tree-relocation-permit/internal/storage/journal"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type sequenceIDs struct{ next int }

func (ids *sequenceIDs) NewID(prefix string) string {
	ids.next++
	return prefix + "-generated-" + time.Unix(int64(ids.next), 0).UTC().Format("150405")
}

func TestCaseQueuePaginationAndTamperDetectionAreReadOnly(t *testing.T) {
	repository, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	clock := fixedClock{now: time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)}
	service := application.NewService(repository, clock, &sequenceIDs{})
	ctx := context.Background()
	created := make([]domain.RelocationCase, 0, 3)
	for i := 0; i < 3; i++ {
		item, err := service.CreateCase(ctx, application.CreateCaseCommand{IdempotencyKey: "queue-create-000" + string(rune('1'+i))})
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, item)
	}
	first, err := service.QueryCases(ctx, application.CaseQuery{Statuses: []domain.CaseStatus{domain.StatusDraft}, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.NextCursor == "" || first.Summary.Total != 3 {
		t.Fatalf("第一页结果不完整: %#v", first)
	}
	second, err := service.QueryCases(ctx, application.CaseQuery{Statuses: []domain.CaseStatus{domain.StatusDraft}, Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].CaseID == first.Items[0].CaseID || second.Items[0].CaseID == first.Items[1].CaseID {
		t.Fatalf("翻页出现重复: %#v", second.Items)
	}
	tampered := first.NextCursor[:len(first.NextCursor)-1] + "x"
	if _, err := service.QueryCases(ctx, application.CaseQuery{Statuses: []domain.CaseStatus{domain.StatusDraft}, Limit: 2, Cursor: tampered}); err == nil {
		t.Fatal("篡改游标应被拒绝")
	}
	for _, before := range created {
		after, err := service.GetCase(ctx, before.CaseID)
		if err != nil || after.Version != before.Version || !after.UpdatedAt.Equal(before.UpdatedAt) {
			t.Fatalf("查询不应改变档案: before=%#v after=%#v err=%v", before, after, err)
		}
	}
}

func TestReassessmentCanCarryLatestRevisionWithCurrentRiskValidation(t *testing.T) {
	repository, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	now := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
	service := application.NewService(repository, fixedClock{now: now}, &sequenceIDs{})
	ctx := context.Background()
	created, err := service.CreateCase(ctx, application.CreateCaseCommand{IdempotencyKey: "carry-create-0001"})
	if err != nil {
		t.Fatal(err)
	}
	assessment := application.AssessmentCommand{
		ExpectedVersion: created.Version, IdempotencyKey: "carry-assess-0001",
		Tree:        domain.TreeProfile{SpeciesName: "香樟", ProtectionGrade: "二级", TrunkDiameterCM: 50, CrownRadiusM: 3, HealthGrade: "良好", RootSurvey: "根系完整并已经完成定位", AssessedAt: now.Add(-time.Hour), Assessor: "甲"},
		Destination: domain.DestinationAssessment{SiteName: "植物园", AvailableRadiusM: 5, SoilType: "壤土", SoilPH: 6.5, DrainageGrade: "良好", RouteClearanceM: 4, AssessedAt: now.Add(-time.Hour), Assessor: "乙"},
	}
	assessed, err := service.RecordAssessments(ctx, created.CaseID, assessment)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.AddRevision(ctx, created.CaseID, application.RevisionCommand{ExpectedVersion: assessed.Version, IdempotencyKey: "carry-revision-0001", RootBallDiameterCM: 420, ExcavationMeasures: "分区断根保持根系湿润", PackingMeasures: "钢丝网分层包装土球", TransportMeasures: "低速运输并由专人指挥", PlantingMeasures: "分层回填并设置支撑", AftercareMeasures: "持续开展水肥树势监测", RiskControls: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	reassessment := assessment
	reassessment.ExpectedVersion = first.Version
	reassessment.IdempotencyKey = "carry-assess-0002"
	reassessment.Tree.TreeProfileID = ""
	reassessment.Destination.DestinationAssessmentID = ""
	reassessment.Tree.HealthGrade = "一般"
	reassessed, err := service.RecordAssessments(ctx, created.CaseID, reassessment)
	if err != nil {
		t.Fatal(err)
	}
	carry := application.RevisionCommand{ExpectedVersion: reassessed.Version, IdempotencyKey: "carry-revision-0002", CarryFromRevisionNumber: 1}
	if _, err := service.AddRevision(ctx, created.CaseID, carry); err == nil || !strings.Contains(err.Error(), "tree_health") {
		t.Fatalf("沿用内容缺少新增健康风险控制时应失败，实际 %v", err)
	}
	unchanged, _ := service.GetCase(ctx, created.CaseID)
	if unchanged.Version != reassessed.Version || unchanged.ActiveRevisionID != "" || len(unchanged.Revisions) != 1 {
		t.Fatalf("失败沿用不应改变聚合: %#v", unchanged)
	}
	carry.IdempotencyKey = "carry-revision-0003"
	carry.RiskControls = map[string]string{"tree_health": "设置树冠喷雾并缩短离土时间"}
	carried, err := service.AddRevision(ctx, created.CaseID, carry)
	if err != nil {
		t.Fatal(err)
	}
	if len(carried.Revisions) != 2 || carried.ActiveRevisionID == "" || len(carried.TreeProfileHistory) != 2 {
		t.Fatalf("有效沿用未建立新修订或评估历史丢失: %#v", carried)
	}
}
