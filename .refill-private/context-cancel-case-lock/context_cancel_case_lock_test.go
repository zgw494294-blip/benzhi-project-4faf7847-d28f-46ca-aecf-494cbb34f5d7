package contextcancellock_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"heritage-tree-relocation-permit/internal/application"
	"heritage-tree-relocation-permit/internal/domain"
	"heritage-tree-relocation-permit/internal/storage/journal"
)

type blockingRepository struct {
	application.Repository
	started      chan struct{}
	release      chan struct{}
	startOnce    sync.Once
	canceledGet  chan struct{}
	canceledOnce sync.Once
}

func (r *blockingRepository) Commit(ctx context.Context, request application.CommitRequest) (application.CommitResult, error) {
	if request.EventType == "revision.created" {
		r.startOnce.Do(func() { close(r.started) })
		<-r.release
	}
	return r.Repository.Commit(ctx, request)
}

func (r *blockingRepository) Get(ctx context.Context, caseID string) (domain.RelocationCase, error) {
	if ctx.Err() != nil {
		r.canceledOnce.Do(func() { close(r.canceledGet) })
	}
	return r.Repository.Get(ctx, caseID)
}

func TestContextCancellationWhileWaitingForCaseLock(t *testing.T) {
	base, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	repository := &blockingRepository{
		Repository:  base,
		started:     make(chan struct{}),
		release:     make(chan struct{}),
		canceledGet: make(chan struct{}),
	}
	service := application.NewService(repository, application.RealClock{}, nil)
	ctx := context.Background()
	created, err := service.CreateCase(ctx, application.CreateCaseCommand{IdempotencyKey: "cancel-lock-create-0001"})
	if err != nil {
		t.Fatal(err)
	}
	assessed, err := service.RecordAssessments(ctx, created.CaseID, application.AssessmentCommand{
		ExpectedVersion: created.Version,
		IdempotencyKey:  "cancel-lock-assess-0001",
		Tree:            domain.TreeProfile{SpeciesName: "香樟", ProtectionGrade: "二级", TrunkDiameterCM: 50, CrownRadiusM: 3, HealthGrade: "良好", RootSurvey: "根系完整并已定位", AssessedAt: application.RealClock{}.Now(), Assessor: "甲"},
		Destination:     domain.DestinationAssessment{SiteName: "植物园", AvailableRadiusM: 5, SoilType: "壤土", SoilPH: 6.5, DrainageGrade: "良好", RouteClearanceM: 3, AssessedAt: application.RealClock{}.Now(), Assessor: "乙"},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan error, 1)
	go func() {
		_, callErr := service.AddRevision(ctx, created.CaseID, application.RevisionCommand{
			ExpectedVersion: assessed.Version, IdempotencyKey: "cancel-lock-revision-0001", RootBallDiameterCM: 420,
			ExcavationMeasures: "分区断根保持湿润", PackingMeasures: "钢丝网分层包装", TransportMeasures: "低速运输专人指挥", PlantingMeasures: "分层回填设置支撑", AftercareMeasures: "持续水肥树势监测", RiskControls: map[string]string{},
		})
		firstDone <- callErr
	}()
	<-repository.started

	secondContext, cancel := context.WithCancel(ctx)
	cancel()
	secondDone := make(chan error, 1)
	go func() {
		_, callErr := service.AddRevision(secondContext, created.CaseID, application.RevisionCommand{ExpectedVersion: assessed.Version, IdempotencyKey: "cancel-lock-revision-0002"})
		secondDone <- callErr
	}()
	close(repository.release)
	if callErr := <-firstDone; callErr != nil {
		t.Fatalf("首个提交失败: %v", callErr)
	}
	if callErr := <-secondDone; !errors.Is(callErr, context.Canceled) {
		t.Fatalf("取消请求应返回 context.Canceled，实际 %v", callErr)
	}
	select {
	case <-repository.canceledGet:
		t.Fatal("取消请求在等待档案锁后仍穿透到了 Repository.Get")
	default:
	}
}
