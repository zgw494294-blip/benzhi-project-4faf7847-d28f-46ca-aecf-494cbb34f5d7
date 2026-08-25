package domain

import (
	"math"
	"time"
)

const (
	assessmentValidityDays = 180
	assessmentExpiringDays = 30
)

func DeriveAssessmentValidity(kind string, assessedAt, now time.Time) AssessmentValidity {
	result := AssessmentValidity{Kind: kind, AssessedAt: assessedAt, Conclusion: "过期"}
	if assessedAt.IsZero() {
		result.Conclusion = "缺失"
		return result
	}
	result.ValidUntil = assessedAt.UTC().Add(assessmentValidityDays * 24 * time.Hour)
	remaining := result.ValidUntil.Sub(now.UTC()).Hours() / 24
	result.RemainingDays = int(math.Ceil(remaining))
	if now.UTC().After(result.ValidUntil) {
		result.Conclusion = "过期"
		return result
	}
	if result.RemainingDays <= assessmentExpiringDays {
		result.Conclusion = "临期"
	} else {
		result.Conclusion = "有效"
	}
	return result
}

func (c RelocationCase) AssessmentValidities(now time.Time) []AssessmentValidity {
	treeAt, destinationAt := time.Time{}, time.Time{}
	if c.TreeProfile != nil {
		treeAt = c.TreeProfile.AssessedAt
	}
	if c.Destination != nil {
		destinationAt = c.Destination.AssessedAt
	}
	return []AssessmentValidity{
		DeriveAssessmentValidity("tree", treeAt, now),
		DeriveAssessmentValidity("destination", destinationAt, now),
	}
}

func (c RelocationCase) ValidateAssessmentValidity(now time.Time) error {
	var violations []Violation
	for _, validity := range c.AssessmentValidities(now) {
		if validity.Conclusion != "缺失" && validity.Conclusion != "过期" {
			continue
		}
		if validity.Kind == "tree" {
			violations = append(violations, Violation{"TREE_ASSESSMENT_EXPIRED", "tree.assessedAt", "树体评估已过期，请重新评估"})
		} else {
			violations = append(violations, Violation{"DESTINATION_ASSESSMENT_EXPIRED", "destination.assessedAt", "迁入地评估已过期，请重新评估"})
		}
	}
	return NewRuleError(violations)
}
