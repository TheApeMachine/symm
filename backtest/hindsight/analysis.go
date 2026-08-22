package hindsight

import (
	"fmt"
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
	At                  time.Time          `json:"at"`
	Action              string             `json:"action"`
	Reason              string             `json:"reason"`
	Cause               string             `json:"cause"`
	GraphScore          float64            `json:"graphScore"`
	ThesisScore         float64            `json:"thesisScore"`
	ThesisConfidence    float64            `json:"thesisConfidence"`
	ThesisSupport       float64            `json:"thesisSupport"`
	ThesisContradiction float64            `json:"thesisContradiction"`
	ThesisConditions    float64            `json:"thesisConditions"`
	Direction           float64            `json:"direction"`
	Confidence          float64            `json:"confidence"`
	AdmissionThreshold  float64            `json:"admissionGraphThreshold"`
	Opportunity         bool               `json:"opportunity"`
	Type                string             `json:"opportunityType,omitempty"`
	PredictiveReady     bool               `json:"predictiveReady"`
	PredictiveStatus    string             `json:"predictiveStatus"`
	Alternatives        map[string]float64 `json:"alternatives"`
}

/*
MissedLeg pairs one perfect hold with the reason the system did not take it:
the nearest decision before the leg opened and the measurement scores it was
reading, plus the decision journal spanning just before entry to just after
exit, and an inferred diagnosis derived from that recorded state.
*/
type MissedLeg struct {
	Leg      Leg           `json:"leg"`
	Signal   SignalContext `json:"signal"`
	Journal  []Decision    `json:"journal"`
	Why      string        `json:"why"`
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
bestDecisionFor scans the decisions inside a leg's upward move and returns the
one that best explains why the system stayed flat. It prefers decisions that
flagged an opportunity but declined entry, and falls back to the highest score.
*/
func bestDecisionFor(decisions []Decision, leg Leg) *Decision {
	var best *Decision

	for i := range decisions {
		decision := &decisions[i]

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
			continue
		}

		if !decision.Opportunity && best.Opportunity {
			continue
		}

		if decision.ThesisScore > best.ThesisScore {
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
truthFor returns the decision whose window the system had available when it
declined a leg, plus the audit journal inside the leg's entry/exit window. A
leg is captured when an enter decision covers its window; otherwise the nearest
prior decision is the one whose measurement scores explain why it stayed flat.
*/
func truthFor(decisions []Decision, leg Leg) MissedLeg {
	captured := hasEntryInside(decisions, leg)
	context := SignalContext{At: leg.BuyAt}
	journal := decisionsAround(decisions, leg)

	if !captured {
		if decision := bestDecisionFor(decisions, leg); decision != nil {
			context = SignalContext{
				At:                  decision.At,
				Action:              decision.Action,
				Reason:              decision.Reason,
				Cause:               decision.Cause,
				GraphScore:          decision.GraphScore,
				ThesisScore:         decision.ThesisScore,
				ThesisConfidence:    decision.ThesisConfidence,
				ThesisSupport:       decision.ThesisSupport,
				ThesisContradiction: decision.ThesisContradiction,
				ThesisConditions:    decision.ThesisConditions,
				Direction:           decision.Direction,
				Confidence:          decision.Confidence,
				AdmissionThreshold:  decision.AdmissionThreshold,
				Opportunity:         decision.Opportunity,
				Type:                decision.OpportunityType,
				PredictiveReady:     decision.PredictiveReady,
				PredictiveStatus:    decision.PredictiveStatus,
				Alternatives:        decision.Alternatives,
			}
		}
	}

	return MissedLeg{
		Leg:      leg,
		Signal:   context,
		Journal:  journal,
		Why:      diagnose(context, leg),
		Captured: captured,
		Missed:   !captured,
	}
}

/*
decisionsAround returns the audit decisions from the nearest moment before a
leg's entry through the first moment after its exit, in order. Replaying that
slice is what lets the dashboard reconstruct the system's actual state — not
just the single arbitration that declined it — across the whole missed hold.
*/
func decisionsAround(decisions []Decision, leg Leg) []Decision {
	if len(decisions) == 0 {
		return nil
	}

	// The first decision strictly after the exit is the trailing boundary;
	// when the hold ends the tape, the last recorded decision is the bound.
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

	// Back up to the decision immediately before the entry so the slice
	// starts with what the system knew just before the hold opened.
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
diagnose infers, from the recorded decision state pinned to a missed leg, the
single most likely reason the system stayed flat. It reads only the fields the
tap recorded, so the explanation is traceable back to the arbitration scores
rather than invented after the fact.
*/
func diagnose(context SignalContext, leg Leg) string {
	if context.Action == "enter" {
		return ""
	}

	upward := leg.SellPrice > leg.BuyPrice
	clause := fmt.Sprintf("missed %s→%s: ", fmtPrice(leg.BuyPrice), fmtPrice(leg.SellPrice))

	switch {
	case context.Opportunity:
		return clause + fmt.Sprintf(
			"flagged as %s but no entry — %s",
			context.Type,
			firstNonEmpty(context.Reason, context.Cause, context.PredictiveStatus, "decided nothing"),
		)
	case upward && context.Direction < 0:
		return clause + fmt.Sprintf(
			"thesis pointed the wrong way (direction %.4f, confidence %.4f)",
			context.Direction,
			context.ThesisConfidence,
		)
	case !upward && context.Direction > 0:
		return clause + fmt.Sprintf(
			"thesis pointed the wrong way (direction %.4f, confidence %.4f)",
			context.Direction,
			context.ThesisConfidence,
		)
	case context.ThesisContradiction > context.ThesisSupport:
		return clause + fmt.Sprintf(
			"thesis contradiction %.4f exceeded support %.4f",
			context.ThesisContradiction,
			context.ThesisSupport,
		)
	case context.GraphScore < context.AdmissionThreshold:
		return clause + fmt.Sprintf(
			"graph score %.4f below admission %.4f",
			context.GraphScore,
			context.AdmissionThreshold,
		)
	}

	if dominant := dominantAlternative(context.Alternatives, upward); dominant != "" {
		return clause + fmt.Sprintf("blocked by %s", dominant)
	}

	return clause + firstNonEmpty(context.Reason, context.Cause, "no admission")
}

/*
dominantAlternative names the largest-magnitude measurement that fought the
leg's direction — the single number that most plausibly kept the system flat.
*/
func dominantAlternative(alternatives map[string]float64, upward bool) string {
	dominant := ""

	for name, value := range alternatives {
		if upward && value >= 0 {
			continue
		}

		if !upward && value <= 0 {
			continue
		}

		if dominant == "" || abs(value) > abs(alternatives[dominant]) {
			dominant = name
		}
	}

	if dominant == "" {
		return ""
	}

	return fmt.Sprintf("%s (%.4f)", dominant, alternatives[dominant])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

func fmtPrice(price float64) string {
	return fmt.Sprintf("%.6f", price)
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}

	return value
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

			report := truthFor(symbolDecisions, leg)

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
