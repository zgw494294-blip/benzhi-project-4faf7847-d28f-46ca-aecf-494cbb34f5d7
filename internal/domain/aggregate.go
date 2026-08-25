package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

var ErrPermitImmutable = errors.New("许可签发后档案不可修改")

func NewCase(id, number string, now time.Time) (RelocationCase, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(number) == "" {
		return RelocationCase{}, errors.New("档案标识和编号不能为空")
	}
	return RelocationCase{
		CaseID: id, CaseNumber: number, Status: StatusDraft, Version: 1,
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(), TreeProfileHistory: []TreeProfile{},
		DestinationHistory: []DestinationAssessment{}, Revisions: []MethodRevision{},
		Findings: []ReviewFinding{}, Prechecks: []PrecheckRecord{},
	}, nil
}

func (c *RelocationCase) ensureMutable() error {
	if c.Status == StatusApproved || c.Permit != nil {
		return ErrPermitImmutable
	}
	return nil
}

func (c *RelocationCase) RecordAssessments(tree TreeProfile, destination DestinationAssessment, now time.Time) error {
	if err := c.ensureMutable(); err != nil {
		return err
	}
	if err := ensureStatus(c.Status, StatusDraft, StatusAssessed, StatusCorrectionRequired, StatusPrecheckReady); err != nil {
		return err
	}
	if err := ValidateAssessments(tree, destination, now); err != nil {
		return err
	}
	if len(c.TreeProfileHistory) == 0 && c.TreeProfile != nil {
		c.TreeProfileHistory = append(c.TreeProfileHistory, *c.TreeProfile)
	}
	if len(c.DestinationHistory) == 0 && c.Destination != nil {
		c.DestinationHistory = append(c.DestinationHistory, *c.Destination)
	}
	c.TreeProfile = &tree
	c.Destination = &destination
	c.TreeProfileID = tree.TreeProfileID
	c.DestinationAssessmentID = destination.DestinationAssessmentID
	c.TreeProfileHistory = append(c.TreeProfileHistory, tree)
	c.DestinationHistory = append(c.DestinationHistory, destination)
	// 新现场事实使旧方案、旧问题和旧清单不再具备签发资格，但历史记录仍完整保留。
	c.ActiveRevisionID = ""
	c.FrozenChecklist = nil
	c.ReviewSummary = ReviewSummary{}
	c.Status = StatusAssessed
	c.touch(now)
	return nil
}

func (c *RelocationCase) AddRevision(revision MethodRevision, now time.Time) error {
	if err := c.ensureMutable(); err != nil {
		return err
	}
	if err := ensureStatus(c.Status, StatusAssessed, StatusCorrectionRequired); err != nil {
		return err
	}
	if c.TreeProfile == nil || c.Destination == nil {
		return errors.New("档案尚未完成评估")
	}
	if revision.CaseID != c.CaseID {
		return errors.New("方案修订引用了其他档案")
	}
	if err := ValidateRevision(*c.TreeProfile, *c.Destination, revision); err != nil {
		return err
	}
	want := len(c.Revisions) + 1
	if revision.RevisionNumber != want {
		return fmt.Errorf("方案修订序号必须为 %d", want)
	}
	c.Revisions = append(c.Revisions, revision)
	c.ActiveRevisionID = revision.RevisionID
	c.FrozenChecklist = nil
	c.ReviewSummary = ReviewSummary{}
	c.Status = StatusAssessed
	c.touch(now)
	return nil
}

func (c *RelocationCase) SubmitReview(findings []ReviewFinding, now time.Time) error {
	if err := c.ensureMutable(); err != nil {
		return err
	}
	if err := ensureStatus(c.Status, StatusAssessed); err != nil {
		return err
	}
	if c.ActiveRevisionID == "" {
		return errors.New("必须先编制迁移方案")
	}
	if err := c.ValidateAssessmentValidity(now); err != nil {
		return err
	}
	seenIDs := make(map[string]bool)
	seenContent := make(map[string]int)
	marked := make(map[int]bool)
	var violations []Violation
	for i := range findings {
		finding := &findings[i]
		finding.Category = normalizeText(finding.Category)
		finding.Description = normalizeText(finding.Description)
		if finding.FindingID == "" || finding.Description == "" || finding.Category == "" {
			violations = append(violations, Violation{"REVIEW_FINDING_REQUIRED", fmt.Sprintf("findings[%d]", i), fmt.Sprintf("第 %d 项审查问题缺少必要字段", i+1)})
			continue
		}
		if finding.RevisionID != c.ActiveRevisionID || finding.CaseID != c.CaseID {
			violations = append(violations, Violation{"REVIEW_REVISION_MISMATCH", fmt.Sprintf("findings[%d].revisionId", i), fmt.Sprintf("第 %d 项必须引用当前方案修订", i+1)})
		}
		if finding.Severity != SeverityAdvisory && finding.Severity != SeverityBlocking {
			violations = append(violations, Violation{"REVIEW_SEVERITY_INVALID", fmt.Sprintf("findings[%d].severity", i), fmt.Sprintf("第 %d 项严重性无效", i+1)})
		}
		if seenIDs[finding.FindingID] {
			violations = append(violations, Violation{"REVIEW_FINDING_ID_DUPLICATE", fmt.Sprintf("findings[%d].findingId", i), fmt.Sprintf("第 %d 项审查问题标识重复", i+1)})
		}
		seenIDs[finding.FindingID] = true
		key := string(finding.Severity) + "\x00" + normalizedDuplicateKey(finding.Category) + "\x00" + normalizedDuplicateKey(finding.Description)
		if first, ok := seenContent[key]; ok {
			if !marked[first] {
				violations = append(violations, Violation{"REVIEW_FINDING_DUPLICATE", fmt.Sprintf("findings[%d].description", first), fmt.Sprintf("第 %d 项与第 %d 项重复", first+1, i+1)})
				marked[first] = true
			}
			violations = append(violations, Violation{"REVIEW_FINDING_DUPLICATE", fmt.Sprintf("findings[%d].description", i), fmt.Sprintf("第 %d 项与第 %d 项重复", i+1, first+1)})
		} else {
			seenContent[key] = i
		}
	}
	if err := NewRuleError(violations); err != nil {
		return err
	}
	sortFindings(findings)
	blocking, advisory := 0, 0
	for _, finding := range findings {
		if finding.Severity == SeverityBlocking {
			blocking++
		} else {
			advisory++
		}
	}
	c.Findings = append(c.Findings, findings...)
	sortFindings(c.Findings)
	c.ReviewSummary = ReviewSummary{RevisionID: c.ActiveRevisionID, BlockingCount: blocking, AdvisoryCount: advisory, Conclusion: "通过"}
	if blocking > 0 {
		c.ReviewSummary.Conclusion = "需要整改"
		c.Status = StatusCorrectionRequired
	} else {
		c.Status = StatusPrecheckReady
		c.freezeChecklist(now)
	}
	c.touch(now)
	return nil
}

// SubmitEvidence 保留原有领域调用形式；应用入口使用 SubmitEvidenceBy 强制记录提交人。
func (c *RelocationCase) SubmitEvidence(findingID, evidence string, now time.Time) error {
	return c.SubmitEvidenceBy(findingID, evidence, "未记录提交人", now)
}

func (c *RelocationCase) SubmitEvidenceBy(findingID, evidence, submitter string, now time.Time) error {
	if err := c.ensureMutable(); err != nil {
		return err
	}
	if err := ensureStatus(c.Status, StatusCorrectionRequired); err != nil {
		return err
	}
	evidence, submitter = strings.TrimSpace(evidence), strings.TrimSpace(submitter)
	if len([]rune(evidence)) < 4 {
		return errors.New("整改证据内容过短")
	}
	if submitter == "" {
		return errors.New("整改证据提交人不能为空")
	}
	idx := c.findingIndex(findingID)
	if idx < 0 {
		return errors.New("审查问题不存在")
	}
	finding := &c.Findings[idx]
	if finding.RevisionID != c.ActiveRevisionID {
		return errors.New("审查问题不属于当前方案修订")
	}
	if finding.Status == FindingClosed {
		return errors.New("已关闭问题不能再次提交证据")
	}
	if finding.Status == FindingSubmitted {
		return errors.New("当前轮次证据尚待复核")
	}
	round := EvidenceRound{Round: len(finding.EvidenceRounds) + 1, Body: evidence, SubmittedBy: submitter, SubmittedAt: now.UTC()}
	finding.EvidenceRounds = append(finding.EvidenceRounds, round)
	finding.RemediationEvidence = evidence
	finding.Status = FindingSubmitted
	c.touch(now)
	return nil
}

// VerifyFinding 保留原调用形式并生成明确的兼容意见。
func (c *RelocationCase) VerifyFinding(findingID, reviewer string, accepted bool, now time.Time) error {
	opinion := "整改证据符合关闭要求"
	if !accepted {
		opinion = "整改证据未通过复核"
	}
	return c.VerifyFindingWithOpinion(findingID, reviewer, opinion, accepted, now)
}

func (c *RelocationCase) VerifyFindingWithOpinion(findingID, reviewer, opinion string, accepted bool, now time.Time) error {
	if err := c.ensureMutable(); err != nil {
		return err
	}
	if err := ensureStatus(c.Status, StatusCorrectionRequired); err != nil {
		return err
	}
	reviewer, opinion = strings.TrimSpace(reviewer), strings.TrimSpace(opinion)
	if reviewer == "" {
		return errors.New("复核人员不能为空")
	}
	if opinion == "" {
		return errors.New("复核意见不能为空")
	}
	idx := c.findingIndex(findingID)
	if idx < 0 {
		return errors.New("审查问题不存在")
	}
	finding := &c.Findings[idx]
	if finding.RevisionID != c.ActiveRevisionID {
		return errors.New("审查问题不属于当前方案修订")
	}
	if finding.Status != FindingSubmitted || len(finding.EvidenceRounds) == 0 {
		return errors.New("问题尚未提交整改证据")
	}
	reviewedAt := now.UTC()
	round := &finding.EvidenceRounds[len(finding.EvidenceRounds)-1]
	if round.Decision != "" {
		return errors.New("当前证据轮次已经复核")
	}
	round.ReviewedBy, round.ReviewedAt, round.Opinion = reviewer, &reviewedAt, opinion
	finding.ReviewedBy, finding.ReviewedAt = reviewer, &reviewedAt
	if accepted {
		round.Decision = "accepted"
		finding.Status = FindingClosed
	} else {
		round.Decision = "rejected"
		finding.Status = FindingRejected
	}
	if c.allBlockingFindingsClosed() {
		c.Status = StatusPrecheckReady
		c.freezeChecklist(now)
	}
	c.touch(now)
	return nil
}

func (c *RelocationCase) RecordPrecheck(record PrecheckRecord, now time.Time) error {
	if err := c.ensureMutable(); err != nil {
		return err
	}
	if err := ensureStatus(c.Status, StatusPrecheckReady); err != nil {
		return err
	}
	if c.FrozenChecklist == nil || c.FrozenChecklist.RevisionID != c.ActiveRevisionID {
		return errors.New("当前冻结核验清单不存在")
	}
	if record.ChecklistID != c.FrozenChecklist.ChecklistID {
		return errors.New("现场核验必须引用当前冻结清单")
	}
	if record.RevisionID != c.ActiveRevisionID {
		return errors.New("现场核验必须引用当前冻结方案")
	}
	if strings.TrimSpace(record.Inspector) == "" {
		return errors.New("现场核验人员不能为空")
	}
	passed, err := ValidatePrecheckItems(*c.FrozenChecklist, record.Items)
	if err != nil {
		return err
	}
	byCode := make(map[string]ChecklistItem, len(record.Items))
	for _, item := range record.Items {
		byCode[item.Code] = item
	}
	normalized := make([]ChecklistItem, 0, len(c.FrozenChecklist.Items))
	for _, templateItem := range c.FrozenChecklist.Items {
		submitted := byCode[templateItem.Code]
		templateItem.Passed = submitted.Passed
		templateItem.Note = strings.TrimSpace(submitted.Note)
		normalized = append(normalized, templateItem)
	}
	record.Items = normalized
	record.Passed = passed
	c.Prechecks = append(c.Prechecks, record)
	c.touch(now)
	return nil
}

func (c *RelocationCase) IssuePermit(permit WorkPermit, now time.Time) error {
	if err := c.ensureMutable(); err != nil {
		return err
	}
	eligibility := c.PermitEligibilityAt(now)
	if !eligibility.Eligible {
		return eligibility.RuleError()
	}
	revision, _ := c.ActiveRevision()
	precheck, _ := c.LatestPassingPrecheck()
	if permit.FrozenRevisionID != revision.RevisionID || permit.PrecheckSnapshot.PrecheckID != precheck.PrecheckID {
		return errors.New("许可冻结内容与当前档案不一致")
	}
	if !snapshotMatchesCurrent(permit.Snapshot, *c, revision, *c.FrozenChecklist, precheck) {
		return errors.New("许可完整冻结快照与当前合格内容不一致")
	}
	digest, err := PermitSnapshotDigest(permit.Snapshot)
	if err != nil {
		return err
	}
	if permit.ContentDigest != digest {
		return errors.New("许可内容摘要不匹配")
	}
	copyPermit := clonePermit(permit)
	c.Permit = &copyPermit
	c.Status = StatusApproved
	c.touch(now)
	return nil
}

func (c RelocationCase) ActiveRevision() (MethodRevision, bool) {
	for i := len(c.Revisions) - 1; i >= 0; i-- {
		if c.Revisions[i].RevisionID == c.ActiveRevisionID && c.Revisions[i].CaseID == c.CaseID {
			return c.Revisions[i], true
		}
	}
	return MethodRevision{}, false
}

func (c RelocationCase) LatestPassingPrecheck() (PrecheckRecord, bool) {
	if c.FrozenChecklist == nil {
		return PrecheckRecord{}, false
	}
	for i := len(c.Prechecks) - 1; i >= 0; i-- {
		if c.Prechecks[i].ChecklistID != c.FrozenChecklist.ChecklistID || c.Prechecks[i].RevisionID != c.ActiveRevisionID {
			continue
		}
		if c.Prechecks[i].Passed {
			return c.Prechecks[i], true
		}
		return PrecheckRecord{}, false
	}
	return PrecheckRecord{}, false
}

func (c RelocationCase) LatestPrecheck() (PrecheckRecord, bool) {
	if c.FrozenChecklist == nil {
		return PrecheckRecord{}, false
	}
	for i := len(c.Prechecks) - 1; i >= 0; i-- {
		if c.Prechecks[i].ChecklistID == c.FrozenChecklist.ChecklistID && c.Prechecks[i].RevisionID == c.ActiveRevisionID {
			return c.Prechecks[i], true
		}
	}
	return PrecheckRecord{}, false
}

func (c RelocationCase) SortedFindings() []ReviewFinding {
	result := append([]ReviewFinding(nil), c.Findings...)
	sortFindings(result)
	return result
}

func sortFindings(findings []ReviewFinding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity == SeverityBlocking
		}
		if findings[i].Category != findings[j].Category {
			return findings[i].Category < findings[j].Category
		}
		return findings[i].FindingID < findings[j].FindingID
	})
}

func normalizeText(value string) string { return strings.Join(strings.Fields(value), " ") }

func normalizedDuplicateKey(value string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value))
}

func (c *RelocationCase) freezeChecklist(now time.Time) {
	revision, ok := c.ActiveRevision()
	if !ok {
		c.FrozenChecklist = nil
		return
	}
	checklist := BuildFrozenChecklist(revision, now)
	c.FrozenChecklist = &checklist
}

func (c *RelocationCase) findingIndex(id string) int {
	for i := range c.Findings {
		if c.Findings[i].FindingID == id {
			return i
		}
	}
	return -1
}

func (c RelocationCase) allBlockingFindingsClosed() bool {
	for _, finding := range c.Findings {
		if finding.RevisionID == c.ActiveRevisionID && finding.Severity == SeverityBlocking && finding.Status != FindingClosed {
			return false
		}
	}
	return true
}

func (c RelocationCase) OpenBlockingFindingCount() int {
	count := 0
	for _, finding := range c.Findings {
		if finding.RevisionID == c.ActiveRevisionID && finding.Severity == SeverityBlocking && finding.Status != FindingClosed {
			count++
		}
	}
	return count
}

func (c *RelocationCase) touch(now time.Time) { c.Version++; c.UpdatedAt = now.UTC() }
