package perspectives

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken/trading"
)

/*
PositionState is the open trade's view, the way the tree reasons about its own
move. Peak is the running high since entry, EntryPrice the fill, Last the current
price, EntryAt/Now the clock — enough to decide whether the move has continued,
ended, or barely started.
*/
type PositionState struct {
	Holding    bool
	Side       trading.Side // Buy = long, Sell = short
	EntryPrice float64
	Peak       float64
	Trough     float64
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
	volume       []float64
	spread       []float64
}

type reasoningConfig struct {
	continuedPct float64
	endedPct     float64
}

var (
	cachedReasoningConfig reasoningConfig
	reasoningConfigOnce   sync.Once
)

func loadReasoningConfig() reasoningConfig {
	reasoningConfigOnce.Do(func() {
		cachedReasoningConfig = reasoningConfig{
			continuedPct: viperFloatDefault("reasoning.continued_pct", 1.0) / 100,
			endedPct:     viperFloatDefault("reasoning.ended_pct", 1.0) / 100,
		}
	})

	return cachedReasoningConfig
}

/*
Reset rebuilds reason in-place from the latest chronological snapshot. The zero
value is valid; replay and live story reuse one instance per event loop to keep
predicate evaluation off the allocator hot path.
*/
func (reason *WindowReason) Reset(
	snapshots []Measurement, regime Regime, position PositionState,
) *WindowReason {
	config := loadReasoningConfig()

	reason.regime = regime
	reason.position = position
	reason.continuedPct = config.continuedPct
	reason.endedPct = config.endedPct

	if reason.signal == nil {
		reason.signal = make(map[CategoryType][]Measurement)
	}

	for category, series := range reason.signal {
		reason.signal[category] = series[:0]
	}

	reason.price = reason.price[:0]
	reason.volume = reason.volume[:0]
	reason.spread = reason.spread[:0]

	var lastPrice float64
	var lastVolume float64

	for index := len(snapshots) - 1; index >= 0; index-- {
		measurement := snapshots[index]

		if measurement.Category != CategoryTypeNone {
			reason.signal[measurement.Category] = append(
				reason.signal[measurement.Category], measurement,
			)
		}

		if measurement.Last > 0 && measurement.Last != lastPrice {
			reason.price = append(reason.price, measurement.Last)
			lastPrice = measurement.Last
		}

		if measurement.Volume > 0 && measurement.Volume != lastVolume {
			reason.volume = append(reason.volume, measurement.Volume)
			lastVolume = measurement.Volume
		}

		reason.spread = append(reason.spread, measurement.SpreadBPS)
	}

	return reason
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
	return (&WindowReason{}).Reset(snapshots, regime, position)
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
		if !position.Holding {
			return false
		}

		if position.Side == trading.Sell {
			return position.Trough > 0 &&
				position.Last >= position.Trough*(1+reason.endedPct)
		}

		return position.Peak > 0 &&
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

	if position.Side == trading.Sell {
		return position.EntryPrice > 0 &&
			position.Trough > 0 &&
			position.Trough <= position.EntryPrice*(1-reason.continuedPct)
	}

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
	case SubjectVolume:
		if ago < 0 || ago >= len(reason.volume) {
			return 0, false
		}

		return reason.volume[ago], true
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
		return 0, false
	}
}
