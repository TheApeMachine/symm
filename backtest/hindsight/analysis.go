package hindsight

import (
	"sort"
	"time"
)

/*
SignalContext is the measurement snapshot the logic was actually reading at the
moment it declined a leg's opportunity. It captures the scored alternatives the
decision stream recorded so a missed leg can be traced back to the exact
measurement that (or the exact admission gate that) kept the system flat.
*/
type SignalContext struct {
	At           time.Time          `json:"at"`
	GraphScore   float64            `json:"graphScore"`
	ThesisScore  float64            `json:"thesisScore"`
	Opportunity  bool               `json:"opportunity"`
	Type         string             `json:"opportunityType,omitempty"`
	Alternatives map[string]float64 `json:"alternatives"`
}

/*
MissedLeg pairs one perfect round trip with the reason the system did not take
it: the nearest decision before the leg opened and the measurement scores it
was reading. Profit is what was left on the table by staying flat.
*/
type MissedLeg struct {
	Leg      Leg           `json:"leg"`
	Signal   SignalContext `json:"signal"`
	Captured bool          `json:"captured"`
	Missed   bool          `json:"missed"`
}

/*
PerSymbol ties a symbol's perfect legs to what the system actually did, and how
much profit the tape offered versus how much the system collected.
*/
type PerSymbol struct {
	Symbol        string      `json:"symbol"`
	UpboundPct    float64     `json:"upboundPct"`
	RealizedPct   float64     `json:"realizedPct"`
	MissedPct     float64     `json:"missedPct"`
	Legs          int         `json:"legs"`
	MissedLegs    int         `json:"missedLegs"`
	Opportunities []MissedLeg `json:"opportunities"`
}

/*
CollectDecisionIndex groups decisions by symbol and sorts each symbol's list by
decision time so nearest-decision lookups run in log time. Only decisions go
into the timeline; capture-order duplicates are dropped by time.
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
		index[symbol] = sorted
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
hasEntryInside reports whether the decisions contain an actionable entry that
opens a position inside the leg's window (from its buy to its sell), meaning
the system was at least on the tape for the rising move.
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
truthOfContext returns the decision whose window (either side of the leg's buy)
the system had available when it declined. A leg is captured when an enter
decision covers its window; otherwise the nearest prior decision is the one
whose measurement scores explain why it stayed flat.
*/
func truthFor(symbol string, decisions []Decision, leg Leg) MissedLeg {
	captured := hasEntryInside(decisions, leg)
	context := SignalContext{At: leg.BuyAt}

	if !captured {
		if decision := latestDecisionBefore(decisions, leg.BuyAt); decision != nil {
			context = SignalContext{
				At:           decision.At,
				GraphScore:   decision.GraphScore,
				ThesisScore:  decision.ThesisScore,
				Opportunity:  decision.Opportunity,
				Type:         decision.OpportunityType,
				Alternatives: decision.Alternatives,
			}
		}
	}

	return MissedLeg{
		Leg:      leg,
		Signal:   context,
		Captured: captured,
		Missed:   !captured,
	}
}

/*
Analyze reduces all series and decisions for a capture into per-symbol hindsight
reports: the tape's maximum possible value, what the system realized, and each
missed leg with the signal that declined it. Realized profit is the share of
the upbound that came from legs the system actually entered, so the upbound -
realized gap is precisely the value the system left flat.
*/
func Analyze(reducer *Reducer, decisions []Decision) ([]PerSymbol, error) {
	decisionIndex := collectDecisionsIndex(decisions)
	reports := make([]PerSymbol, 0, len(reducer.series))

	for _, series := range reducer.Symbols() {
		legs := RoundTrips(series)
		symbolDecisions := decisionIndex[series.Symbol]
		upbound := 0.0
		realized := 0.0
		missed := 0.0
		missedCount := 0
		opportunities := make([]MissedLeg, 0, len(legs.Legs))

		for _, leg := range legs.Legs {
			upbound += leg.ProfitPct

			report := truthFor(series.Symbol, symbolDecisions, leg)

			if report.Captured {
				realized += leg.ProfitPct
			}

			if report.Missed {
				missed += leg.ProfitPct
				missedCount++
			}

			if report.Signal.Opportunity || report.Missed {
				opportunities = append(opportunities, report)
			}
		}

		reports = append(reports, PerSymbol{
			Symbol:        series.Symbol,
			UpboundPct:    upbound,
			RealizedPct:   realized,
			MissedPct:     missed,
			Legs:          len(legs.Legs),
			MissedLegs:    missedCount,
			Opportunities: opportunities,
		})
	}

	// Stable output order is friendlier to the dashboard than capture order.
	sort.SliceStable(reports, func(i, j int) bool {
		return reports[i].MissedPct > reports[j].MissedPct
	})

	return reports, nil
}
