package cursor_reorder_duplicate_test

import (
	"context"
	"testing"
	"time"

	"heritage-tree-relocation-permit/internal/application"
	"heritage-tree-relocation-permit/internal/domain"
	"heritage-tree-relocation-permit/internal/storage/journal"
)

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

func TestQueueCursorDoesNotRepeatCaseAfterPriorityReorder(t *testing.T) {
	directory := t.TempDir()
	repository, err := journal.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := repository.Close(); err != nil {
			t.Errorf("关闭 repository: %v", err)
		}
	})

	now := time.Date(2026, time.August, 26, 9, 0, 0, 0, time.UTC)
	service := application.NewService(repository, fixedClock{now: now}, nil)
	ctx := context.Background()
	created := make([]domain.RelocationCase, 0, 3)
	for _, key := range []string{"create-page-0001", "create-page-0002", "create-page-0003"} {
		item, err := service.CreateCase(ctx, application.CreateCaseCommand{IdempotencyKey: key})
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, item)
	}

	first, err := service.QueryCases(ctx, application.CaseQuery{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatalf("第一页应包含一个档案和后续游标: %+v", first)
	}

	_, err = service.RecordAssessments(ctx, created[2].CaseID, application.AssessmentCommand{
		ExpectedVersion: created[2].Version,
		IdempotencyKey:  "assess-page-0003",
		Tree: domain.TreeProfile{
			SpeciesName: "香樟", ProtectionGrade: "二级", TrunkDiameterCM: 50,
			CrownRadiusM: 3, HealthGrade: "良好", RootSurvey: "根系完整并已定位",
			AssessedAt: now.Add(-time.Hour), Assessor: "评估员甲",
		},
		Destination: domain.DestinationAssessment{
			SiteName: "植物园", AvailableRadiusM: 5, SoilType: "壤土", SoilPH: 6.5,
			DrainageGrade: "良好", RouteClearanceM: 3,
			AssessedAt: now.Add(-time.Hour), Assessor: "评估员乙",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	repository, err = journal.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	service = application.NewService(repository, fixedClock{now: now}, nil)

	second, err := service.QueryCases(ctx, application.CaseQuery{Limit: 1, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 {
		t.Fatalf("第二页应包含一个档案: %+v", second)
	}
	if second.Items[0].CaseID == first.Items[0].CaseID {
		t.Fatalf("状态重排后第二页重复了第一页档案 %s", first.Items[0].CaseID)
	}
}
