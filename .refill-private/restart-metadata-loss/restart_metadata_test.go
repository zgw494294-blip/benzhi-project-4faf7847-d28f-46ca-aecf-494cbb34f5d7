package restart_metadata_loss_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"heritage-tree-relocation-permit/internal/domain"
	"heritage-tree-relocation-permit/internal/storage/journal"
)

func TestSnapshotRecoveryRestoresOperationalMetadata(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	now := time.Date(2026, 8, 26, 9, 30, 0, 0, time.UTC)

	repository, err := journal.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	firstCaseNumber, err := repository.NextCaseNumber(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	original, err := domain.NewCase("case-original", firstCaseNumber, now)
	if err != nil {
		t.Fatal(err)
	}
	const key = "restart-create-0001"
	if _, err := repository.Create(ctx, original, key, now); err != nil {
		t.Fatal(err)
	}
	firstPermitNumber, err := repository.NextPermitNumber(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := journal.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	nextCaseNumber, err := reopened.NextCaseNumber(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if nextCaseNumber == firstCaseNumber || !strings.HasSuffix(nextCaseNumber, "-0002") {
		t.Errorf("重启后档案编号应从 -0002 继续，首次为 %q，实际为 %q", firstCaseNumber, nextCaseNumber)
	}
	nextPermitNumber, err := reopened.NextPermitNumber(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if nextPermitNumber == firstPermitNumber || !strings.HasSuffix(nextPermitNumber, "-0002") {
		t.Errorf("重启后许可编号应从 -0002 继续，首次为 %q，实际为 %q", firstPermitNumber, nextPermitNumber)
	}

	retryState, err := domain.NewCase("case-retry-must-not-persist", "GQ-20260826-9999", now)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := reopened.Create(ctx, retryState, key, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Idempotent || replayed.State.CaseID != original.CaseID {
		t.Errorf("重启后的同键重试应返回原档案 %q，实际 idempotent=%t caseId=%q", original.CaseID, replayed.Idempotent, replayed.State.CaseID)
	}
	items, err := reopened.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Errorf("同键重试不应创建第二份档案，实际档案数 %d", len(items))
	}
}
