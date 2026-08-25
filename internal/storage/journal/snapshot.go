package journal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"heritage-tree-relocation-permit/internal/domain"
)

type snapshotPayload struct {
	SchemaVersion int                              `json:"schemaVersion"`
	LastSequence  int64                            `json:"lastSequence"`
	LastDigest    string                           `json:"lastDigest"`
	CaseCounter   int64                            `json:"caseCounter"`
	PermitCounter int64                            `json:"permitCounter"`
	Cases         map[string]domain.RelocationCase `json:"cases"`
	Idempotency   map[string]idempotencyResult     `json:"idempotency"`
}

type snapshotFile struct {
	Checksum string          `json:"checksum"`
	Payload  snapshotPayload `json:"payload"`
}

func snapshotChecksum(payload snapshotPayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func readSnapshot(path string) (*snapshotPayload, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取投影快照失败: %w", err)
	}
	var file snapshotFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("解析投影快照失败: %w", err)
	}
	if file.Payload.SchemaVersion != schemaVersion {
		return nil, fmt.Errorf("投影快照 schemaVersion 未知: %d", file.Payload.SchemaVersion)
	}
	want, err := snapshotChecksum(file.Payload)
	if err != nil {
		return nil, err
	}
	if want != file.Checksum {
		return nil, fmt.Errorf("投影快照校验和错误")
	}
	return &file.Payload, nil
}

func writeSnapshot(path string, payload snapshotPayload) error {
	checksum, err := snapshotChecksum(payload)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshotFile{Checksum: checksum, Payload: payload}, "", "  ")
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	temp, err := os.CreateTemp(directory, ".snapshot-*.tmp")
	if err != nil {
		return fmt.Errorf("创建快照临时文件失败: %w", err)
	}
	tempName := temp.Name()
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(tempName)
		}
	}()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("写入快照失败: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("同步快照失败: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("关闭快照失败: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("原子替换快照失败: %w", err)
	}
	remove = false
	dir, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("打开快照目录失败: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("同步快照目录失败: %w", err)
	}
	return nil
}
