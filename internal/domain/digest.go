package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
)

type digestRisk struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type permitDigestInput struct {
	TreeProfile TreeProfile           `json:"treeProfile"`
	Destination DestinationAssessment `json:"destination"`
	Revision    digestRevision        `json:"revision"`
	Checklist   digestChecklist       `json:"checklist"`
	Precheck    digestPrecheck        `json:"precheck"`
}

type digestRevision struct {
	RevisionID         string       `json:"revisionId"`
	RevisionNumber     int          `json:"revisionNumber"`
	RootBallDiameterCM float64      `json:"rootBallDiameterCm"`
	Measures           []string     `json:"measures"`
	RiskControls       []digestRisk `json:"riskControls"`
}

type digestChecklist struct {
	ChecklistID string          `json:"checklistId"`
	Version     int             `json:"version"`
	RevisionID  string          `json:"revisionId"`
	Items       []ChecklistItem `json:"items"`
}

type digestPrecheck struct {
	PrecheckID  string          `json:"precheckId"`
	ChecklistID string          `json:"checklistId"`
	RevisionID  string          `json:"revisionId"`
	Inspector   string          `json:"inspector"`
	CheckedAt   string          `json:"checkedAt"`
	Items       []ChecklistItem `json:"items"`
	Passed      bool            `json:"passed"`
}

func PermitDigest(c RelocationCase, revision MethodRevision, precheck PrecheckRecord) (string, error) {
	snapshot := PermitSnapshot{TreeProfile: c.TreeProfile, Destination: c.Destination, Revision: &revision, Precheck: &precheck}
	if c.FrozenChecklist != nil {
		checklist := cloneChecklist(*c.FrozenChecklist)
		snapshot.Checklist = &checklist
	}
	return PermitSnapshotDigest(snapshot)
}

func PermitSnapshotDigest(snapshot PermitSnapshot) (string, error) {
	if snapshot.TreeProfile == nil || snapshot.Destination == nil || snapshot.Revision == nil || snapshot.Precheck == nil {
		return "", errors.New("许可冻结字段缺失")
	}
	risks := make([]digestRisk, 0, len(snapshot.Revision.RiskControls))
	for key, value := range snapshot.Revision.RiskControls {
		risks = append(risks, digestRisk{Key: key, Value: value})
	}
	sort.Slice(risks, func(i, j int) bool { return risks[i].Key < risks[j].Key })
	checklist := digestChecklist{}
	if snapshot.Checklist != nil {
		checklist.ChecklistID = snapshot.Checklist.ChecklistID
		checklist.Version = snapshot.Checklist.Version
		checklist.RevisionID = snapshot.Checklist.RevisionID
		checklist.Items = sortedChecklistItems(snapshot.Checklist.Items)
	}
	input := permitDigestInput{
		TreeProfile: *snapshot.TreeProfile,
		Destination: *snapshot.Destination,
		Revision: digestRevision{
			RevisionID: snapshot.Revision.RevisionID, RevisionNumber: snapshot.Revision.RevisionNumber,
			RootBallDiameterCM: snapshot.Revision.RootBallDiameterCM,
			Measures:           []string{snapshot.Revision.ExcavationMeasures, snapshot.Revision.PackingMeasures, snapshot.Revision.TransportMeasures, snapshot.Revision.PlantingMeasures, snapshot.Revision.AftercareMeasures},
			RiskControls:       risks,
		},
		Checklist: checklist,
		Precheck: digestPrecheck{
			PrecheckID: snapshot.Precheck.PrecheckID, ChecklistID: snapshot.Precheck.ChecklistID,
			RevisionID: snapshot.Precheck.RevisionID, Inspector: snapshot.Precheck.Inspector,
			CheckedAt: snapshot.Precheck.CheckedAt.UTC().Format("2006-01-02T15:04:05.000000000Z"),
			Items:     sortedChecklistItems(snapshot.Precheck.Items), Passed: snapshot.Precheck.Passed,
		},
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func VerifyPermitDigest(permit WorkPermit) DigestVerification {
	result := DigestVerification{Stored: permit.ContentDigest}
	if snapshotMissingFields(permit.Snapshot) {
		result.Status, result.Message = "missing_fields", "许可冻结字段缺失，不可作为现场放行依据"
		return result
	}
	calculated, err := PermitSnapshotDigest(permit.Snapshot)
	if err != nil {
		result.Status, result.Message = "missing_fields", "许可冻结字段缺失，不可作为现场放行依据"
		return result
	}
	result.Calculated = calculated
	if calculated != permit.ContentDigest {
		result.Status, result.Message = "mismatch", "许可摘要复验不匹配，不可作为现场放行依据"
		return result
	}
	result.Status, result.Message = "matching", "许可冻结内容与摘要匹配"
	return result
}

func snapshotMissingFields(snapshot PermitSnapshot) bool {
	if snapshot.TreeProfile == nil || snapshot.Destination == nil || snapshot.Revision == nil || snapshot.Checklist == nil || snapshot.Precheck == nil {
		return true
	}
	return snapshot.TreeProfile.TreeProfileID == "" || snapshot.TreeProfile.SpeciesName == "" ||
		snapshot.Destination.DestinationAssessmentID == "" || snapshot.Destination.SiteName == "" ||
		snapshot.Revision.RevisionID == "" || snapshot.Revision.CaseID == "" ||
		snapshot.Checklist.ChecklistID == "" || len(snapshot.Checklist.Items) == 0 ||
		snapshot.Precheck.PrecheckID == "" || snapshot.Precheck.ChecklistID == "" || len(snapshot.Precheck.Items) == 0
}

func NewPermitSnapshot(c RelocationCase, revision MethodRevision, checklist FrozenChecklist, precheck PrecheckRecord) PermitSnapshot {
	tree, destination := *c.TreeProfile, *c.Destination
	revisionCopy := cloneRevision(revision)
	checklistCopy := cloneChecklist(checklist)
	precheckCopy := clonePrecheck(precheck)
	return PermitSnapshot{TreeProfile: &tree, Destination: &destination, Revision: &revisionCopy, Checklist: &checklistCopy, Precheck: &precheckCopy}
}

func snapshotMatchesCurrent(snapshot PermitSnapshot, c RelocationCase, revision MethodRevision, checklist FrozenChecklist, precheck PrecheckRecord) bool {
	want := NewPermitSnapshot(c, revision, checklist, precheck)
	return reflect.DeepEqual(snapshot, want)
}

func cloneRevision(source MethodRevision) MethodRevision {
	clone := source
	clone.RiskControls = make(map[string]string, len(source.RiskControls))
	for key, value := range source.RiskControls {
		clone.RiskControls[key] = value
	}
	return clone
}

func cloneChecklist(source FrozenChecklist) FrozenChecklist {
	clone := source
	clone.Items = append([]ChecklistItem(nil), source.Items...)
	return clone
}

func clonePrecheck(source PrecheckRecord) PrecheckRecord {
	clone := source
	clone.Items = append([]ChecklistItem(nil), source.Items...)
	return clone
}

func clonePermit(source WorkPermit) WorkPermit {
	clone := source
	clone.PrecheckSnapshot = clonePrecheck(source.PrecheckSnapshot)
	if source.Snapshot.TreeProfile != nil {
		value := *source.Snapshot.TreeProfile
		clone.Snapshot.TreeProfile = &value
	}
	if source.Snapshot.Destination != nil {
		value := *source.Snapshot.Destination
		clone.Snapshot.Destination = &value
	}
	if source.Snapshot.Revision != nil {
		revisionCopy := cloneRevision(*source.Snapshot.Revision)
		clone.Snapshot.Revision = &revisionCopy
	}
	if source.Snapshot.Checklist != nil {
		checklistCopy := cloneChecklist(*source.Snapshot.Checklist)
		clone.Snapshot.Checklist = &checklistCopy
	}
	if source.Snapshot.Precheck != nil {
		precheckCopy := clonePrecheck(*source.Snapshot.Precheck)
		clone.Snapshot.Precheck = &precheckCopy
	}
	return clone
}

func sortedChecklistItems(source []ChecklistItem) []ChecklistItem {
	items := append([]ChecklistItem(nil), source...)
	sort.Slice(items, func(i, j int) bool { return items[i].Code < items[j].Code })
	return items
}
