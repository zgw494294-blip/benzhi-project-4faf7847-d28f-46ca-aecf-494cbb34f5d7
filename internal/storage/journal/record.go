package journal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"heritage-tree-relocation-permit/internal/domain"
)

const schemaVersion = 1

type eventRecord struct {
	SchemaVersion  int                   `json:"schemaVersion"`
	Sequence       int64                 `json:"sequence"`
	PreviousDigest string                `json:"previousDigest"`
	Checksum       string                `json:"checksum"`
	CaseID         string                `json:"caseId"`
	CaseVersion    int64                 `json:"caseVersion"`
	IdempotencyKey string                `json:"idempotencyKey"`
	EventType      string                `json:"eventType"`
	Summary        string                `json:"summary"`
	OccurredAt     time.Time             `json:"occurredAt"`
	State          domain.RelocationCase `json:"state"`
}

type checksumRecord struct {
	SchemaVersion  int                   `json:"schemaVersion"`
	Sequence       int64                 `json:"sequence"`
	PreviousDigest string                `json:"previousDigest"`
	CaseID         string                `json:"caseId"`
	CaseVersion    int64                 `json:"caseVersion"`
	IdempotencyKey string                `json:"idempotencyKey"`
	EventType      string                `json:"eventType"`
	Summary        string                `json:"summary"`
	OccurredAt     time.Time             `json:"occurredAt"`
	State          domain.RelocationCase `json:"state"`
}

func (record eventRecord) calculateChecksum() (string, error) {
	payload, err := json.Marshal(checksumRecord{record.SchemaVersion, record.Sequence, record.PreviousDigest, record.CaseID, record.CaseVersion, record.IdempotencyKey, record.EventType, record.Summary, record.OccurredAt, record.State})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func (record eventRecord) validate(previous string, sequence int64) error {
	if record.SchemaVersion != schemaVersion {
		return fmt.Errorf("未知 schemaVersion %d，事件序号 %d", record.SchemaVersion, record.Sequence)
	}
	if record.Sequence != sequence {
		return fmt.Errorf("事件序号不连续：期望 %d，实际 %d", sequence, record.Sequence)
	}
	if record.PreviousDigest != previous {
		return fmt.Errorf("事件摘要链断裂：序号 %d", record.Sequence)
	}
	want, err := record.calculateChecksum()
	if err != nil {
		return err
	}
	if record.Checksum != want {
		return fmt.Errorf("事件校验和错误：序号 %d", record.Sequence)
	}
	if record.State.CaseID != record.CaseID || record.State.Version != record.CaseVersion {
		return fmt.Errorf("事件状态引用不一致：序号 %d", record.Sequence)
	}
	return nil
}
