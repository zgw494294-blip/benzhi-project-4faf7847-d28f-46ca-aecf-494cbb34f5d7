package precheck_template_drift_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"heritage-tree-relocation-permit/internal/application"
	"heritage-tree-relocation-permit/internal/domain"
	"heritage-tree-relocation-permit/internal/storage/journal"
)

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type sequenceIDs struct {
	next int
}

func (ids *sequenceIDs) NewID(prefix string) string {
	ids.next++
	return fmt.Sprintf("%s-private-%02d", prefix, ids.next)
}

func TestPermitSnapshotKeepsPrecheckTemplateCanonical(t *testing.T) {
	repository, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	now := time.Date(2026, 8, 26, 9, 30, 0, 0, time.UTC)
	service := application.NewService(repository, fixedClock{now: now}, &sequenceIDs{})
	ctx := context.Background()

	current, err := service.CreateCase(ctx, application.CreateCaseCommand{IdempotencyKey: "template-create-0001"})
	if err != nil {
		t.Fatal(err)
	}
	current, err = service.RecordAssessments(ctx, current.CaseID, application.AssessmentCommand{
		ExpectedVersion: current.Version,
		IdempotencyKey:  "template-assess-0001",
		Tree: domain.TreeProfile{
			SpeciesName: "香樟", ProtectionGrade: "二级", TrunkDiameterCM: 50,
			CrownRadiusM: 3, HealthGrade: "良好", RootSurvey: "根系完整并已完成定位",
			AssessedAt: now.Add(-time.Hour), Assessor: "树体评估员",
		},
		Destination: domain.DestinationAssessment{
			SiteName: "植物园", AvailableRadiusM: 5, SoilType: "壤土", SoilPH: 6.5,
			DrainageGrade: "良好", RouteClearanceM: 4,
			AssessedAt: now.Add(-time.Hour), Assessor: "场地评估员",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err = service.AddRevision(ctx, current.CaseID, application.RevisionCommand{
		ExpectedVersion:    current.Version,
		IdempotencyKey:     "template-revision-0001",
		RootBallDiameterCM: 420,
		ExcavationMeasures: "分区断根并保持根系湿润",
		PackingMeasures:    "钢丝网分层包装并固定土球",
		TransportMeasures:  "低速运输并安排专人指挥",
		PlantingMeasures:   "分层回填并设置稳固支撑",
		AftercareMeasures:  "持续开展水肥和树势监测",
		RiskControls:       map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err = service.SubmitReview(ctx, current.CaseID, application.ReviewCommand{
		ExpectedVersion: current.Version,
		IdempotencyKey:  "template-review-0001",
		Findings:        []application.FindingInput{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if current.FrozenChecklist == nil || len(current.FrozenChecklist.Items) < 2 {
		t.Fatalf("审查后未形成可用于复现的冻结清单: %#v", current.FrozenChecklist)
	}

	template := *current.FrozenChecklist
	submitted := make([]domain.ChecklistItem, len(template.Items))
	for i := range template.Items {
		item := template.Items[len(template.Items)-1-i]
		item.Label = "调用方改写标签-" + item.Code
		item.Passed = true
		item.Note = "现场核验合格"
		submitted[i] = item
	}
	current, err = service.RecordPrecheck(ctx, current.CaseID, application.PrecheckCommand{
		ExpectedVersion: current.Version,
		IdempotencyKey:  "template-precheck-0001",
		ChecklistID:     template.ChecklistID,
		Inspector:       "现场核验员",
		CheckedAt:       now,
		Items:           submitted,
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err = service.IssuePermit(ctx, current.CaseID, application.IssuePermitCommand{
		ExpectedVersion: current.Version,
		IdempotencyKey:  "template-permit-0001",
		IssuedBy:        "树木保护专员",
	})
	if err != nil {
		t.Fatal(err)
	}
	if current.Permit == nil || current.Permit.Snapshot.Checklist == nil || current.Permit.Snapshot.Precheck == nil {
		t.Fatalf("许可冻结快照不完整: %#v", current.Permit)
	}

	checklistItems := current.Permit.Snapshot.Checklist.Items
	precheckItems := current.Permit.Snapshot.Precheck.Items
	if len(checklistItems) != len(precheckItems) {
		t.Fatalf("许可中的冻结清单与核验记录项目数不一致: %d != %d", len(checklistItems), len(precheckItems))
	}
	for i := range checklistItems {
		if checklistItems[i].Code != precheckItems[i].Code || checklistItems[i].Label != precheckItems[i].Label {
			t.Fatalf("许可冻结内容发生模板漂移: index=%d checklist=%#v precheck=%#v", i, checklistItems[i], precheckItems[i])
		}
	}
}
