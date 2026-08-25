package domain

import (
	"fmt"
	"sort"
)

type RevisionDifference struct {
	Field  string `json:"field"`
	Label  string `json:"label"`
	Change string `json:"change"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
}

type RevisionComparison struct {
	FromRevision int                  `json:"fromRevision"`
	ToRevision   int                  `json:"toRevision"`
	Differences  []RevisionDifference `json:"differences"`
}

func (c RelocationCase) RevisionByNumber(number int) (MethodRevision, bool) {
	for _, revision := range c.Revisions {
		if revision.RevisionNumber == number && revision.CaseID == c.CaseID {
			return revision, true
		}
	}
	return MethodRevision{}, false
}

func (c RelocationCase) CompareRevisions(fromNumber, toNumber int) (RevisionComparison, error) {
	if fromNumber <= 0 || toNumber <= 0 || fromNumber >= toNumber {
		return RevisionComparison{}, fmt.Errorf("方案修订范围必须按较早版本到较新版本排列")
	}
	from, ok := c.RevisionByNumber(fromNumber)
	if !ok {
		return RevisionComparison{}, fmt.Errorf("方案修订 %d 不存在", fromNumber)
	}
	to, ok := c.RevisionByNumber(toNumber)
	if !ok {
		return RevisionComparison{}, fmt.Errorf("方案修订 %d 不存在", toNumber)
	}
	differences := []RevisionDifference{
		difference("rootBallDiameterCm", "土球直径", from.RootBallDiameterCM, to.RootBallDiameterCM),
		difference("excavationMeasures", "起掘措施", from.ExcavationMeasures, to.ExcavationMeasures),
		difference("packingMeasures", "包装措施", from.PackingMeasures, to.PackingMeasures),
		difference("transportMeasures", "运输措施", from.TransportMeasures, to.TransportMeasures),
		difference("plantingMeasures", "定植措施", from.PlantingMeasures, to.PlantingMeasures),
		difference("aftercareMeasures", "养护措施", from.AftercareMeasures, to.AftercareMeasures),
	}
	keys := make(map[string]struct{}, len(from.RiskControls)+len(to.RiskControls))
	for key := range from.RiskControls {
		keys[key] = struct{}{}
	}
	for key := range to.RiskControls {
		keys[key] = struct{}{}
	}
	riskKeys := make([]string, 0, len(keys))
	for key := range keys {
		riskKeys = append(riskKeys, key)
	}
	sort.Strings(riskKeys)
	for _, key := range riskKeys {
		differences = append(differences, difference("riskControls."+key, "风险控制 "+key, from.RiskControls[key], to.RiskControls[key]))
	}
	return RevisionComparison{FromRevision: fromNumber, ToRevision: toNumber, Differences: differences}, nil
}

func difference(field, label string, before, after any) RevisionDifference {
	change := "unchanged"
	beforeText, beforeOK := before.(string)
	afterText, afterOK := after.(string)
	if beforeOK && afterOK {
		switch {
		case beforeText == "" && afterText != "":
			change = "added"
		case beforeText != "" && afterText == "":
			change = "deleted"
		case beforeText != afterText:
			change = "modified"
		}
	} else if before != after {
		change = "modified"
	}
	return RevisionDifference{Field: field, Label: label, Change: change, Before: before, After: after}
}
