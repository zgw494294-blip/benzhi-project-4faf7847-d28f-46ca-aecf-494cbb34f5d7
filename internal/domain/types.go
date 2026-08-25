package domain

import "time"

type RelocationCase struct {
	CaseID                  string                  `json:"caseId"`
	CaseNumber              string                  `json:"caseNumber"`
	TreeProfileID           string                  `json:"treeProfileId,omitempty"`
	DestinationAssessmentID string                  `json:"destinationAssessmentId,omitempty"`
	ActiveRevisionID        string                  `json:"activeRevisionId,omitempty"`
	Status                  CaseStatus              `json:"status"`
	Version                 int64                   `json:"version"`
	CreatedAt               time.Time               `json:"createdAt"`
	UpdatedAt               time.Time               `json:"updatedAt"`
	TreeProfile             *TreeProfile            `json:"treeProfile,omitempty"`
	Destination             *DestinationAssessment  `json:"destination,omitempty"`
	TreeProfileHistory      []TreeProfile           `json:"treeProfileHistory"`
	DestinationHistory      []DestinationAssessment `json:"destinationHistory"`
	Revisions               []MethodRevision        `json:"revisions"`
	Findings                []ReviewFinding         `json:"findings"`
	ReviewSummary           ReviewSummary           `json:"reviewSummary"`
	FrozenChecklist         *FrozenChecklist        `json:"frozenChecklist,omitempty"`
	Prechecks               []PrecheckRecord        `json:"prechecks"`
	Permit                  *WorkPermit             `json:"permit,omitempty"`
}

type TreeProfile struct {
	TreeProfileID   string    `json:"treeProfileId"`
	SpeciesName     string    `json:"speciesName"`
	ProtectionGrade string    `json:"protectionGrade"`
	TrunkDiameterCM float64   `json:"trunkDiameterCm"`
	CrownRadiusM    float64   `json:"crownRadiusM"`
	HealthGrade     string    `json:"healthGrade"`
	RootSurvey      string    `json:"rootSurvey"`
	AssessedAt      time.Time `json:"assessedAt"`
	Assessor        string    `json:"assessor"`
}

type DestinationAssessment struct {
	DestinationAssessmentID string    `json:"destinationAssessmentId"`
	SiteName                string    `json:"siteName"`
	AvailableRadiusM        float64   `json:"availableRadiusM"`
	SoilType                string    `json:"soilType"`
	SoilPH                  float64   `json:"soilPH"`
	DrainageGrade           string    `json:"drainageGrade"`
	RouteClearanceM         float64   `json:"routeClearanceM"`
	AssessedAt              time.Time `json:"assessedAt"`
	Assessor                string    `json:"assessor"`
}

type MethodRevision struct {
	RevisionID         string            `json:"revisionId"`
	CaseID             string            `json:"caseId"`
	RevisionNumber     int               `json:"revisionNumber"`
	RootBallDiameterCM float64           `json:"rootBallDiameterCm"`
	ExcavationMeasures string            `json:"excavationMeasures"`
	PackingMeasures    string            `json:"packingMeasures"`
	TransportMeasures  string            `json:"transportMeasures"`
	PlantingMeasures   string            `json:"plantingMeasures"`
	AftercareMeasures  string            `json:"aftercareMeasures"`
	RiskControls       map[string]string `json:"riskControls"`
	CreatedAt          time.Time         `json:"createdAt"`
}

type ReviewFinding struct {
	FindingID           string          `json:"findingId"`
	CaseID              string          `json:"caseId"`
	RevisionID          string          `json:"revisionId"`
	Severity            Severity        `json:"severity"`
	Category            string          `json:"category"`
	Description         string          `json:"description"`
	Status              FindingStatus   `json:"status"`
	RemediationEvidence string          `json:"remediationEvidence,omitempty"`
	ReviewedBy          string          `json:"reviewedBy,omitempty"`
	ReviewedAt          *time.Time      `json:"reviewedAt,omitempty"`
	EvidenceRounds      []EvidenceRound `json:"evidenceRounds"`
}

type EvidenceRound struct {
	Round       int        `json:"round"`
	Body        string     `json:"body"`
	SubmittedBy string     `json:"submittedBy"`
	SubmittedAt time.Time  `json:"submittedAt"`
	Decision    string     `json:"decision,omitempty"`
	ReviewedBy  string     `json:"reviewedBy,omitempty"`
	ReviewedAt  *time.Time `json:"reviewedAt,omitempty"`
	Opinion     string     `json:"opinion,omitempty"`
}

type ReviewSummary struct {
	RevisionID    string `json:"revisionId,omitempty"`
	BlockingCount int    `json:"blockingCount"`
	AdvisoryCount int    `json:"advisoryCount"`
	Conclusion    string `json:"conclusion,omitempty"`
}

type ChecklistItem struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	Passed   bool   `json:"passed"`
	Note     string `json:"note,omitempty"`
}

type FrozenChecklist struct {
	ChecklistID string          `json:"checklistId"`
	Version     int             `json:"version"`
	RevisionID  string          `json:"revisionId"`
	Items       []ChecklistItem `json:"items"`
	FrozenAt    time.Time       `json:"frozenAt"`
}

type PrecheckRecord struct {
	PrecheckID  string          `json:"precheckId"`
	ChecklistID string          `json:"checklistId"`
	RevisionID  string          `json:"revisionId"`
	Inspector   string          `json:"inspector"`
	CheckedAt   time.Time       `json:"checkedAt"`
	Items       []ChecklistItem `json:"items"`
	Passed      bool            `json:"passed"`
}

type PermitSnapshot struct {
	TreeProfile *TreeProfile           `json:"treeProfile,omitempty"`
	Destination *DestinationAssessment `json:"destination,omitempty"`
	Revision    *MethodRevision        `json:"revision,omitempty"`
	Checklist   *FrozenChecklist       `json:"checklist,omitempty"`
	Precheck    *PrecheckRecord        `json:"precheck,omitempty"`
}

type WorkPermit struct {
	PermitID         string         `json:"permitId"`
	PermitNumber     string         `json:"permitNumber"`
	CaseID           string         `json:"caseId"`
	FrozenRevisionID string         `json:"frozenRevisionId"`
	PrecheckSnapshot PrecheckRecord `json:"precheckSnapshot"`
	Snapshot         PermitSnapshot `json:"snapshot"`
	ContentDigest    string         `json:"contentDigest"`
	IssuedBy         string         `json:"issuedBy"`
	IssuedAt         time.Time      `json:"issuedAt"`
}

type AssessmentValidity struct {
	Kind          string    `json:"kind"`
	AssessedAt    time.Time `json:"assessedAt,omitempty"`
	ValidUntil    time.Time `json:"validUntil,omitempty"`
	RemainingDays int       `json:"remainingDays"`
	Conclusion    string    `json:"conclusion"`
}

type EligibilityItem struct {
	Code    string `json:"code"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
	Target  string `json:"target,omitempty"`
}

type PermitEligibility struct {
	Eligible bool              `json:"eligible"`
	Version  int64             `json:"version"`
	Items    []EligibilityItem `json:"items"`
}

type DigestVerification struct {
	Status     string `json:"status"`
	Stored     string `json:"stored"`
	Calculated string `json:"calculated,omitempty"`
	Message    string `json:"message"`
}
