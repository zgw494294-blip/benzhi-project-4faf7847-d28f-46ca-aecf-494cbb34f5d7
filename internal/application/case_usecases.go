package application

import (
	"context"
	"strings"

	"heritage-tree-relocation-permit/internal/domain"
)

func (s *Service) CreateCase(ctx context.Context, command CreateCaseCommand) (domain.RelocationCase, error) {
	if err := ValidateIdempotencyKey(command.IdempotencyKey); err != nil {
		return domain.RelocationCase{}, err
	}
	now := s.clock.Now()
	number, err := s.repository.NextCaseNumber(ctx, now)
	if err != nil {
		return domain.RelocationCase{}, mapRepositoryError(err)
	}
	created, err := domain.NewCase(s.ids.NewID("case"), number, now)
	if err != nil {
		return domain.RelocationCase{}, err
	}
	result, err := s.repository.Create(ctx, created, command.IdempotencyKey, now)
	if err != nil {
		return domain.RelocationCase{}, mapRepositoryError(err)
	}
	return result.State, nil
}

func (s *Service) RecordAssessments(ctx context.Context, caseID string, command AssessmentCommand) (domain.RelocationCase, error) {
	if command.Tree.TreeProfileID == "" {
		command.Tree.TreeProfileID = s.ids.NewID("tree")
	}
	if command.Destination.DestinationAssessmentID == "" {
		command.Destination.DestinationAssessmentID = s.ids.NewID("site")
	}
	return s.mutate(ctx, caseID, command.ExpectedVersion, command.IdempotencyKey, "assessments.recorded", "完成树体与迁入地评估", func(c *domain.RelocationCase) error {
		for _, historical := range c.TreeProfileHistory {
			if historical.TreeProfileID == command.Tree.TreeProfileID {
				command.Tree.TreeProfileID = s.ids.NewID("tree")
				break
			}
		}
		for _, historical := range c.DestinationHistory {
			if historical.DestinationAssessmentID == command.Destination.DestinationAssessmentID {
				command.Destination.DestinationAssessmentID = s.ids.NewID("site")
				break
			}
		}
		return c.RecordAssessments(command.Tree, command.Destination, s.clock.Now())
	})
}

func (s *Service) AddRevision(ctx context.Context, caseID string, command RevisionCommand) (domain.RelocationCase, error) {
	return s.mutate(ctx, caseID, command.ExpectedVersion, command.IdempotencyKey, "revision.created", "编制迁移保护方案修订", func(c *domain.RelocationCase) error {
		if command.CarryFromRevisionNumber > 0 {
			source, ok := c.RevisionByNumber(command.CarryFromRevisionNumber)
			if !ok || source.RevisionNumber != len(c.Revisions) {
				return &ValidationError{Message: "carryFromRevisionNumber 必须引用最近一版方案修订"}
			}
			if command.RootBallDiameterCM == 0 {
				command.RootBallDiameterCM = source.RootBallDiameterCM
			}
			if strings.TrimSpace(command.ExcavationMeasures) == "" {
				command.ExcavationMeasures = source.ExcavationMeasures
			}
			if strings.TrimSpace(command.PackingMeasures) == "" {
				command.PackingMeasures = source.PackingMeasures
			}
			if strings.TrimSpace(command.TransportMeasures) == "" {
				command.TransportMeasures = source.TransportMeasures
			}
			if strings.TrimSpace(command.PlantingMeasures) == "" {
				command.PlantingMeasures = source.PlantingMeasures
			}
			if strings.TrimSpace(command.AftercareMeasures) == "" {
				command.AftercareMeasures = source.AftercareMeasures
			}
			controls := make(map[string]string, len(source.RiskControls)+len(command.RiskControls))
			for key, value := range source.RiskControls {
				controls[key] = value
			}
			for key, value := range command.RiskControls {
				controls[key] = value
			}
			command.RiskControls = controls
		}
		revision := domain.MethodRevision{
			RevisionID: s.ids.NewID("rev"), CaseID: c.CaseID, RevisionNumber: len(c.Revisions) + 1,
			RootBallDiameterCM: command.RootBallDiameterCM, ExcavationMeasures: strings.TrimSpace(command.ExcavationMeasures),
			PackingMeasures: strings.TrimSpace(command.PackingMeasures), TransportMeasures: strings.TrimSpace(command.TransportMeasures),
			PlantingMeasures: strings.TrimSpace(command.PlantingMeasures), AftercareMeasures: strings.TrimSpace(command.AftercareMeasures),
			RiskControls: command.RiskControls, CreatedAt: s.clock.Now(),
		}
		return c.AddRevision(revision, s.clock.Now())
	})
}

func (s *Service) CompareRevisions(ctx context.Context, caseID string, from, to int) (domain.RevisionComparison, error) {
	item, err := s.GetCase(ctx, caseID)
	if err != nil {
		return domain.RevisionComparison{}, err
	}
	return item.CompareRevisions(from, to)
}
