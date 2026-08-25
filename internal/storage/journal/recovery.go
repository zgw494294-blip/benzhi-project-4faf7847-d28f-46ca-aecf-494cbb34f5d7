package journal

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const maxFrameSize = 16 << 20

func scanLog(path string) ([]eventRecord, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("打开事件日志失败: %w", err)
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	var records []eventRecord
	previous := ""
	sequence := int64(1)
	for {
		var length uint64
		err := binary.Read(reader, binary.BigEndian, &length)
		if errors.Is(err, io.EOF) {
			break
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("事件日志存在截断尾帧：长度前缀不完整")
		}
		if err != nil {
			return nil, fmt.Errorf("读取事件帧长度失败: %w", err)
		}
		if length == 0 || length > maxFrameSize {
			return nil, fmt.Errorf("事件帧长度无效: %d", length)
		}
		payload := make([]byte, int(length))
		if _, err := io.ReadFull(reader, payload); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, fmt.Errorf("事件日志存在截断尾帧：序号 %d", sequence)
			}
			return nil, fmt.Errorf("读取事件帧失败: %w", err)
		}
		var record eventRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			return nil, fmt.Errorf("解析事件帧失败：序号 %d: %w", sequence, err)
		}
		if err := record.validate(previous, sequence); err != nil {
			return nil, err
		}
		records = append(records, record)
		previous = record.Checksum
		sequence++
	}
	return records, nil
}

func appendFrame(file *os.File, record eventRecord) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("编码事件失败: %w", err)
	}
	if len(payload) > maxFrameSize {
		return fmt.Errorf("事件帧超过最大限制")
	}
	if err := binary.Write(file, binary.BigEndian, uint64(len(payload))); err != nil {
		return fmt.Errorf("写入事件帧长度失败: %w", err)
	}
	if _, err := file.Write(payload); err != nil {
		return fmt.Errorf("写入事件帧失败: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("同步事件日志失败: %w", err)
	}
	return nil
}
