package journal

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"heritage-tree-relocation-permit/internal/domain"
)

func TestRepositoryRecoversSnapshotAndTimeline(t *testing.T) {
	directory := t.TempDir()
	repository, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	number, err := repository.NextCaseNumber(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	state, err := domain.NewCase("case-recovery", number, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Create(context.Background(), state, "recovery-create-0001", now); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	recovered, err := reopened.Get(context.Background(), state.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.CaseNumber != number || recovered.Version != 1 {
		t.Fatalf("恢复状态错误: %#v", recovered)
	}
	timeline, err := reopened.Timeline(context.Background(), state.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(timeline) != 1 || timeline[0].EventType != "case.created" {
		t.Fatalf("恢复时间线错误: %#v", timeline)
	}
}

func TestOpenRejectsTruncatedTailFrame(t *testing.T) {
	directory := t.TempDir()
	repository, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state, _ := domain.NewCase("case-corrupt", "GQ-20260820-0001", now)
	if _, err := repository.Create(context.Background(), state, "corrupt-create-0001", now); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(directory, "events.bin"), os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte{0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = Open(directory)
	if err == nil || !strings.Contains(err.Error(), "截断尾帧") {
		t.Fatalf("期望截断尾帧诊断，实际 %v", err)
	}
}
