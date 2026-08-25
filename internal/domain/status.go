package domain

import "fmt"

type CaseStatus string

const (
	StatusDraft              CaseStatus = "draft"
	StatusAssessed           CaseStatus = "assessed"
	StatusUnderReview        CaseStatus = "under_review"
	StatusCorrectionRequired CaseStatus = "correction_required"
	StatusPrecheckReady      CaseStatus = "precheck_ready"
	StatusApproved           CaseStatus = "approved"
)

func (s CaseStatus) Valid() bool {
	switch s {
	case StatusDraft, StatusAssessed, StatusUnderReview, StatusCorrectionRequired, StatusPrecheckReady, StatusApproved:
		return true
	default:
		return false
	}
}

func ensureStatus(actual CaseStatus, allowed ...CaseStatus) error {
	for _, candidate := range allowed {
		if actual == candidate {
			return nil
		}
	}
	return fmt.Errorf("状态 %s 不允许执行该操作", actual)
}

type FindingStatus string

const (
	FindingOpen      FindingStatus = "open"
	FindingSubmitted FindingStatus = "submitted"
	FindingClosed    FindingStatus = "closed"
	FindingRejected  FindingStatus = "rejected"
)

type Severity string

const (
	SeverityAdvisory Severity = "advisory"
	SeverityBlocking Severity = "blocking"
)
