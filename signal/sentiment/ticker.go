package sentiment

import (
	"fmt"
	"math"
	"sync"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

type symbolState struct {
	previousPrice float64
	previousSec   float64
	previousNsec  float64
	count         int
	hasPrice      bool
}

/*
Ticker is the per-symbol price-state market entity. It tracks each symbol's
price path to derive log returns and folds them into the shared cross-section.
Outputs are directly projected into data.Measurement without intermediate
Frame allocations or string intern table lookups.
*/
type Ticker struct {
	section *data.CrossSection
	symbols map[string]*symbolState
	mu      sync.RWMutex
}

/*
NewTicker constructs the Ticker entity and its cross-section.
*/
func NewTicker() *Ticker {
	return &Ticker{
		section: data.NewCrossSection(),
		symbols: make(map[string]*symbolState),
	}
}

/*
Step receives one ticker data point, updates the per-symbol path,
folds the price into the shared cross-section, and directly emits the
resulting Measurement.
*/
func (ticker *Ticker) Step(tick kraken.TickerData) *data.Measurement[float64] {
	if tick.Last == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("sentiment: ticker requires a last price")}
	}

	last := tick.Last.Float64()

	if last < 0 {
		return &data.Measurement[float64]{Err: fmt.Errorf("sentiment: ticker last price must be non-negative")}
	}

	id := fmt.Sprintf("sentiment:%s:%d", tick.Symbol, tick.Timestamp.UnixNano())

	if last == 0 {
		measurement := data.NewMeasurement[float64](id, tick.Symbol, "sentiment", tick.Timestamp, tick.Timestamp)
		measurement.Metadata = make(map[string]float64)
		measurement.Metadata[data.MetadataSupport] = 0.0
		measurement.Maturity = 0.0
		measurement.Provenance = map[string]string{
			"last_trade_price_state": "unobserved",
		}

		return measurement
	}

	ticker.mu.Lock()
	state, found := ticker.symbols[tick.Symbol]

	if !found {
		state = &symbolState{}
		ticker.symbols[tick.Symbol] = state
	}

	tickSec := float64(tick.Timestamp.Unix())
	tickNsec := float64(tick.Timestamp.Nanosecond())

	if state.hasPrice {
		if tickSec < state.previousSec || (tickSec == state.previousSec && tickNsec < state.previousNsec) {
			ticker.mu.Unlock()

			return nil
		}
	}

	state.count++
	previousPrice := state.previousPrice
	hasPrevious := state.hasPrice

	state.previousPrice = last
	state.previousSec = tickSec
	state.previousNsec = tickNsec
	state.hasPrice = true
	ticker.mu.Unlock()

	measurement := data.NewMeasurement[float64](id, tick.Symbol, "sentiment", tick.Timestamp, tick.Timestamp)
	measurement.Metadata = make(map[string]float64)

	if hasPrevious && previousPrice > 0 && last > 0 {
		logReturn := math.Log(last / previousPrice)
		putMetric(measurement, "return", logReturn, data.UnitDimensionless)
		putMetric(measurement, "absolute_return", math.Abs(logReturn), data.UnitDimensionless)
	}

	snapshot, hasSnapshot := ticker.section.Process(
		tick.Symbol,
		last,
		tick.Timestamp,
		tick.Symbol,
	)

	if hasSnapshot {
		foldSnapshot(measurement, snapshot)

		if measurement.Provenance == nil {
			measurement.Provenance = make(map[string]string)
		}

		measurement.Provenance["largest_move_symbol"] = snapshot.ExtremeKey
	}

	measurement.Metadata[data.MetadataSupport] = float64(state.count)
	measurement.Finalize()

	return measurement
}

func (ticker *Ticker) Close() error { return nil }

func foldSnapshot(measurement *data.Measurement[float64], snapshot data.Snapshot) {
	valid := float64(snapshot.Count)
	positive := float64(snapshot.PositiveCount)
	negative := float64(snapshot.NegativeCount)
	zero := float64(snapshot.ZeroCount)

	putMetric(measurement, "advance_count", positive, data.UnitCount)
	putMetric(measurement, "decline_count", negative, data.UnitCount)
	putMetric(measurement, "unchanged_count", zero, data.UnitCount)
	putMetric(measurement, "valid_member_count", valid, data.UnitCount)
	putMetric(measurement, "cohort_member_count", float64(snapshot.TotalMembers), data.UnitCount)
	putMetric(measurement, "excluded_member_count", float64(snapshot.TotalMembers-snapshot.Count), data.UnitCount)

	putRatio(measurement, "advance_fraction", positive, valid, data.UnitDimensionless)
	putRatio(measurement, "decline_fraction", negative, valid, data.UnitDimensionless)
	putRatio(measurement, "unchanged_fraction", zero, valid, data.UnitDimensionless)
	putRatio(measurement, "directional_participation", positive+negative, valid, data.UnitDimensionless)
	putRatio(measurement, "directional_agreement", math.Max(positive, negative), positive+negative, data.UnitDimensionless)
	putRatio(measurement, "directional_consensus", math.Abs(positive-negative), positive+negative, data.UnitDimensionless)

	peerCount := float64(snapshot.Count - 1)
	putMetric(measurement, "same_direction_peer_count", float64(snapshot.SameDirectionCount), data.UnitCount)
	putMetric(measurement, "opposite_direction_peer_count", float64(snapshot.OppositeDirectionCount), data.UnitCount)
	putMetric(measurement, "zero_return_peer_count", float64(snapshot.ZeroDirectionCount), data.UnitCount)

	putRatio(measurement, "same_direction_peer_fraction", float64(snapshot.SameDirectionCount), peerCount, data.UnitDimensionless)
	putRatio(measurement, "opposite_direction_peer_fraction", float64(snapshot.OppositeDirectionCount), peerCount, data.UnitDimensionless)
	putRatio(measurement, "zero_return_peer_fraction", float64(snapshot.ZeroDirectionCount), peerCount, data.UnitDimensionless)

	emitAggregate(measurement, snapshot.Aggregates["signed_fraction"], "breadth", true, true, true, true)
	emitQuality(measurement, snapshot.Aggregates["signed_fraction"])

	emitAggregate(measurement, snapshot.Aggregates["signed_median"], "median_return", true, true, true, true)

	medianAbsView := snapshot.Aggregates["median_absolute"]
	emitAggregate(measurement, medianAbsView, "median_absolute_return", true, false, true, true)
	emitRatio(measurement, "median_absolute_return_ratio", medianAbsView)

	emitAggregate(measurement, snapshot.Aggregates["mean_absolute"], "mean_absolute_return", false, false, false, false)
	emitAggregate(measurement, snapshot.Aggregates["rms"], "rms_return", false, false, false, false)

	iqrView := snapshot.Aggregates["iqr"]
	putMetric(measurement, "return_interquartile_range", iqrView.Value, data.UnitDimensionless)

	if iqrView.Ready {
		putMetric(measurement, "return_dispersion_baseline", iqrView.Baseline, data.UnitDimensionless)
		putMetric(measurement, "return_dispersion_zscore", iqrView.ZScore, data.UnitDimensionless)
		putMetric(measurement, "return_dispersion_velocity", iqrView.Velocity, data.UnitPerSecond)

		if iqrView.Baseline != 0 {
			putMetric(measurement, "return_dispersion_ratio", iqrView.Value/iqrView.Baseline, data.UnitDimensionless)
		}
	}

	putMetric(measurement, "return_mad", snapshot.Mad, data.UnitDimensionless)
	putMetric(measurement, "magnitude_mad", snapshot.MagnitudeMad, data.UnitDimensionless)

	putMetric(measurement, "largest_absolute_return", snapshot.ExtremeMagnitude, data.UnitDimensionless)
	putMetric(measurement, "largest_move_tie_count", float64(snapshot.ExtremeTieCount), data.UnitCount)

	if snapshot.ExtremeTieCount == 0 {
		putMetric(measurement, "largest_signed_return", snapshot.ExtremeSigned, data.UnitDimensionless)
	}

	putMetric(measurement, "largest_move_excess", snapshot.ExtremeMagnitude-snapshot.PeerMedianAbsolute, data.UnitDimensionless)
	putRatio(measurement, "largest_move_mad_excess", snapshot.ExtremeMagnitude-snapshot.PeerMedianAbsolute, snapshot.PeerMad, data.UnitDimensionless)

	extremeRatioView := snapshot.Aggregates["extreme_ratio"]
	putMetric(measurement, "largest_move_ratio", extremeRatioView.Value, data.UnitDimensionless)

	if extremeRatioView.Ready {
		putMetric(measurement, "largest_move_ratio_baseline", extremeRatioView.Baseline, data.UnitDimensionless)
		putMetric(measurement, "largest_move_ratio_zscore", extremeRatioView.ZScore, data.UnitDimensionless)
	}

	extremeShareView := snapshot.Aggregates["extreme_share"]
	putMetric(measurement, "largest_move_share", extremeShareView.Value, data.UnitDimensionless)

	if extremeShareView.Ready {
		putMetric(measurement, "largest_move_share_baseline", extremeShareView.Baseline, data.UnitDimensionless)
		putMetric(measurement, "largest_move_share_zscore", extremeShareView.ZScore, data.UnitDimensionless)
	}

	putMetric(measurement, "peer_median_absolute_return", snapshot.PeerMedianAbsolute, data.UnitDimensionless)
	putMetric(measurement, "peer_magnitude_mad", snapshot.PeerMad, data.UnitDimensionless)

	putMetric(measurement, "median_asof_age_seconds", snapshot.MedianAge, data.UnitSecond)
	putMetric(measurement, "max_asof_age_seconds", snapshot.MaxAge, data.UnitSecond)
	putMetric(measurement, "median_from_age_seconds", snapshot.MedianFromAge, data.UnitSecond)
	putMetric(measurement, "cohort_horizon_seconds", snapshot.MaxAge, data.UnitSecond)
	putMetric(measurement, "asof_age_seconds", snapshot.FocalAge, data.UnitSecond)
	putMetric(measurement, "from_age_seconds", snapshot.FocalFromAge, data.UnitSecond)
}

func putMetric(measurement *data.Measurement[float64], name string, value float64, unit data.Unit) {
	measurement.PutMetric(data.NewMetric(
		name, value, nil, nil, unit, data.TimescaleInstantaneous,
	))
}

func putRatio(measurement *data.Measurement[float64], name string, num, den float64, unit data.Unit) {
	if den == 0 {
		return
	}

	putMetric(measurement, name, num/den, unit)
}

func emitAggregate(
	measurement *data.Measurement[float64],
	view data.AggregateView,
	name string,
	hasBaseline, hasDivergence, hasZScore, hasVelocity bool,
) {
	if name != "" {
		putMetric(measurement, name, view.Value, data.UnitDimensionless)
	}

	if !view.Ready {
		return
	}

	if hasBaseline {
		putMetric(measurement, name+"_baseline", view.Baseline, data.UnitDimensionless)
	}

	if hasDivergence {
		putMetric(measurement, name+"_divergence", view.Divergence, data.UnitDimensionless)
	}

	if hasZScore {
		putMetric(measurement, name+"_zscore", view.ZScore, data.UnitDimensionless)
	}

	if hasVelocity {
		putMetric(measurement, name+"_velocity", view.Velocity, data.UnitPerSecond)
	}
}

func emitRatio(measurement *data.Measurement[float64], name string, view data.AggregateView) {
	if !view.Ready || view.Baseline == 0 {
		return
	}

	putMetric(measurement, name, view.Value/view.Baseline, data.UnitDimensionless)
}

func emitQuality(measurement *data.Measurement[float64], view data.AggregateView) {
	if !view.Ready || view.NoiseVariance <= 0 {
		return
	}

	measurement.Metadata[data.MetadataDivergence] = view.Divergence
	measurement.Metadata[data.MetadataNoiseVariance] = view.NoiseVariance
}
