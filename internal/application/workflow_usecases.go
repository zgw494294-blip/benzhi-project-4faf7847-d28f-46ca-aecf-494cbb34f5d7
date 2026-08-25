package application

import (
	"context"
	"errors"
	"strings"

	"heritage-tree-relocation-permit/internal/domain"
)

func (s *Service) SubmitReview(ctx context.Context, caseID string, command ReviewCommand) (domain.RelocationCase, error) {
	return s.mutate(ctx, caseID, command.ExpectedVersion, command.IdempotencyKey, "review.submitted", "提交技术审查结论", func(c *domain.RelocationCase) error {
		findings := make([]domain.ReviewFinding, 0, len(command.Findings))
		for _, input := range command.Findings {
			findings = append(findings, domain.ReviewFinding{FindingID: s.ids.NewID("finding"), CaseID: c.CaseID, RevisionID: c.ActiveRevisionID, Severity: input.Severity, Category: strings.TrimSpace(input.Category), Description: strings.TrimSpace(input.Description), Status: domain.FindingOpen})
		}
		return c.SubmitReview(findings, s.clock.Now())
	})
}

func (s *Service) SubmitEvidence(ctx context.Context, caseID, findingID string, command EvidenceCommand) (domain.RelocationCase, error) {
	return s.mutate(ctx, caseID, command.ExpectedVersion, command.IdempotencyKey, "finding.evidence_submitted", "提交问题整改证据", func(c *domain.RelocationCase) error {
		return c.SubmitEvidenceBy(findingID, command.Evidence, command.Submitter, s.clock.Now())
	})
}

func (s *Service) VerifyFinding(ctx context.Context, caseID, findingID string, command VerifyFindingCommand) (domain.RelocationCase, error) {
	return s.mutate(ctx, caseID, command.ExpectedVersion, command.IdempotencyKey, "finding.verified", "复核问题整改结果", func(c *domain.RelocationCase) error {
		return c.VerifyFindingWithOpinion(findingID, command.Reviewer, command.Opinion, command.Accepted, s.clock.Now())
	})
}

func (s *Service) RecordPrecheck(ctx context.Context, caseID string, command PrecheckCommand) (domain.RelocationCase, error) {
	return s.mutate(ctx, caseID, command.ExpectedVersion, command.IdempotencyKey, "precheck.recorded", "完成开工前现场核验", func(c *domain.RelocationCase) error {
		checkedAt := command.CheckedAt
		if checkedAt.IsZero() {
			checkedAt = s.clock.Now()
		}
		record := domain.PrecheckRecord{PrecheckID: s.ids.NewID("precheck"), ChecklistID: command.ChecklistID, RevisionID: c.ActiveRevisionID, Inspector: strings.TrimSpace(command.Inspector), CheckedAt: checkedAt.UTC(), Items: command.Items}
		return c.RecordPrecheck(record, s.clock.Now())
	})
}

func (s *Service) IssuePermit(ctx context.Context, caseID string, command IssuePermitCommand) (domain.RelocationCase, error) {
	if strings.TrimSpace(command.IssuedBy) == "" {
		return domain.RelocationCase{}, &ValidationError{Message: "issuedBy 不能为空"}
	}
	if err := ValidateIdempotencyKey(command.IdempotencyKey); err != nil {
		return domain.RelocationCase{}, err
	}
	unlock, err := s.lock(ctx, caseID)
	if err != nil {
		return domain.RelocationCase{}, err
	}
	defer unlock()
	current, err := s.repository.Get(ctx, caseID)
	if err != nil {
		return domain.RelocationCase{}, err
	}
	if err := checkVersion(current, command.ExpectedVersion); err != nil {
		result, commitErr := s.repository.Commit(ctx, CommitRequest{CaseID: caseID, ExpectedVersion: command.ExpectedVersion, IdempotencyKey: command.IdempotencyKey})
		if commitErr == nil && result.Idempotent {
			return result.State, nil
		}
		return domain.RelocationCase{}, err
	}
	now := s.clock.Now()
	eligibility := current.PermitEligibilityAt(now)
	if !eligibility.Eligible {
		return domain.RelocationCase{}, eligibility.RuleError()
	}
	revision, ok := current.ActiveRevision()
	if !ok {
		return domain.RelocationCase{}, errors.New("当前方案修订不存在")
	}
	precheck, ok := current.LatestPassingPrecheck()
	if !ok {
		return domain.RelocationCase{}, errors.New("尚无合格的现场核验")
	}
	snapshot := domain.NewPermitSnapshot(current, revision, *current.FrozenChecklist, precheck)
	digest, err := domain.PermitSnapshotDigest(snapshot)
	if err != nil {
		return domain.RelocationCase{}, err
	}
	permitNumber, err := s.repository.NextPermitNumber(ctx, now)
	if err != nil {
		return domain.RelocationCase{}, err
	}
	permit := domain.WorkPermit{PermitID: s.ids.NewID("permit"), PermitNumber: permitNumber, CaseID: current.CaseID, FrozenRevisionID: revision.RevisionID, PrecheckSnapshot: precheck, Snapshot: snapshot, ContentDigest: digest, IssuedBy: strings.TrimSpace(command.IssuedBy), IssuedAt: now}
	updated := cloneCase(current)
	if err := updated.IssuePermit(permit, now); err != nil {
		return domain.RelocationCase{}, err
	}
	result, err := s.repository.Commit(ctx, CommitRequest{CaseID: caseID, ExpectedVersion: command.ExpectedVersion, IdempotencyKey: command.IdempotencyKey, EventType: "permit.issued", Summary: "冻结获批内容并签发作业许可", OccurredAt: now, State: updated})
	if err != nil {
		return domain.RelocationCase{}, err
	}
	return result.State, nil
}

func (s *Service) CurrentChecklist(ctx context.Context, caseID string) (domain.FrozenChecklist, error) {
	item, err := s.GetCase(ctx, caseID)
	if err != nil {
		return domain.FrozenChecklist{}, err
	}
	if item.FrozenChecklist == nil {
		return domain.FrozenChecklist{}, errors.New("当前冻结核验清单不存在")
	}
	return *item.FrozenChecklist, nil
}

func (s *Service) PermitEligibility(ctx context.Context, caseID string) (domain.PermitEligibility, error) {
	item, err := s.GetCase(ctx, caseID)
	if err != nil {
		return domain.PermitEligibility{}, err
	}
	return item.PermitEligibilityAt(s.clock.Now()), nil
}

type PermitView struct {
	domain.WorkPermit
	DigestVerification domain.DigestVerification `json:"digestVerification"`
}

func (s *Service) GetPermit(ctx context.Context, caseID string) (PermitView, error) {
	item, err := s.GetCase(ctx, caseID)
	if err != nil {
		return PermitView{}, err
	}
	if item.Permit == nil {
		return PermitView{}, errors.New("许可尚未签发")
	}
	return PermitView{WorkPermit: *item.Permit, DigestVerification: domain.VerifyPermitDigest(*item.Permit)}, nil
}
