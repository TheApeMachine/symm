package hindsight

import (
	"fmt"
	"strings"
	"time"
)

const (
	DiagnosisAdverseSelection = "adverse_selection"
	DiagnosisFrictionDrag     = "friction_drag"
	DiagnosisWhipsawStopout   = "whipsaw_stopout"
	DiagnosisLiquiditySlip    = "liquidity_slippage"
)

/*
PositionLoss captures one entered position that closed at a loss (or
non-profitable), along with its entry decision context, holding journal, exit
trigger reason, and inferred post-mortem diagnosis.
*/
type PositionLoss struct {
	Symbol        string        `json:"symbol"`
	DecisionID    string        `json:"decisionId"`
	EntryAt       time.Time     `json:"entryAt"`
	ExitAt        time.Time     `json:"exitAt"`
	EntryPrice    float64       `json:"entryPrice"`
	ExitPrice     float64       `json:"exitPrice"`
	LossPerUnit   float64       `json:"lossPerUnit"`
	ReturnPct     float64       `json:"returnPct"`
	GrossPct      float64       `json:"grossPct"`
	FrictionPct   float64       `json:"frictionPct"`
	TriggerReason string        `json:"triggerReason"`
	Diagnosis     Diagnosis     `json:"diagnosis"`
	Signal        SignalContext `json:"signal"`
	Journal       []Decision    `json:"journal"`
}

/*
ExtractLosses discovers non-profitable entered positions from a symbol's
decision journal and tape series.
*/
func ExtractLosses(decisions []Decision, series *Series) []PositionLoss {
	losses := make([]PositionLoss, 0)
	var activeEntry *Decision
	var activeJournal []Decision

	for index := range decisions {
		decision := decisions[index]

		if decision.Action == "enter" {
			if activeEntry != nil {
				if loss, ok := evaluateLossPosition(*activeEntry, decision, activeJournal, series); ok {
					losses = append(losses, loss)
				}
			}

			activeEntry = &decision
			activeJournal = []Decision{decision}
			continue
		}

		if activeEntry == nil {
			continue
		}

		activeJournal = append(activeJournal, decision)

		if decision.Action == "exit" || decision.Action == "close" || decision.Action == "reduce" {
			if loss, ok := evaluateLossPosition(*activeEntry, decision, activeJournal, series); ok {
				losses = append(losses, loss)
			}

			activeEntry = nil
			activeJournal = nil
		}
	}

	return losses
}

func evaluateLossPosition(
	entry Decision,
	exit Decision,
	journal []Decision,
	series *Series,
) (PositionLoss, bool) {
	entryPrice := series.PriceAt(entry.At)
	exitPrice := series.PriceAt(exit.At)

	if entryPrice <= 0 {
		return PositionLoss{}, false
	}

	if exitPrice <= 0 {
		exitPrice = entryPrice
	}

	grossPct := (exitPrice - entryPrice) / entryPrice
	entryPoint := series.PointAt(entry.At)
	exitPoint := series.PointAt(exit.At)

	frictionPct := entryPoint.CrossingCost() + exitPoint.CrossingCost()

	returnPct := grossPct - frictionPct

	if returnPct > 0 {
		return PositionLoss{}, false
	}

	triggerReason := firstNonEmpty(exit.Reason, exit.Cause, "exit order executed")
	signal := SignalFromDecision(entry)

	diagnosis := diagnoseLossPosition(entry, exit, grossPct, frictionPct, returnPct)

	return PositionLoss{
		Symbol:        entry.Symbol,
		DecisionID:    entry.ID,
		EntryAt:       entry.At,
		ExitAt:        exit.At,
		EntryPrice:    entryPrice,
		ExitPrice:     exitPrice,
		LossPerUnit:   returnPct * entryPrice,
		ReturnPct:     returnPct,
		GrossPct:      grossPct,
		FrictionPct:   frictionPct,
		TriggerReason: triggerReason,
		Diagnosis:     diagnosis,
		Signal:        signal,
		Journal:       journal,
	}, true
}

func diagnoseLossPosition(
	entry Decision,
	exit Decision,
	grossPct float64,
	frictionPct float64,
	returnPct float64,
) Diagnosis {
	lossMagnitude := -returnPct
	reasonText := strings.ToLower(firstNonEmpty(exit.Reason, exit.Cause, entry.Reason))

	if grossPct >= 0 {
		blocker := Blocker{
			Key:       "loss:friction_drag",
			Category:  DiagnosisFrictionDrag,
			Label:     "friction drag",
			Observed:  frictionPct,
			Target:    grossPct,
			HasTarget: true,
			Gap:       frictionPct - grossPct,
			Severity:  lossMagnitude,
			Explanation: fmt.Sprintf(
				"gross return was +%.2f%%, but round-trip friction of %.2f%% consumed the gain",
				grossPct*100,
				frictionPct*100,
			),
		}

		return Diagnosis{
			Category:        DiagnosisFrictionDrag,
			Summary:         fmt.Sprintf("loss %.2f%%: friction drag exceeded gross excursion", lossMagnitude*100),
			EvidenceQuality: 1.0,
			EvidenceStatus:  "complete",
			Blockers:        []Blocker{blocker},
			Recommendation: Recommendation{
				Key:         "widen_friction_hurdle",
				Kind:        RecommendationTuneParameter,
				Target:      "trading.admission.friction_hurdle",
				Title:       "Increase the friction hurdle against wide spreads",
				Action:      "Require the expected move to exceed round-trip friction before admitting entry",
				Rationale:   "Position suffered from spread and fee drag despite non-negative price movement",
				ImpactPct:   lossMagnitude,
				Occurrences: 1,
				Symbols:     []string{entry.Symbol},
			},
		}
	}

	if strings.Contains(reasonText, "stoploss") || strings.Contains(reasonText, "floor") {
		blocker := Blocker{
			Key:      "loss:whipsaw_stopout",
			Category: DiagnosisWhipsawStopout,
			Label:    "stoploss trigger",
			Severity: lossMagnitude,
			Explanation: fmt.Sprintf(
				"position was stopped out: %s",
				firstNonEmpty(exit.Reason, exit.Cause, "stoploss floor breached"),
			),
		}

		return Diagnosis{
			Category:        DiagnosisWhipsawStopout,
			Summary:         fmt.Sprintf("loss %.2f%%: stopped out by volatility wick", lossMagnitude*100),
			EvidenceQuality: 0.9,
			EvidenceStatus:  "complete",
			Blockers:        []Blocker{blocker},
			Recommendation: Recommendation{
				Key:         "tune_stoploss_buffer",
				Kind:        RecommendationTuneParameter,
				Target:      "trading.risk.stoploss_buffer",
				Title:       "Tune dynamic stoploss buffer to local volatility",
				Action:      "Anchor stoploss distances to order book depth and recent ATR rather than tight tick bounds",
				Rationale:   "Position was liquidated on a local adverse excursion",
				ImpactPct:   lossMagnitude,
				Occurrences: 1,
				Symbols:     []string{entry.Symbol},
			},
		}
	}

	blocker := Blocker{
		Key:      "loss:adverse_selection",
		Category: DiagnosisAdverseSelection,
		Label:    "adverse selection",
		Observed: grossPct,
		Severity: lossMagnitude,
		Explanation: fmt.Sprintf(
			"price immediately fell %.2f%% after entry: %s",
			-grossPct*100,
			firstNonEmpty(entry.Reason, entry.Cause, "entered near local top / exhaustion"),
		),
	}

	return Diagnosis{
		Category:        DiagnosisAdverseSelection,
		Summary:         fmt.Sprintf("loss %.2f%%: adverse entry into market exhaustion", lossMagnitude*100),
		EvidenceQuality: 0.8,
		EvidenceStatus:  "partial",
		Blockers:        []Blocker{blocker},
		Recommendation: Recommendation{
			Key:         "filter_exhaustion_entries",
			Kind:        RecommendationImproveMeasurement,
			Target:      "trading.evidence.exhaustion_filter",
			Title:       "Filter out late-stage momentum entries",
			Action:      "Require confirmation of order book lift and volume absorption before entry",
			Rationale:   "Entry occurred at peak momentum before immediate adverse price reversal",
			ImpactPct:   lossMagnitude,
			Occurrences: 1,
			Symbols:     []string{entry.Symbol},
		},
	}
}
