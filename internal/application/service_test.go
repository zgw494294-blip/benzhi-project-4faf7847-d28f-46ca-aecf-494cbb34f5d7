package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"heritage-tree-relocation-permit/internal/application"
	"heritage-tree-relocation-permit/internal/domain"
	"heritage-tree-relocation-permit/internal/storage/journal"
)

func TestIdempotentRetryReturnsOriginalState(t *testing.T) {
	repository, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	service := application.NewService(repository, application.RealClock{}, nil)
	ctx := context.Background()
	created, err := service.CreateCase(ctx, application.CreateCaseCommand{IdempotencyKey: "create-case-0001"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Hour)
	command := application.AssessmentCommand{ExpectedVersion: created.Version, IdempotencyKey: "assessment-0001", Tree: domain.TreeProfile{SpeciesName: "香樟", ProtectionGrade: "二级", TrunkDiameterCM: 50, CrownRadiusM: 3, HealthGrade: "良好", RootSurvey: "根系完整并已定位", AssessedAt: now, Assessor: "甲"}, Destination: domain.DestinationAssessment{SiteName: "植物园", AvailableRadiusM: 5, SoilType: "壤土", SoilPH: 6.5, DrainageGrade: "良好", RouteClearanceM: 3, AssessedAt: now, Assessor: "乙"}}
	assessed, err := service.RecordAssessments(ctx, created.CaseID, command)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := service.AddRevision(ctx, created.CaseID, application.RevisionCommand{ExpectedVersion: assessed.Version, IdempotencyKey: "revision-0001", RootBallDiameterCM: 420, ExcavationMeasures: "分区断根保持湿润", PackingMeasures: "钢丝网分层包装", TransportMeasures: "低速运输专人指挥", PlantingMeasures: "分层回填设置支撑", AftercareMeasures: "持续水肥树势监测", RiskControls: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if revision.Version != 3 {
		t.Fatalf("期望当前版本 3，实际 %d", revision.Version)
	}
	retried, err := service.RecordAssessments(ctx, created.CaseID, command)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Version != assessed.Version {
		t.Fatalf("幂等重试应返回首次版本 %d，实际 %d", assessed.Version, retried.Version)
	}
}

func TestStaleVersionDoesNotWriteEvent(t *testing.T) {
	repository, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	service := application.NewService(repository, application.RealClock{}, nil)
	ctx := context.Background()
	created, err := service.CreateCase(ctx, application.CreateCaseCommand{IdempotencyKey: "create-case-0002"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.AddRevision(ctx, created.CaseID, application.RevisionCommand{ExpectedVersion: 99, IdempotencyKey: "stale-revision-0001"})
	if !errors.Is(err, application.ErrConflict) {
		t.Fatalf("期望版本冲突，实际 %v", err)
	}
	timeline, err := service.Timeline(ctx, created.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(timeline) != 1 {
		t.Fatalf("失败命令不应写事件，实际时间线长度 %d", len(timeline))
	}
}
