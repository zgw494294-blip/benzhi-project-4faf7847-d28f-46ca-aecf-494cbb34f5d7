package appendfailure_test

import (
	"context"
	"testing"
	"time"

	"heritage-tree-relocation-permit/internal/application"
	"heritage-tree-relocation-permit/internal/domain"
	"heritage-tree-relocation-permit/internal/storage/journal"
)

func TestFailedAppendDoesNotPublishState(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 26, 9, 30, 0, 0, time.UTC)
	repository, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := domain.NewCase("case-append-failure", "GQ-20260826-0001", now)
	if err != nil {
		t.Fatal(err)
	}
	result, err := repository.Create(ctx, created, "create-append-failure-0001", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	updated := result.State
	updated.Version++
	updated.UpdatedAt = now.Add(time.Minute)
	_, err = repository.Commit(ctx, application.CommitRequest{
		CaseID:          updated.CaseID,
		ExpectedVersion: result.State.Version,
		IdempotencyKey:  "commit-after-close-0001",
		EventType:       "case.test_updated",
		Summary:         "验证失效日志句柄的提交原子性",
		OccurredAt:      updated.UpdatedAt,
		State:           updated,
	})
	if err == nil {
		t.Fatal("已关闭的事件日志不应接受提交")
	}

	visible, getErr := repository.Get(ctx, updated.CaseID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if visible.Version != result.State.Version {
		t.Fatalf("失败的日志追加不得发布内存状态：提交前 version=%d，当前 version=%d", result.State.Version, visible.Version)
	}
}
