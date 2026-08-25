package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"heritage-tree-relocation-permit/internal/application"
	"heritage-tree-relocation-permit/internal/domain"
)

const maxRequestBody = 1 << 20

type errorResponse struct {
	Code          string             `json:"code"`
	Message       string             `json:"message"`
	CorrelationID string             `json:"correlationId"`
	Violations    []domain.Violation `json:"violations,omitempty"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return &application.ValidationError{Message: "请求体超过 1 MiB 限制"}
		}
		if errors.Is(err, io.EOF) {
			return &application.ValidationError{Message: "请求体不能为空"}
		}
		return &application.ValidationError{Message: "JSON 请求体无效: " + err.Error()}
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return &application.ValidationError{Message: "请求体只能包含一个 JSON 对象"}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	message := "服务处理请求时发生错误"
	switch {
	case errors.Is(err, application.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", err.Error()
	case errors.Is(err, application.ErrConflict):
		status, code, message = http.StatusConflict, "version_conflict", err.Error()
	case errors.Is(err, application.ErrDuplicate):
		status, code, message = http.StatusConflict, "duplicate", err.Error()
	case errors.Is(err, application.ErrInvalidCommand):
		status, code, message = http.StatusBadRequest, "invalid_request", err.Error()
	case errors.Is(err, domain.ErrPermitImmutable):
		status, code, message = http.StatusConflict, "permit_immutable", err.Error()
	default:
		var ruleErr *domain.RuleError
		if errors.As(err, &ruleErr) {
			status, code, message = http.StatusUnprocessableEntity, "rule_violation", ruleErr.Error()
		}
		if status == http.StatusInternalServerError && isBusinessError(err) {
			status, code, message = http.StatusUnprocessableEntity, "business_rule", err.Error()
		}
	}
	response := errorResponse{Code: code, Message: message, CorrelationID: correlationID(r.Context())}
	var ruleErr *domain.RuleError
	if errors.As(err, &ruleErr) {
		response.Violations = ruleErr.Violations
	}
	writeJSON(w, status, response)
}

func isBusinessError(err error) bool {
	message := err.Error()
	markers := []string{"状态 ", "必须", "不能", "尚未", "不存在", "不允许", "不一致", "缺少", "无效", "过短", "未关闭"}
	for _, marker := range markers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
