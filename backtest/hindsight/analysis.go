package hindsight

import (
	"sort"
	"time"
)

/*
SignalContext is the current-architecture decision snapshot the logic recorded
at the moment it declined a leg: its Opportunity and Valuation state, the MCTS
selection it made, and the execution/risk plan it solved. It deliberately no
longer carries the retired Thesis/Graph scoring fields.
*/
type SignalContext struct {
	ID     string    `json:"id"`
	At     time.Time `json:"at"`
	Action string    `json:"action"`
	Reason string    `json:"reason"`
	Cause  string    `json:"cause"`

	// Opportunity state.
	Opportunity      bool   `json:"opportunity"`
	OpportunityType  string `json:"opportunityType,omitempty"`
	OpportunityPhase string `json:"opportunityPhase,omitempty"`

	// Valuation state.
	ValuationAttempted       bool   `json:"valuationAttempted"`
	ValuationAvailable       bool   `json:"valuationAvailable"`
	ValuationStatus          string `json:"valuationStatus,omitempty"`
	CausalIdentification     string `json:"causalIdentification,omitempty"`
	CausalBlockingCoordinate string `json:"causalBlockingCoordinate,omitempty"`

	// Selection state.
	Utility          float64            `json:"utility"`
	UtilityAvailable bool               `json:"utilityAvailable"`
	Alternatives     map[string]float64 `json:"alternatives"`
	MCTS             DecisionTrace      `json:"mcts,omitempty"`

	// Execution state.
	ProposedQuantity Number    `json:"proposedQuantity"`
	ProposedNotional Number    `json:"proposedNotional"`
	AvailableCapital Number    `json:"availableCapital"`
	EntryCost        EntryCost `json:"entryCost,omitempty"`
	Risk             RiskPlan  `json:"risk"`
	ExpectedReturn   Number    `json:"expectedReturn"`
	ExpectedFees     Number    `json:"expectedFees"`
	ExpectedSpread   Number    `json:"expectedSpread"`
	ExpectedImpact   Number    `json:"expectedImpact"`
	AdverseSelection Number    `json:"adverseSelection"`
	Uncertainty      float64   `json:"uncertainty"`

	// Allocation state.
	AllocationClass   string  `json:"allocationClass"`
	AllocationHaircut float64 `json:"allocation_haircut"`
	OpenPositions     int     `json:"openPositions"`
	SlotCapacity      int     `json:"slotCapacity"`
}

/*
RegretLayer names which stage of the opportunity→outcome chain owns a regret.
It is the correction to the old single-score view: detection, valuation,
selection, execution, and management each fail on their own evidence and must
not be blamed for one another.
*/
type RegretLayer struct {
	Detection  bool `json:"detection"`
	Valuation  bool `json:"valuation"`
	Selection  bool `json:"selection"`
	Execution  bool `json:"execution"`
	Management bool `json:"management"`
}

/*
MissedLeg pairs one theoretical hold with the current decision state that
declined it, the executable counterfactual (when the recorded quantity could be
defended through historical depth), and the layer-by-layer regret.
*/
type MissedLeg struct {
	Leg        Leg            `json:"leg"`
	Signal     SignalContext  `json:"signal"`
	Journal    []Decision     `json:"journal"`
	Executable *ExecutableLeg `json:"executable,omitempty"`
	Regret     RegretLayer    `json:"regret"`
	Why        string         `json:"why"`
	Diagnosis  Diagnosis      `json:"diagnosis"`
	Captured   bool           `json:"captured"`
	Missed     bool           `json:"missed"`
}

/*
PerSymbol separates the price-theoretical ceiling (what the observed price path
offered) from the executable ceiling (what the recorded quantity could actually
capture through L3 depth), alongside what the system realized and lost.
*/
type PerSymbol struct {
	Symbol string `json:"symbol"`

	// PriceTheoreticalCeiling is the sum of theoretical leg returns with no
	// size claim — the tape's path, not an achievable PnL.
	PriceTheoreticalCeiling float64 `json:"priceTheoreticalCeiling"`
	// ExecutableCeiling is the sum of executable leg returns over legs whose
	// recorded quantity could be fully captured; legs without a defensible
	// quantity or sufficient depth contribute nothing here (undefined ≠ zero
	// to the reader, and is tracked via ExecutableLegsDefined).
	ExecutableCeiling     float64 `json:"executableCeiling"`
	ExecutableLegsDefined int     `json:"executableLegsDefined"`
	ExecutableLegsTotal   int     `json:"executableLegsTotal"`

	RealizedPct   float64 `json:"realizedPct"`
	MissedPct     float64 `json:"missedPct"`
	LossPct       float64 `json:"lossPct"`
	Legs          int     `json:"legs"`
	MissedLegs    int     `json:"missedLegs"`
	LossPositions int     `json:"lossPositions"`

	Opportunities []MissedLeg    `json:"opportunities"`
	Losses        []PositionLoss `json:"losses"`
}

/*
collectDecisionsIndex groups decisions by symbol and sorts each symbol's list by
decision time so nearest-decision lookups run in log time. Capture-order
duplicates at the same venue time are dropped in favour of the later replay.
*/
func collectDecisionsIndex(decisions []Decision) map[string][]Decision {
	index := map[string][]Decision{}

	for _, decision := range decisions {
		index[decision.Symbol] = append(index[decision.Symbol], decision)
	}

	for symbol := range index {
		sorted := index[symbol]
		sort.SliceStable(sorted, func(i, j int) bool {
			return sorted[i].At.Before(sorted[j].At)
		})

		deduplicated := sorted[:0]

		for _, decision := range sorted {
			last := len(deduplicated) - 1

			if last >= 0 && deduplicated[last].At.Equal(decision.At) {
				deduplicated[last] = decision
				continue
			}

			deduplicated = append(deduplicated, decision)
		}

		index[symbol] = deduplicated
	}

	return index
}

/*
latestDecisionBefore returns the decision immediately before `at` for a symbol,
or nil when the symbol has no decision by that time.
*/
func latestDecisionBefore(decisions []Decision, at time.Time) *Decision {
	for index := len(decisions) - 1; index >= 0; index-- {
		decision := decisions[index]

		if !decision.At.After(at) {
			return &decision
		}
	}

	return nil
}

/*
bestDecisionFor scans the decisions inside a leg's move and returns the one that
best explains why the system stayed flat: it prefers decisions that flagged an
opportunity but declined, then the one with selection utility recorded.
*/
func bestDecisionFor(decisions []Decision, leg Leg) *Decision {
	var best *Decision

	for index := range decisions {
		decision := &decisions[index]

		if decision.At.Before(leg.BuyAt) {
			continue
		}

		if decision.At.After(leg.SellAt) {
			break
		}

		if best == nil {
			best = decision
			continue
		}

		if decision.Opportunity && !best.Opportunity {
			best = decision
		}
	}

	if best == nil {
		return latestDecisionBefore(decisions, leg.BuyAt)
	}

	return best
}

/*
hasEntryInside reports whether the decisions contain an actionable entry that
opens a position inside the leg's window.
*/
func hasEntryInside(decisions []Decision, leg Leg) bool {
	for _, decision := range decisions {
		if decision.Action != "enter" {
			continue
		}

		if !decision.At.Before(leg.BuyAt) && !decision.At.After(leg.SellAt) {
			return true
		}
	}

	return false
}

/*
SignalFromDecision projects one recorded Decision onto the SignalContext the
diagnosis and the dashboard consume.
*/
func SignalFromDecision(decision Decision) SignalContext {
	return SignalContext{
		ID:                       decision.ID,
		At:                       decision.At,
		Action:                   decision.Action,
		Reason:                   decision.Reason,
		Cause:                    decision.Cause,
		Opportunity:              decision.Opportunity,
		OpportunityType:          decision.OpportunityType,
		OpportunityPhase:         decision.OpportunityPhase,
		ValuationAttempted:       decision.ValuationAttempted,
		ValuationAvailable:       decision.ValuationAvailable,
		ValuationStatus:          decision.ValuationStatus,
		CausalIdentification:     decision.CausalIdentification,
		CausalBlockingCoordinate: decision.CausalBlockingCoordinate,
		Utility:                  decision.Utility,
		UtilityAvailable:         decision.UtilityAvailable,
		Alternatives:             decision.Alternatives,
		MCTS:                     decision.Trace,
		ProposedQuantity:         decision.ProposedQuantity,
		ProposedNotional:         decision.ProposedNotional,
		AvailableCapital:         decision.AvailableCapital,
		EntryCost:                decision.EntryCost,
		Risk:                     decision.Risk,
		ExpectedReturn:           decision.ExpectedReturn,
		ExpectedFees:             decision.ExpectedFees,
		ExpectedSpread:           decision.ExpectedSpread,
		ExpectedImpact:           decision.ExpectedImpact,
		AdverseSelection:         decision.AdverseSelection,
		Uncertainty:              decision.Uncertainty,
		AllocationClass:          decision.AllocationClass,
		AllocationHaircut:        decision.AllocationHaircut,
		OpenPositions:            decision.OpenPositions,
		SlotCapacity:             decision.SlotCapacity,
	}
}

/*
truthFor builds the missed-leg record for one theoretical leg against the
decision timeline: captured flag, the declining decision's current-architecture
context, the audit journal, and the layer-by-layer regret.
*/
func truthFor(decisions []Decision, leg Leg, observerAvailable bool) MissedLeg {
	captured := hasEntryInside(decisions, leg)
	context := SignalContext{At: leg.BuyAt}
	journal := decisionsAround(decisions, leg)
	recorded := false

	if decision := bestDecisionFor(decisions, leg); decision != nil {
		recorded = true
		context = SignalFromDecision(*decision)
	}

	diagnosis := Diagnosis{}
	regret := RegretLayer{}

	if !captured {
		diagnosis = diagnoseOpportunity(context, leg, recorded, observerAvailable)
		regret = regretLayers(context, leg, recorded, observerAvailable)
	}

	return MissedLeg{
		Leg:       leg,
		Signal:    context,
		Journal:   journal,
		Why:       diagnosis.Summary,
		Diagnosis: diagnosis,
		Regret:    regret,
		Captured:  captured,
		Missed:    !captured,
	}
}

/*
decisionsAround returns the audit decisions from the nearest moment before a
leg's entry through the first moment after its exit, in order.
*/
func decisionsAround(decisions []Decision, leg Leg) []Decision {
	if len(decisions) == 0 {
		return nil
	}

	first := len(decisions)

	for index, decision := range decisions {
		if decision.At.After(leg.SellAt) {
			first = index
			break
		}
	}

	if first >= len(decisions) {
		first = len(decisions) - 1
	}

	start := 0

	for index := first; index >= 0; index-- {
		if decisions[index].At.Before(leg.BuyAt) {
			start = index
			break
		}
	}

	out := make([]Decision, 0, first-start+1)

	for index := start; index <= first; index++ {
		out = append(out, decisions[index])
	}

	return out
}

/*
Analyze reduces one capture's series, decisions, and reconstructed L3 book into
per-symbol hindsight reports. The theoretical ceiling is the tape's path; the
executable ceiling is the share any recorded quantity could actually capture.
`observerStartedAt` is the process observation start so movement before the
system was live is not blamed on the strategy.
*/
func Analyze(
	reducer *Reducer,
	decisions []Decision,
	bookStore *BookStore,
	observerStartedAt time.Time,
) ([]PerSymbol, error) {
	if reducer == nil {
		return nil, nil
	}

	decisionIndex := collectDecisionsIndex(decisions)
	reports := make([]PerSymbol, 0, len(reducer.series))

	for _, series := range reducer.Symbols() {
		legs := RoundTrips(series)
		symbolDecisions := decisionIndex[series.Symbol]
		theoretical := 0.0
		executableCeiling := 0.0
		realized := 0.0
		missed := 0.0
		missedCount := 0
		executableDefined := 0
		executableTotal := 0
		opportunities := make([]MissedLeg, 0, len(legs.Legs))

		for _, leg := range legs.Legs {
			theoretical += leg.ProfitPct

			// A zero observer start means the whole capture is observed: only an
			// explicit later start gates pre-start legs out of "missed strategy".
			observerAvailable := observerStartedAt.IsZero() ||
				!leg.BuyAt.Before(observerStartedAt)

			report := truthFor(symbolDecisions, leg, observerAvailable)

			// Executable counterfactual needs a defensible quantity.
			quantity, quantityDefensible := counterfactualQuantity(symbolDecisions, leg)

			if quantityDefensible {
				executableTotal++
				feeRate := counterfactualFeeRate(symbolDecisions, leg)

				if bookStore != nil {
					if outcome, ok := ExecutableCounterfactual(
						bookStore,
						leg,
						quantity,
						feeRate,
					); ok {
						report.Executable = &outcome
						executableCeiling += outcome.ExecutableReturn
						executableDefined++
					}
				}
			}

			if report.Captured {
				realized += leg.ProfitPct
			}

			if report.Missed && observerAvailable {
				missed += leg.ProfitPct
				missedCount++
			}

			if report.Signal.Opportunity || report.Missed {
				opportunities = append(opportunities, report)
			}
		}

		losses := ExtractLosses(symbolDecisions, series)
		lossPct := 0.0

		for _, loss := range losses {
			lossAmount := -loss.ReturnPct

			if lossAmount > 0 {
				lossPct += lossAmount
			}
		}

		reports = append(reports, PerSymbol{
			Symbol:                  series.Symbol,
			PriceTheoreticalCeiling: theoretical,
			ExecutableCeiling:       executableCeiling,
			ExecutableLegsDefined:   executableDefined,
			ExecutableLegsTotal:     executableTotal,
			RealizedPct:             realized,
			MissedPct:               missed,
			LossPct:                 lossPct,
			Legs:                    len(legs.Legs),
			MissedLegs:              missedCount,
			LossPositions:           len(losses),
			Opportunities:           opportunities,
			Losses:                  losses,
		})
	}

	sort.SliceStable(reports, func(i, j int) bool {
		totalI := reports[i].MissedPct + reports[i].LossPct
		totalJ := reports[j].MissedPct + reports[j].LossPct

		if totalI != totalJ {
			return totalI > totalJ
		}

		if reports[i].MissedPct != reports[j].MissedPct {
			return reports[i].MissedPct > reports[j].MissedPct
		}

		return reports[i].Symbol < reports[j].Symbol
	})

	return reports, nil
}
