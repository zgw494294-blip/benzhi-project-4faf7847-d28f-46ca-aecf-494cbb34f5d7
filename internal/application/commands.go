package application

import (
	"time"

	"heritage-tree-relocation-permit/internal/domain"
)

type CreateCaseCommand struct {
	IdempotencyKey string `json:"idempotencyKey"`
}

type AssessmentCommand struct {
	ExpectedVersion int64                        `json:"expectedVersion"`
	IdempotencyKey  string                       `json:"idempotencyKey"`
	Tree            domain.TreeProfile           `json:"tree"`
	Destination     domain.DestinationAssessment `json:"destination"`
}

type RevisionCommand struct {
	ExpectedVersion         int64             `json:"expectedVersion"`
	IdempotencyKey          string            `json:"idempotencyKey"`
	CarryFromRevisionNumber int               `json:"carryFromRevisionNumber,omitempty"`
	RootBallDiameterCM      float64           `json:"rootBallDiameterCm"`
	ExcavationMeasures      string            `json:"excavationMeasures"`
	PackingMeasures         string            `json:"packingMeasures"`
	TransportMeasures       string            `json:"transportMeasures"`
	PlantingMeasures        string            `json:"plantingMeasures"`
	AftercareMeasures       string            `json:"aftercareMeasures"`
	RiskControls            map[string]string `json:"riskControls"`
}

type FindingInput struct {
	Severity    domain.Severity `json:"severity"`
	Category    string          `json:"category"`
	Description string          `json:"description"`
}

type ReviewCommand struct {
	ExpectedVersion int64          `json:"expectedVersion"`
	IdempotencyKey  string         `json:"idempotencyKey"`
	Findings        []FindingInput `json:"findings"`
}

type EvidenceCommand struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	IdempotencyKey  string `json:"idempotencyKey"`
	Evidence        string `json:"evidence"`
	Submitter       string `json:"submitter"`
}

type VerifyFindingCommand struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	IdempotencyKey  string `json:"idempotencyKey"`
	Reviewer        string `json:"reviewer"`
	Opinion         string `json:"opinion"`
	Accepted        bool   `json:"accepted"`
}

type PrecheckCommand struct {
	ExpectedVersion int64                  `json:"expectedVersion"`
	IdempotencyKey  string                 `json:"idempotencyKey"`
	Inspector       string                 `json:"inspector"`
	ChecklistID     string                 `json:"checklistId"`
	CheckedAt       time.Time              `json:"checkedAt"`
	Items           []domain.ChecklistItem `json:"items"`
}

type IssuePermitCommand struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	IdempotencyKey  string `json:"idempotencyKey"`
	IssuedBy        string `json:"issuedBy"`
}
