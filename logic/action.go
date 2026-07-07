package logic

import (
	"strconv"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"
)

/*
Action is the execution-facing decision output.
Sizing remains owned by broker.Desk through Fraction.
*/
type Action struct {
	ID              string             `json:"id"`
	Tick            int64              `json:"tick"`
	Symbol          string             `json:"symbol"`
	Type            string             `json:"type"`
	Side            string             `json:"side"`
	Verdict         string             `json:"verdict"`
	Reason          string             `json:"reason"`
	Score           float64            `json:"score"`
	EntryLine       float64            `json:"entryLine"`
	EntryScore      float64            `json:"entryScore"`
	EntryConfidence float64            `json:"entryConfidence"`
	Fraction        float64            `json:"fraction"`
	Price           decimal.Decimal    `json:"price"`
	BranchKey       string             `json:"branchKey"`
	ReasonSource    types.SourceType   `json:"reasonSource"`
	ReasonCategory  types.CategoryType `json:"reasonCategory"`
	DecisionAt      string             `json:"decisionAt"`
}

func (decision *Decision) action(
	tick int64,
	symbol string,
	evidence decisionEvidence,
) *Action {
	decisionResult := decision.gate.Decide(evidence)
	score := min(evidence.predictive.confidence, evidence.counterfactual.confidence)
	fraction := 0.0

	if decisionResult.verdict == "allow" {
		fraction = decision.baseFraction * score
	}

	id := strings.Join([]string{
		strconv.FormatInt(tick, 10),
		symbol,
		string(evidence.physical.category),
		string(evidence.predictive.category),
		string(evidence.counterfactual.category),
	}, ":")

	return &Action{
		ID:              id,
		Tick:            tick,
		Symbol:          symbol,
		Type:            decisionResult.actionType,
		Side:            decisionResult.side,
		Verdict:         decisionResult.verdict,
		Reason:          decisionResult.reason,
		Score:           score,
		EntryLine:       evidence.counterfactual.baseline,
		EntryScore:      evidence.counterfactual.strength,
		EntryConfidence: evidence.counterfactual.confidence,
		Fraction:        fraction,
		Price:           evidence.price,
		BranchKey: strings.Join([]string{
			string(evidence.physical.category),
			string(evidence.predictive.category),
			string(evidence.counterfactual.category),
		}, "/"),
		ReasonSource:   decisionResult.source,
		ReasonCategory: decisionResult.category,
		DecisionAt:     evidence.at,
	}
}
