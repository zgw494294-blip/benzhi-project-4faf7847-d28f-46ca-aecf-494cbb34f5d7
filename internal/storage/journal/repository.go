package journal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"heritage-tree-relocation-permit/internal/application"
	"heritage-tree-relocation-permit/internal/domain"
)

type idempotencyResult struct {
	CaseID   string                `json:"caseId"`
	Sequence int64                 `json:"sequence"`
	Version  int64                 `json:"version"`
	State    domain.RelocationCase `json:"state"`
}

type Repository struct {
	mu            sync.RWMutex
	directory     string
	logPath       string
	snapshotPath  string
	logFile       *os.File
	cases         map[string]domain.RelocationCase
	idempotency   map[string]idempotencyResult
	timeline      map[string][]application.TimelineEntry
	lastSequence  int64
	lastDigest    string
	caseCounter   int64
	permitCounter int64
	closed        bool
}

func Open(directory string) (*Repository, error) {
	if directory == "" {
		return nil, errors.New("数据目录不能为空")
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}
	repository := &Repository{directory: directory, logPath: filepath.Join(directory, "events.bin"), snapshotPath: filepath.Join(directory, "snapshot.json"), cases: make(map[string]domain.RelocationCase), idempotency: make(map[string]idempotencyResult), timeline: make(map[string][]application.TimelineEntry)}
	if err := repository.recover(); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(repository.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return nil, fmt.Errorf("打开事件日志用于追加失败: %w", err)
	}
	repository.logFile = file
	return repository, nil
}

func (r *Repository) recover() error {
	records, err := scanLog(r.logPath)
	if err != nil {
		return err
	}
	snapshot, err := readSnapshot(r.snapshotPath)
	if err != nil {
		return err
	}
	startSequence := int64(0)
	if snapshot != nil {
		if snapshot.LastSequence > int64(len(records)) {
			return fmt.Errorf("快照序号超过事件日志末尾")
		}
		if snapshot.LastSequence > 0 && records[snapshot.LastSequence-1].Checksum != snapshot.LastDigest {
			return fmt.Errorf("快照摘要与事件日志不一致")
		}
		r.cases = snapshot.Cases
		if r.cases == nil {
			r.cases = make(map[string]domain.RelocationCase)
		}
		r.idempotency = snapshot.Idempotency
		if r.idempotency == nil {
			r.idempotency = make(map[string]idempotencyResult)
		}
		r.caseCounter = snapshot.CaseCounter
		r.permitCounter = snapshot.PermitCounter
		startSequence = snapshot.LastSequence
	}
	for _, record := range records {
		r.timeline[record.CaseID] = append(r.timeline[record.CaseID], application.TimelineEntry{Sequence: record.Sequence, EventType: record.EventType, CaseID: record.CaseID, CaseVersion: record.CaseVersion, IdempotencyKey: record.IdempotencyKey, OccurredAt: record.OccurredAt, Summary: record.Summary})
		if record.Sequence <= startSequence {
			continue
		}
		r.cases[record.CaseID] = record.State
		r.idempotency[record.IdempotencyKey] = idempotencyResult{CaseID: record.CaseID, Sequence: record.Sequence, Version: record.CaseVersion, State: record.State}
		if record.EventType == "case.created" {
			r.caseCounter++
		}
		if record.EventType == "permit.issued" {
			r.permitCounter++
		}
	}
	if len(records) > 0 {
		r.lastSequence = records[len(records)-1].Sequence
		r.lastDigest = records[len(records)-1].Checksum
	}
	return nil
}

func (r *Repository) Create(ctx context.Context, state domain.RelocationCase, key string, occurredAt time.Time) (application.CommitResult, error) {
	select {
	case <-ctx.Done():
		return application.CommitResult{}, ctx.Err()
	default:
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if result, ok := r.idempotency[key]; ok {
		return r.idempotentResult(result)
	}
	if _, exists := r.cases[state.CaseID]; exists {
		return application.CommitResult{}, application.ErrDuplicate
	}
	return r.commitLocked(application.CommitRequest{CaseID: state.CaseID, ExpectedVersion: 0, IdempotencyKey: key, EventType: "case.created", Summary: "建立古树迁移档案", OccurredAt: occurredAt, State: state})
}

func (r *Repository) Get(ctx context.Context, caseID string) (domain.RelocationCase, error) {
	select {
	case <-ctx.Done():
		return domain.RelocationCase{}, ctx.Err()
	default:
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	state, ok := r.cases[caseID]
	if !ok {
		return domain.RelocationCase{}, application.ErrNotFound
	}
	return state, nil
}

func (r *Repository) List(ctx context.Context) ([]domain.RelocationCase, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]domain.RelocationCase, 0, len(r.cases))
	for _, state := range r.cases {
		items = append(items, state)
	}
	return items, nil
}

func (r *Repository) Commit(ctx context.Context, request application.CommitRequest) (application.CommitResult, error) {
	select {
	case <-ctx.Done():
		return application.CommitResult{}, ctx.Err()
	default:
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if result, ok := r.idempotency[request.IdempotencyKey]; ok {
		return r.idempotentResult(result)
	}
	current, ok := r.cases[request.CaseID]
	if !ok {
		return application.CommitResult{}, application.ErrNotFound
	}
	if current.Version != request.ExpectedVersion {
		return application.CommitResult{}, &application.VersionConflictError{Expected: request.ExpectedVersion, Actual: current.Version}
	}
	if request.EventType == "" {
		return application.CommitResult{}, &application.VersionConflictError{Expected: request.ExpectedVersion, Actual: current.Version}
	}
	if request.State.Version != current.Version+1 {
		return application.CommitResult{}, fmt.Errorf("提交状态版本必须递增一位")
	}
	return r.commitLocked(request)
}

func (r *Repository) commitLocked(request application.CommitRequest) (application.CommitResult, error) {
	record := eventRecord{SchemaVersion: schemaVersion, Sequence: r.lastSequence + 1, PreviousDigest: r.lastDigest, CaseID: request.CaseID, CaseVersion: request.State.Version, IdempotencyKey: request.IdempotencyKey, EventType: request.EventType, Summary: request.Summary, OccurredAt: request.OccurredAt.UTC(), State: request.State}
	checksum, err := record.calculateChecksum()
	if err != nil {
		return application.CommitResult{}, err
	}
	record.Checksum = checksum
	if err := appendFrame(r.logFile, record); err != nil {
		return application.CommitResult{}, err
	}
	r.cases[record.CaseID] = record.State
	r.idempotency[record.IdempotencyKey] = idempotencyResult{CaseID: record.CaseID, Sequence: record.Sequence, Version: record.CaseVersion, State: record.State}
	r.timeline[record.CaseID] = append(r.timeline[record.CaseID], application.TimelineEntry{Sequence: record.Sequence, EventType: record.EventType, CaseID: record.CaseID, CaseVersion: record.CaseVersion, IdempotencyKey: record.IdempotencyKey, OccurredAt: record.OccurredAt, Summary: record.Summary})
	r.lastSequence = record.Sequence
	r.lastDigest = record.Checksum
	if record.EventType == "case.created" && r.caseCounter == 0 {
		r.caseCounter = 1
	}
	if err := r.writeSnapshotLocked(); err != nil {
		return application.CommitResult{}, fmt.Errorf("事件已落盘但快照更新失败: %w", err)
	}
	return application.CommitResult{State: record.State, Sequence: record.Sequence}, nil
}

func (r *Repository) idempotentResult(result idempotencyResult) (application.CommitResult, error) {
	if _, ok := r.cases[result.CaseID]; !ok {
		return application.CommitResult{}, fmt.Errorf("幂等记录引用的档案不存在")
	}
	return application.CommitResult{State: result.State, Sequence: result.Sequence, Idempotent: true}, nil
}

func (r *Repository) Timeline(ctx context.Context, caseID string) ([]application.TimelineEntry, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.cases[caseID]; !ok {
		return nil, application.ErrNotFound
	}
	entries := append([]application.TimelineEntry(nil), r.timeline[caseID]...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Sequence < entries[j].Sequence })
	return entries, nil
}

func (r *Repository) NextCaseNumber(ctx context.Context, now time.Time) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.caseCounter++
	return fmt.Sprintf("GQ-%s-%04d", now.Format("20060102"), r.caseCounter), nil
}

func (r *Repository) NextPermitNumber(ctx context.Context, now time.Time) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.permitCounter++
	return fmt.Sprintf("XK-%s-%04d", now.Format("20060102"), r.permitCounter), nil
}

func (r *Repository) writeSnapshotLocked() error {
	return writeSnapshot(r.snapshotPath, snapshotPayload{SchemaVersion: schemaVersion, LastSequence: r.lastSequence, LastDigest: r.lastDigest, CaseCounter: r.caseCounter, PermitCounter: r.permitCounter, Cases: r.cases, Idempotency: r.idempotency})
}

func (r *Repository) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	if err := r.writeSnapshotLocked(); err != nil {
		return err
	}
	if r.logFile != nil {
		return r.logFile.Close()
	}
	return nil
}
