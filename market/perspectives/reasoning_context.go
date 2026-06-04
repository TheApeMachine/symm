package perspectives

import "time"

/*
PositionState is the open trade's view, the way the tree reasons about its own
move. Peak is the running high since entry, EntryPrice the fill, Last the current
price, EntryAt/Now the clock — enough to decide whether the move has continued,
ended, or barely started.
*/
type PositionState struct {
	Holding    bool
	EntryPrice float64
	Peak       float64
	Last       float64
	EntryAt    time.Time
	Now        time.Time
}

/*
WindowReason answers a Thought's predicates from a per-symbol measurement window
(newest first), the current regime, and the open position. It is the live/replay
bridge: build one of these each tick and hand it to Evaluate.

Indexing of `ago`:
  - signals  -> the Nth most recent reading OF THAT CATEGORY (occurrences back)
  - price    -> the Nth most recent DISTINCT price (price-changes back, so a
                quiet market doesn't make "10 ago" mean "10 duplicate quotes ago")
  - spread   -> the Nth most recent reading
These are first-pass conventions; the optimizer tunes the thresholds around them.
*/
type WindowReason struct {
	regime       Regime
	position     PositionState
	continuedPct float64
	endedPct     float64
	signal       map[CategoryType][]Measurement
	price        []float64
	spread       []float64
}

/*
NewWindowReason builds the context from a symbol's ring snapshot, the classified
regime, and the open position. Lifecycle thresholds come from config
(reasoning.continued_pct / reasoning.ended_pct) and are optimizer-tunable.

CONTRACT: snapshots MUST be in chronological order, oldest→newest. Every temporal
predicate (rose_by / fell_by / crossed_up / crossed_down) and every `ago` lookup
reads this ordering to mean "N readings into the past"; a non-chronological slice
silently inverts those comparisons. market.RingSnapshot (the replay tape and the
live ring) satisfies this. Note that the replay ledger's measurements.Snapshot is
sorted by source, NOT by time — it must never be fed here; the Thought path uses
the tape (RingSnapshot) exactly so this holds.
*/
func NewWindowReason(
	snapshots []Measurement, regime Regime, position PositionState,
) *WindowReason {
	reason := &WindowReason{
		regime:       regime,
		position:     position,
		continuedPct: viperFloatDefault("reasoning.continued_pct", 1.0) / 100,
		endedPct:     viperFloatDefault("reasoning.ended_pct", 1.0) / 100,
		signal:       make(map[CategoryType][]Measurement),
	}

	var lastPrice float64

	for index := len(snapshots) - 1; index >= 0; index-- {
		measurement := snapshots[index]

		if measurement.Category != CategoryTypeNone {
			reason.signal[measurement.Category] = append(reason.signal[measurement.Category], measurement)
		}

		if measurement.Last > 0 && measurement.Last != lastPrice {
			reason.price = append(reason.price, measurement.Last)
			lastPrice = measurement.Last
		}

		reason.spread = append(reason.spread, measurement.SpreadBPS)
	}

	return reason
}

func (reason *WindowReason) Regime() Regime {
	return reason.regime
}

func (reason *WindowReason) Lifecycle(state ObservationType) bool {
	position := reason.position

	switch state {
	case ObservationNotHolding:
		return !position.Holding
	case ObservationHolding:
		return position.Holding
	case ObservationHasContinued:
		return position.Holding && reason.moveContinued()
	case ObservationHasEnded:
		return position.Holding &&
			position.Peak > 0 &&
			position.Last <= position.Peak*(1-reason.endedPct)
	case ObservationHasStarted:
		// Fresh: holding, but the move has not yet confirmed it is running.
		return position.Holding && !reason.moveContinued()
	default:
		return false
	}
}

func (reason *WindowReason) moveContinued() bool {
	position := reason.position

	return position.EntryPrice > 0 &&
		position.Peak >= position.EntryPrice*(1+reason.continuedPct)
}

func (reason *WindowReason) Signal(category CategoryType, unit UnitType, ago int) (float64, bool) {
	series, ok := reason.signal[category]

	if !ok || ago < 0 || ago >= len(series) {
		return 0, false
	}

	measurement := series[ago]

	switch unit {
	case UnitConfidence:
		return measurement.Confidence, true
	default:
		return measurement.SNR, true
	}
}

func (reason *WindowReason) Scalar(subject Subject, unit UnitType, ago int) (float64, bool) {
	switch subject {
	case SubjectPrice:
		if ago < 0 || ago >= len(reason.price) {
			return 0, false
		}

		return reason.price[ago], true
	case SubjectSpread:
		if ago < 0 || ago >= len(reason.spread) {
			return 0, false
		}

		return reason.spread[ago], true
	case SubjectElapsed:
		if !reason.position.Holding || reason.position.EntryAt.IsZero() {
			return 0, false
		}

		elapsed := reason.position.Now.Sub(reason.position.EntryAt)

		switch unit {
		case UnitTimeMinutes:
			return elapsed.Minutes(), true
		default:
			return elapsed.Seconds(), true
		}
	default:
		// SubjectVolume has no measurement source yet; honestly report absence.
		return 0, false
	}
}
