package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"heritage-tree-relocation-permit/internal/domain"
)

var idempotencyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{7,127}$`)

type Service struct {
	repository Repository
	clock      Clock
	ids        IDGenerator
	locksMu    sync.Mutex
	locks      map[string]*sync.Mutex
	caseListMu sync.RWMutex
	caseList   []domain.RelocationCase
}

func NewService(repository Repository, clock Clock, ids IDGenerator) *Service {
	if clock == nil {
		clock = RealClock{}
	}
	if ids == nil {
		ids = randomIDs{}
	}
	return &Service{repository: repository, clock: clock, ids: ids, locks: make(map[string]*sync.Mutex)}
}

type randomIDs struct{}

func (randomIDs) NewID(prefix string) string {
	data := make([]byte, 12)
	if _, err := rand.Read(data); err != nil {
		panic(fmt.Sprintf("生成随机标识失败: %v", err))
	}
	return prefix + "_" + hex.EncodeToString(data)
}

func ValidateIdempotencyKey(key string) error {
	if !idempotencyPattern.MatchString(key) {
		return &ValidationError{Message: "idempotencyKey 必须为 8 至 128 个安全字符"}
	}
	return nil
}

func (s *Service) lock(caseID string) func() {
	s.locksMu.Lock()
	mu := s.locks[caseID]
	if mu == nil {
		mu = &sync.Mutex{}
		s.locks[caseID] = mu
	}
	s.locksMu.Unlock()
	mu.Lock()
	return mu.Unlock
}

func (s *Service) GetCase(ctx context.Context, caseID string) (domain.RelocationCase, error) {
	if strings.TrimSpace(caseID) == "" {
		return domain.RelocationCase{}, &ValidationError{Message: "caseId 不能为空"}
	}
	return s.repository.Get(ctx, caseID)
}

type CaseView struct {
	domain.RelocationCase
	AssessmentValidity []domain.AssessmentValidity `json:"assessmentValidity"`
}

func (s *Service) GetCaseView(ctx context.Context, caseID string) (CaseView, error) {
	item, err := s.GetCase(ctx, caseID)
	if err != nil {
		return CaseView{}, err
	}
	return CaseView{RelocationCase: item, AssessmentValidity: item.AssessmentValidities(s.clock.Now())}, nil
}

func (s *Service) ListCases(ctx context.Context) ([]domain.RelocationCase, error) {
	s.caseListMu.RLock()
	if s.caseList != nil {
		items := cloneCases(s.caseList)
		s.caseListMu.RUnlock()
		return items, nil
	}
	s.caseListMu.RUnlock()

	items, err := s.repository.List(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	s.caseListMu.Lock()
	if s.caseList == nil {
		s.caseList = cloneCases(items)
	}
	items = cloneCases(s.caseList)
	s.caseListMu.Unlock()
	return items, nil
}

// invalidateCaseList drops the cached case list so the next ListCases call
// re-reads fresh state from the repository. Any successful write (create or
// commit) may have changed persisted state, so the cache must not survive it
// within the same Service lifecycle.
func (s *Service) invalidateCaseList() {
	s.caseListMu.Lock()
	s.caseList = nil
	s.caseListMu.Unlock()
}

func (s *Service) Timeline(ctx context.Context, caseID string) ([]TimelineEntry, error) {
	return s.repository.Timeline(ctx, caseID)
}

func checkVersion(c domain.RelocationCase, expected int64) error {
	if expected <= 0 {
		return &ValidationError{Message: "expectedVersion 必须大于零"}
	}
	if c.Version != expected {
		return &VersionConflictError{Expected: expected, Actual: c.Version}
	}
	return nil
}

func (s *Service) mutate(ctx context.Context, caseID string, expected int64, key, eventType, summary string, change func(*domain.RelocationCase) error) (domain.RelocationCase, error) {
	if err := ValidateIdempotencyKey(key); err != nil {
		return domain.RelocationCase{}, err
	}
	unlock := s.lock(caseID)
	defer unlock()
	current, err := s.repository.Get(ctx, caseID)
	if err != nil {
		return domain.RelocationCase{}, err
	}
	if err := checkVersion(current, expected); err != nil {
		// Repository may already know this key; allow it to return the original result.
		result, commitErr := s.repository.Commit(ctx, CommitRequest{CaseID: caseID, ExpectedVersion: expected, IdempotencyKey: key})
		if commitErr == nil && result.Idempotent {
			s.invalidateCaseList()
			return result.State, nil
		}
		return domain.RelocationCase{}, err
	}
	updated := cloneCase(current)
	if err := change(&updated); err != nil {
		return domain.RelocationCase{}, err
	}
	result, err := s.repository.Commit(ctx, CommitRequest{CaseID: caseID, ExpectedVersion: expected, IdempotencyKey: key, EventType: eventType, Summary: summary, OccurredAt: s.clock.Now(), State: updated})
	if err != nil {
		return domain.RelocationCase{}, err
	}
	s.invalidateCaseList()
	return result.State, nil
}

func cloneCase(source domain.RelocationCase) domain.RelocationCase {
	clone := source
	if source.TreeProfile != nil {
		value := *source.TreeProfile
		clone.TreeProfile = &value
	}
	if source.Destination != nil {
		value := *source.Destination
		clone.Destination = &value
	}
	clone.Revisions = append([]domain.MethodRevision(nil), source.Revisions...)
	for i := range clone.Revisions {
		controls := make(map[string]string, len(clone.Revisions[i].RiskControls))
		for key, value := range clone.Revisions[i].RiskControls {
			controls[key] = value
		}
		clone.Revisions[i].RiskControls = controls
	}
	clone.Findings = append([]domain.ReviewFinding(nil), source.Findings...)
	for i := range clone.Findings {
		clone.Findings[i].EvidenceRounds = append([]domain.EvidenceRound(nil), source.Findings[i].EvidenceRounds...)
	}
	clone.TreeProfileHistory = append([]domain.TreeProfile(nil), source.TreeProfileHistory...)
	clone.DestinationHistory = append([]domain.DestinationAssessment(nil), source.DestinationHistory...)
	if source.FrozenChecklist != nil {
		value := *source.FrozenChecklist
		value.Items = append([]domain.ChecklistItem(nil), source.FrozenChecklist.Items...)
		clone.FrozenChecklist = &value
	}
	clone.Prechecks = append([]domain.PrecheckRecord(nil), source.Prechecks...)
	for i := range clone.Prechecks {
		clone.Prechecks[i].Items = append([]domain.ChecklistItem(nil), clone.Prechecks[i].Items...)
	}
	if source.Permit != nil {
		value := *source.Permit
		value.PrecheckSnapshot.Items = append([]domain.ChecklistItem(nil), source.Permit.PrecheckSnapshot.Items...)
		if source.Permit.Snapshot.TreeProfile != nil {
			copyValue := *source.Permit.Snapshot.TreeProfile
			value.Snapshot.TreeProfile = &copyValue
		}
		if source.Permit.Snapshot.Destination != nil {
			copyValue := *source.Permit.Snapshot.Destination
			value.Snapshot.Destination = &copyValue
		}
		if source.Permit.Snapshot.Revision != nil {
			copyValue := *source.Permit.Snapshot.Revision
			copyValue.RiskControls = make(map[string]string, len(source.Permit.Snapshot.Revision.RiskControls))
			for key, entry := range source.Permit.Snapshot.Revision.RiskControls {
				copyValue.RiskControls[key] = entry
			}
			value.Snapshot.Revision = &copyValue
		}
		if source.Permit.Snapshot.Checklist != nil {
			copyValue := *source.Permit.Snapshot.Checklist
			copyValue.Items = append([]domain.ChecklistItem(nil), source.Permit.Snapshot.Checklist.Items...)
			value.Snapshot.Checklist = &copyValue
		}
		if source.Permit.Snapshot.Precheck != nil {
			copyValue := *source.Permit.Snapshot.Precheck
			copyValue.Items = append([]domain.ChecklistItem(nil), source.Permit.Snapshot.Precheck.Items...)
			value.Snapshot.Precheck = &copyValue
		}
		clone.Permit = &value
	}
	return clone
}

func cloneCases(source []domain.RelocationCase) []domain.RelocationCase {
	clones := make([]domain.RelocationCase, len(source))
	for i := range source {
		clones[i] = cloneCase(source[i])
	}
	return clones
}

func mapRepositoryError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) || errors.Is(err, ErrDuplicate) {
		return err
	}
	return fmt.Errorf("持久化操作失败: %w", err)
}
