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

// queueCursor 锁定"上一页最后一条"在稳定排序中的位置。
//
// 采用键集游标而非数值偏移：保存最后返回项的排序键
// （优先级、开放阻断数、档案编号），恢复时返回排序严格靠后的条目。
// 这样即使队列因评估/审查导致状态变化而重排，或服务重启重建队列，
// 也不会重复返回已经分页过的档案，签名校验仍然有效。
type queueCursor struct {
	// CursorAfter 指示游标指向的最后一条排序键；首页请求时为空。
	CursorAfter *caseSortKey `json:"after,omitempty"`
	Filter      string       `json:"filter"`
	Signature   string       `json:"signature,omitempty"`
}

// caseSortKey 复刻 QueryCases 中的稳定排序顺序：
// 优先级升序 → 开放阻断数降序 → 档案编号升序。
type caseSortKey struct {
	Priority          int    `json:"priority"`
	OpenBlockingCount int    `json:"openBlockingCount"`
	CaseNumber        string `json:"caseNumber"`
}

func caseSortKeyOf(item CaseQueueItem) caseSortKey {
	_, _, priority := nextCaseAction(item.RelocationCase)
	return caseSortKey{Priority: priority, OpenBlockingCount: item.OpenBlockingCount, CaseNumber: item.CaseNumber}
}

// sortAfter 报告 candidate 是否在 key 的稳定排序之后（严格靠后）。
// 与 QueryCases 的 sort.Slice 顺序保持一致。
func caseSortAfter(key caseSortKey, candidate CaseQueueItem) bool {
	ck := caseSortKeyOf(candidate)
	if ck.Priority != key.Priority {
		return ck.Priority > key.Priority
	}
	if ck.OpenBlockingCount != key.OpenBlockingCount {
		// 阻断数大的靠前，故大的"之前"，小的"之后"
		return ck.OpenBlockingCount < key.OpenBlockingCount
	}
	return ck.CaseNumber > key.CaseNumber
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
		if cursor.CursorAfter != nil {
			// 键集翻页：跳过排序不严格靠后的条目，避免重复返回已分页档案。
			start = len(queue)
			for i, item := range queue {
				if caseSortAfter(*cursor.CursorAfter, item) {
					start = i
					break
				}
			}
		}
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
		page.NextCursor = encodeQueueCursor(queueCursor{CursorAfter: ptrCaseSortKey(caseSortKeyOf(queue[end-1])), Filter: filter})
	}
	return page, nil
}

func ptrCaseSortKey(key caseSortKey) *caseSortKey { return &key }

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
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.Filter != filter {
		return queueCursor{}, fmt.Errorf("cursor payload invalid")
	}
	if cursor.CursorAfter != nil && cursor.CursorAfter.CaseNumber == "" {
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
