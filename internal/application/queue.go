package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"heritage-tree-relocation-permit/internal/domain"
)

const (
	defaultPageSize = 20
	maximumPageSize = 100
)

var cursorSigningKey = []byte("heritage-tree-relocation-permit:case-queue:v1")

type CaseQuery struct {
	Keyword  string
	Statuses []domain.CaseStatus
	Limit    int
	Cursor   string
}

type CaseQueueItem struct {
	domain.RelocationCase
	NextAction        string `json:"nextAction"`
	ActionTarget      string `json:"actionTarget"`
	OpenBlockingCount int    `json:"openBlockingCount"`
}

type CaseQueueSummary struct {
	Total    int            `json:"total"`
	ByStatus map[string]int `json:"byStatus"`
}

type CaseQueuePage struct {
	Items      []CaseQueueItem  `json:"items"`
	NextCursor string           `json:"nextCursor,omitempty"`
	Summary    CaseQueueSummary `json:"summary"`
}

type queueCursor struct {
	Offset    int    `json:"offset"`
	Filter    string `json:"filter"`
	Signature string `json:"signature"`
}

func (s *Service) QueryCases(ctx context.Context, query CaseQuery) (CaseQueuePage, error) {
	query.Keyword = strings.ToLower(strings.TrimSpace(query.Keyword))
	if query.Limit == 0 {
		query.Limit = defaultPageSize
	}
	if query.Limit < 1 || query.Limit > maximumPageSize {
		return CaseQueuePage{}, &ValidationError{Message: "limit 必须在 1 至 100 之间"}
	}
	statusSet := make(map[domain.CaseStatus]bool, len(query.Statuses))
	for _, status := range query.Statuses {
		if !status.Valid() {
			return CaseQueuePage{}, &ValidationError{Message: fmt.Sprintf("无效档案状态：%s", status)}
		}
		statusSet[status] = true
	}
	items, err := s.repository.List(ctx)
	if err != nil {
		return CaseQueuePage{}, err
	}
	queue := make([]CaseQueueItem, 0, len(items))
	for _, item := range items {
		if len(statusSet) > 0 && !statusSet[item.Status] {
			continue
		}
		if query.Keyword != "" && !matchesCaseKeyword(item, query.Keyword) {
			continue
		}
		action, target, _ := nextCaseAction(item)
		queue = append(queue, CaseQueueItem{RelocationCase: item, NextAction: action, ActionTarget: target, OpenBlockingCount: item.OpenBlockingFindingCount()})
	}
	sort.Slice(queue, func(i, j int) bool {
		_, _, leftPriority := nextCaseAction(queue[i].RelocationCase)
		_, _, rightPriority := nextCaseAction(queue[j].RelocationCase)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if queue[i].OpenBlockingCount != queue[j].OpenBlockingCount {
			return queue[i].OpenBlockingCount > queue[j].OpenBlockingCount
		}
		return queue[i].CaseNumber < queue[j].CaseNumber
	})
	filter := queryFingerprint(query.Keyword, query.Statuses)
	start := 0
	if query.Cursor != "" {
		cursor, err := decodeQueueCursor(query.Cursor, filter)
		if err != nil {
			return CaseQueuePage{}, &ValidationError{Message: "cursor 无效或已失效"}
		}
		if cursor.Offset < 0 || cursor.Offset > len(queue) {
			return CaseQueuePage{}, &ValidationError{Message: "cursor 无效或已失效"}
		}
		start = cursor.Offset
	}
	summary := CaseQueueSummary{Total: len(queue), ByStatus: make(map[string]int)}
	for _, item := range queue {
		summary.ByStatus[string(item.Status)]++
	}
	end := start + query.Limit
	if end > len(queue) {
		end = len(queue)
	}
	page := CaseQueuePage{Items: append([]CaseQueueItem(nil), queue[start:end]...), Summary: summary}
	if end < len(queue) && end > start {
		page.NextCursor = encodeQueueCursor(queueCursor{Offset: end, Filter: filter})
	}
	return page, nil
}

func matchesCaseKeyword(item domain.RelocationCase, keyword string) bool {
	values := []string{item.CaseNumber}
	if item.TreeProfile != nil {
		values = append(values, item.TreeProfile.SpeciesName)
	}
	if item.Destination != nil {
		values = append(values, item.Destination.SiteName)
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), keyword) {
			return true
		}
	}
	return false
}

func nextCaseAction(item domain.RelocationCase) (string, string, int) {
	switch item.Status {
	case domain.StatusCorrectionRequired:
		return fmt.Sprintf("整改并复核 %d 项阻断问题", item.OpenBlockingFindingCount()), "review", 0
	case domain.StatusPrecheckReady:
		if latest, ok := item.LatestPrecheck(); ok && !latest.Passed {
			return "复查现场核验失败项", "precheck", 1
		}
		if _, ok := item.LatestPassingPrecheck(); ok {
			return "预检资格并签发许可", "permit", 2
		}
		return "执行开工前现场核验", "precheck", 1
	case domain.StatusUnderReview:
		return "完成技术审查", "review", 2
	case domain.StatusAssessed:
		if item.ActiveRevisionID == "" {
			return "编制迁移保护方案", "revision", 3
		}
		return "提交技术审查", "review", 3
	case domain.StatusDraft:
		return "登记树体与迁入地评估", "assessment", 4
	case domain.StatusApproved:
		return "查看已签发许可", "permit", 5
	default:
		return "查看档案", "overview", 6
	}
}

func queryFingerprint(keyword string, statuses []domain.CaseStatus) string {
	values := make([]string, len(statuses))
	for i, status := range statuses {
		values[i] = string(status)
	}
	sort.Strings(values)
	sum := sha256.Sum256([]byte(keyword + "\x00" + strings.Join(values, ",")))
	return hex.EncodeToString(sum[:8])
}

func encodeQueueCursor(cursor queueCursor) string {
	cursor.Signature = ""
	payload, _ := json.Marshal(cursor)
	mac := hmac.New(sha256.New, cursorSigningKey)
	_, _ = mac.Write(payload)
	cursor.Signature = hex.EncodeToString(mac.Sum(nil))
	payload, _ = json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeQueueCursor(value, filter string) (queueCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return queueCursor{}, err
	}
	var cursor queueCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.Filter != filter || cursor.Offset < 0 {
		return queueCursor{}, fmt.Errorf("cursor payload invalid")
	}
	signature, err := hex.DecodeString(cursor.Signature)
	if err != nil {
		return queueCursor{}, err
	}
	cursor.Signature = ""
	unsigned, _ := json.Marshal(cursor)
	mac := hmac.New(sha256.New, cursorSigningKey)
	_, _ = mac.Write(unsigned)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return queueCursor{}, fmt.Errorf("cursor signature invalid")
	}
	return cursor, nil
}
