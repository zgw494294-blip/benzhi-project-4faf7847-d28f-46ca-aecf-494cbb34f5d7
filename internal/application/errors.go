package application

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound       = errors.New("档案不存在")
	ErrConflict       = errors.New("版本冲突")
	ErrDuplicate      = errors.New("资源已存在")
	ErrInvalidCommand = errors.New("命令参数无效")
)

type VersionConflictError struct {
	Expected int64
	Actual   int64
}

func (e *VersionConflictError) Error() string {
	return fmt.Sprintf("版本冲突：期望 %d，当前 %d", e.Expected, e.Actual)
}
func (e *VersionConflictError) Unwrap() error { return ErrConflict }

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalidCommand }
