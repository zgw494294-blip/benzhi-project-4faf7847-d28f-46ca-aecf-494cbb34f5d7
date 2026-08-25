package domain

import (
	"fmt"
	"sort"
	"time"
)

func (c RelocationCase) PermitEligibilityAt(now time.Time) PermitEligibility {
	items := []EligibilityItem{
		{Code: "ACTIVE_REVISION", Passed: false, Message: "当前活动方案不存在", Target: "revision"},
		{Code: "BLOCKING_FINDINGS", Passed: c.allBlockingFindingsClosed(), Message: "仍有未关闭的阻断问题", Target: "review"},
		{Code: "CASE_STATUS", Passed: c.Status == StatusPrecheckReady, Message: "档案尚未进入待现场核验状态", Target: "overview"},
		{Code: "DESTINATION_ASSESSMENT_VALID", Passed: false, Message: "迁入地评估已过期或缺失", Target: "assessment"},
		{Code: "FROZEN_CHECKLIST", Passed: false, Message: "当前冻结核验清单不存在", Target: "precheck"},
		{Code: "LATEST_PRECHECK", Passed: false, Message: "最近一次现场核验未全部合格", Target: "precheck"},
		{Code: "RISK_CONTROLS", Passed: false, Message: "活动方案未覆盖当前高风险控制", Target: "revision"},
		{Code: "TREE_ASSESSMENT_VALID", Passed: false, Message: "树体评估已过期或缺失", Target: "assessment"},
	}
	revision, hasRevision := c.ActiveRevision()
	items[0].Passed = hasRevision
	if hasRevision {
		items[0].Message = "当前活动方案存在"
	}
	validities := c.AssessmentValidities(now)
	for _, validity := range validities {
		passed := validity.Conclusion == "有效" || validity.Conclusion == "临期"
		if validity.Kind == "tree" {
			items[7].Passed = passed
			if passed {
				items[7].Message = fmt.Sprintf("树体评估%s，剩余 %d 天", validity.Conclusion, validity.RemainingDays)
			}
		} else {
			items[3].Passed = passed
			if passed {
				items[3].Message = fmt.Sprintf("迁入地评估%s，剩余 %d 天", validity.Conclusion, validity.RemainingDays)
			}
		}
	}
	if hasRevision && c.TreeProfile != nil && c.Destination != nil {
		items[6].Passed = ValidateRevision(*c.TreeProfile, *c.Destination, revision) == nil
		if items[6].Passed {
			items[6].Message = "活动方案已覆盖当前高风险控制"
		}
	}
	items[4].Passed = c.FrozenChecklist != nil && hasRevision && c.FrozenChecklist.RevisionID == revision.RevisionID
	if items[4].Passed {
		items[4].Message = "冻结核验清单与活动方案一致"
	}
	_, items[5].Passed = c.LatestPassingPrecheck()
	if items[5].Passed {
		items[5].Message = "最近一次现场核验全部合格"
	}
	if items[1].Passed {
		items[1].Message = "当前方案无未关闭阻断问题"
	}
	if items[2].Passed {
		items[2].Message = "档案状态允许签发"
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Code < items[j].Code })
	result := PermitEligibility{Eligible: true, Version: c.Version, Items: items}
	for _, item := range items {
		if !item.Passed {
			result.Eligible = false
		}
	}
	return result
}

func (e PermitEligibility) RuleError() error {
	var violations []Violation
	for _, item := range e.Items {
		if !item.Passed {
			violations = append(violations, Violation{Code: "PERMIT_" + item.Code, Field: item.Target, Message: item.Message})
		}
	}
	return NewRuleError(violations)
}
