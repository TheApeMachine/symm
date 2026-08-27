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
	DiagnosisThesisCollapse   = "thesis_invalidation"
	DiagnosisLiquiditySlip    = "liquidity_slippage"
	DiagnosisRegimeCollapse   = "regime_collapse"
)

/*
PositionLoss captures one entered position that closed at a loss (or non-profitable),
along with its entry signal context, holding journal, exit trigger reason, and
inferred post-mortem diagnosis.
*/
type PositionLoss struct {
	Symbol        string        `json:"symbol"`
	DecisionID    string        `json:"decisionId"`
	EntryAt       time.Time     `json:"entryAt"`
	ExitAt        time.Time     `json:"exitAt"`
	EntryPrice    float64       `json:"entryPrice"`
	ExitPrice     float64       `json:"exitPrice"`
	PnL           float64       `json:"pnl"`
	ReturnPct     float64       `json:"returnPct"`
	GrossPct      float64       `json:"grossPct"`
	FrictionPct   float64       `json:"frictionPct"`
	TriggerReason string        `json:"triggerReason"`
	Diagnosis     Diagnosis     `json:"diagnosis"`
	Signal        SignalContext `json:"signal"`
	Journal       []Decision    `json:"journal"`
}

/*
ExtractLosses discovers non-profitable entered positions from a symbol's decision journal
and tape series.
*/
func ExtractLosses(decisions []Decision, series *Series) []PositionLoss {
	losses := make([]PositionLoss, 0)
	var activeEntry *Decision
	var activeJournal []Decision

	for index := range decisions {
		decision := decisions[index]

		if decision.Action == "enter" {
			if activeEntry != nil {
				// Close previous unclosed entry at current decision time
				if loss, ok := evaluateLossPosition(*activeEntry, decision, activeJournal, series); ok {
					losses = append(losses, loss)
				}
			}

			activeEntry = &decision
			activeJournal = []Decision{decision}
			continue
		}

		if activeEntry != nil {
			activeJournal = append(activeJournal, decision)

			if decision.Action == "exit" || decision.Action == "close" || decision.Action == "reduce" {
				if loss, ok := evaluateLossPosition(*activeEntry, decision, activeJournal, series); ok {
					losses = append(losses, loss)
				}

				activeEntry = nil
				activeJournal = nil
			}
		}
	}

	if activeEntry != nil && len(activeJournal) > 0 {
		lastDecision := activeJournal[len(activeJournal)-1]

		if loss, ok := evaluateLossPosition(*activeEntry, lastDecision, activeJournal, series); ok {
			losses = append(losses, loss)
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

	frictionPct := entryPoint.Friction + exitPoint.Friction

	if frictionPct == 0 {
		spread := 0.0

		if entryPoint.Ask > entryPoint.Bid && entryPoint.Price > 0 {
			spread += (entryPoint.Ask - entryPoint.Bid) / entryPoint.Price
		}

		if exitPoint.Ask > exitPoint.Bid && exitPoint.Price > 0 {
			spread += (exitPoint.Ask - exitPoint.Bid) / exitPoint.Price
		}

		frictionPct = spread
	}

	returnPct := grossPct - frictionPct

	// If position is profitable after friction, it is not a loss.
	if returnPct > 0 {
		return PositionLoss{}, false
	}

	triggerReason := firstNonEmpty(exit.Reason, exit.Cause, "exit order executed")
	signal := SignalContext{
		ID:                      entry.ID,
		At:                      entry.At,
		Action:                  entry.Action,
		Reason:                  entry.Reason,
		Cause:                   entry.Cause,
		GraphScore:              entry.GraphScore,
		ThesisScore:             entry.ThesisScore,
		ThesisConfidence:        entry.ThesisConfidence,
		ThesisSupport:           entry.ThesisSupport,
		ThesisContradiction:     entry.ThesisContradiction,
		ThesisConditions:        entry.ThesisConditions,
		Direction:               entry.Direction,
		Confidence:              entry.Confidence,
		AdmissionThreshold:      entry.AdmissionThreshold,
		Opportunity:             entry.Opportunity,
		Type:                    entry.OpportunityType,
		PredictiveReady:         entry.PredictiveReady,
		PredictiveStatus:        entry.PredictiveStatus,
		AllocationClass:         entry.AllocationClass,
		AllocationHaircut:       entry.AllocationHaircut,
		AllocationHaircutReason: entry.AllocationHaircutReason,
		ReserveEligible:         entry.ReserveEligible,
		ReserveReason:           entry.ReserveReason,
		OpenPositions:           entry.OpenPositions,
		SlotCapacity:            entry.SlotCapacity,
		Alternatives:            entry.Alternatives,
	}

	diagnosis := diagnoseLossPosition(entry, exit, journal, series, grossPct, frictionPct, returnPct)

	return PositionLoss{
		Symbol:        entry.Symbol,
		DecisionID:    entry.ID,
		EntryAt:       entry.At,
		ExitAt:        exit.At,
		EntryPrice:    entryPrice,
		ExitPrice:     exitPrice,
		PnL:           returnPct * entryPrice,
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
	journal []Decision,
	series *Series,
	grossPct float64,
	frictionPct float64,
	returnPct float64,
) Diagnosis {
	lossMagnitude := -returnPct
	reasonText := strings.ToLower(firstNonEmpty(exit.Reason, exit.Cause, entry.Reason))

	// Check for friction drag: gross move was flat/positive, but spread/fees caused net loss.
	if grossPct >= 0 && returnPct <= 0 {
		blocker := Blocker{
			Key:      "loss:friction_drag",
			Category: DiagnosisFrictionDrag,
			Label:    "friction drag",
			Observed: frictionPct,
			Target:   grossPct,
			HasTarget: true,
			Gap:      frictionPct - grossPct,
			Severity: lossMagnitude,
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
				Target:      "trading.admission.minimum_thesis_score",
				Title:       "Increase minimum score hurdle against wide spreads",
				Action:      "Require expected move to exceed round-trip friction before admitting entry",
				Rationale:   "Position suffered from spread and fee drag despite non-negative price movement",
				ImpactPct:   lossMagnitude,
				Occurrences: 1,
				Symbols:     []string{entry.Symbol},
			},
		}
	}

	// Check for whipsaw / tight stoploss.
	if strings.Contains(reasonText, "stoploss") || strings.Contains(reasonText, "floor") {
		blocker := Blocker{
			Key:      "loss:whipsaw_stopout",
			Category: DiagnosisWhipsawStopout,
			Label:    "stoploss trigger",
			Observed: exitPriceFromSeries(series, exit.At),
			Severity: lossMagnitude,
			Explanation: fmt.Sprintf(
				"position was stopped out: %s",
				firstNonEmpty(exit.Reason, exit.Cause, "stoploss floor breached"),
			),
		}

		return Diagnosis{
			Category:        DiagnosisWhipsawStopout,
			Summary:         fmt.Sprintf("loss %.2f%%: stopped out by volatility wick (%s)", lossMagnitude*100, blocker.Explanation),
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

	// Check for thesis collapse / contradiction spike during hold.
	for _, step := range journal {
		if step.ThesisContradiction > step.ThesisSupport && step.ThesisContradiction > 0.5 {
			blocker := Blocker{
				Key:       "loss:thesis_invalidation",
				Category:  DiagnosisThesisCollapse,
				Label:     "thesis contradiction spike",
				Observed:  step.ThesisContradiction,
				Target:    step.ThesisSupport,
				HasTarget: true,
				Gap:       step.ThesisContradiction - step.ThesisSupport,
				Severity:  lossMagnitude,
				Explanation: fmt.Sprintf(
					"thesis contradiction spiked to %.4f against support %.4f during hold",
					step.ThesisContradiction,
					step.ThesisSupport,
				),
			}

			return Diagnosis{
				Category:        DiagnosisThesisCollapse,
				Summary:         fmt.Sprintf("loss %.2f%%: thesis contradiction spiked during hold", lossMagnitude*100),
				EvidenceQuality: 0.9,
				EvidenceStatus:  "complete",
				Blockers:        []Blocker{blocker},
				Recommendation: Recommendation{
					Key:         "fast_thesis_exit",
					Kind:        RecommendationTuneParameter,
					Target:      "trading.risk.max_contradiction",
					Title:       "Enforce early exit on contradiction spike",
					Action:      "Trigger faster risk-off exit when contradiction exceeds support during hold",
					Rationale:   "Holding decayed as market structure turned contradictory",
					ImpactPct:   lossMagnitude,
					Occurrences: 1,
					Symbols:     []string{entry.Symbol},
				},
			}
		}
	}

	// Default to adverse selection / exhaustion entry.
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

func exitPriceFromSeries(series *Series, at time.Time) float64 {
	if series == nil {
		return 0
	}

	return series.PriceAt(at)
}
