package permit_eligibility_expiry_cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"heritage-tree-relocation-permit/internal/application"
	"heritage-tree-relocation-permit/internal/domain"
)

type mutableClock struct {
	now time.Time
}

func (c *mutableClock) Now() time.Time { return c.now }

type caseRepository struct {
	state domain.RelocationCase
}

func (r *caseRepository) Get(context.Context, string) (domain.RelocationCase, error) {
	return r.state, nil
}

func (*caseRepository) Create(context.Context, domain.RelocationCase, string, time.Time) (application.CommitResult, error) {
	return application.CommitResult{}, errors.New("unexpected Create call")
}

func (*caseRepository) List(context.Context) ([]domain.RelocationCase, error) {
	return nil, errors.New("unexpected List call")
}

func (*caseRepository) Commit(context.Context, application.CommitRequest) (application.CommitResult, error) {
	return application.CommitResult{}, errors.New("unexpected Commit call")
}

func (*caseRepository) Timeline(context.Context, string) ([]application.TimelineEntry, error) {
	return nil, errors.New("unexpected Timeline call")
}

func (*caseRepository) NextCaseNumber(context.Context, time.Time) (string, error) {
	return "", errors.New("unexpected NextCaseNumber call")
}

func (*caseRepository) NextPermitNumber(context.Context, time.Time) (string, error) {
	return "", errors.New("unexpected NextPermitNumber call")
}

func (*caseRepository) Close() error { return nil }

func TestPermitEligibilityExpiresWithoutCaseVersionChange(t *testing.T) {
	assessedAt := time.Date(2026, time.January, 1, 8, 0, 0, 0, time.UTC)
	state := eligibleCase(assessedAt)
	clock := &mutableClock{now: assessedAt.Add(179 * 24 * time.Hour)}
	service := application.NewService(&caseRepository{state: state}, clock, nil)

	initial, err := service.PermitEligibility(context.Background(), state.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if !initial.Eligible {
		t.Fatalf("测试前提错误：评估到期前应允许签发，实际 %#v", initial.Items)
	}

	clock.now = assessedAt.Add(181 * 24 * time.Hour)
	want := state.PermitEligibilityAt(clock.now)
	if want.Eligible {
		t.Fatal("测试前提错误：评估超过 180 天后领域层应拒绝签发")
	}
	got, err := service.PermitEligibility(context.Background(), state.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Eligible {
		t.Fatalf("档案版本未变化但评估已过期，缓存仍返回 eligible=true: %#v", got.Items)
	}
}

func eligibleCase(assessedAt time.Time) domain.RelocationCase {
	tree := domain.TreeProfile{
		TreeProfileID: "tree-1", SpeciesName: "香樟", ProtectionGrade: "二级",
		TrunkDiameterCM: 50, CrownRadiusM: 3, HealthGrade: "良好",
		RootSurvey: "根系完整并已定位", AssessedAt: assessedAt, Assessor: "评估员甲",
	}
	destination := domain.DestinationAssessment{
		DestinationAssessmentID: "site-1", SiteName: "植物园", AvailableRadiusM: 5,
		SoilType: "壤土", SoilPH: 6.5, DrainageGrade: "良好", RouteClearanceM: 5,
		AssessedAt: assessedAt, Assessor: "评估员乙",
	}
	revision := domain.MethodRevision{
		RevisionID: "rev-1", CaseID: "case-1", RevisionNumber: 1, RootBallDiameterCM: 420,
		ExcavationMeasures: "分区断根保持湿润", PackingMeasures: "钢丝网分层包装",
		TransportMeasures: "低速运输专人指挥", PlantingMeasures: "分层回填设置支撑",
		AftercareMeasures: "持续水肥树势监测", RiskControls: map[string]string{},
	}
	checklist := domain.FrozenChecklist{
		ChecklistID: "checklist-1", Version: 1, RevisionID: revision.RevisionID,
		Items: []domain.ChecklistItem{{Code: "equipment", Label: "设备到位", Required: true}},
	}
	precheck := domain.PrecheckRecord{
		PrecheckID: "precheck-1", ChecklistID: checklist.ChecklistID, RevisionID: revision.RevisionID,
		Inspector: "核验员", CheckedAt: assessedAt.Add(24 * time.Hour),
		Items:  []domain.ChecklistItem{{Code: "equipment", Label: "设备到位", Required: true, Passed: true}},
		Passed: true,
	}
	return domain.RelocationCase{
		CaseID: "case-1", CaseNumber: "GQ-20260101-0001", Status: domain.StatusPrecheckReady,
		Version: 7, ActiveRevisionID: revision.RevisionID, TreeProfile: &tree, Destination: &destination,
		Revisions: []domain.MethodRevision{revision}, FrozenChecklist: &checklist,
		Prechecks: []domain.PrecheckRecord{precheck},
	}
}
