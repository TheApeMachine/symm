package derivatives

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/types"
)

type tickerState struct {
	basisPipeline  *nomagique.Pipeline
	basisDynamics  *equation.AdaptiveZScore
	growthPipeline *nomagique.Pipeline
	growthDynamics *equation.CausalResidual

	hasPrev   bool
	prevTime  time.Time
	prevLast  float64
	prevIndex float64
	prevSpot  float64
	prevOI    float64
	prevBasis float64
}

/*
Ticker is the derivative/reference state market entity. It measures basis,
open-interest growth, and cross-instrument relative returns using native nomagique
v2 equations without Frame or Wire blocks.
*/
type Ticker struct {
	states map[string]*tickerState
	clock  causalClock
	mu     sync.RWMutex
}

func NewTicker() *Ticker {
	return &Ticker{
		states: make(map[string]*tickerState),
		clock:  newCausalClock(),
	}
}

func (ticker *Ticker) Close() error {
	return nil
}

func (ticker *Ticker) Step(point kraken.FuturesTickerData) *data.Measurement[float64] {
	stamped, advanced := ticker.clock.stamp(
		point.Symbol, point.Timestamp, point.SyntheticTimestamp,
	)
	point.Timestamp = stamped

	last := point.Last.Float64()
	index := point.IndexPrice.Float64()
	mark := point.MarkPrice.Float64()
	oi := point.OpenInterest

	if index <= 0 || mark <= 0 || last < 0 || oi < 0 {
		return &data.Measurement[float64]{
			Err: fmt.Errorf("derivatives: non-positive prices or oi (last=%f, index=%f, mark=%f, oi=%f)", last, index, mark, oi),
		}
	}

	ticker.mu.Lock()
	defer ticker.mu.Unlock()

	state, found := ticker.states[point.Symbol]

	if !found {
		basisDyn := &equation.AdaptiveZScore{}
		growthDyn := &equation.CausalResidual{}
		state = &tickerState{
			basisDynamics:  basisDyn,
			basisPipeline:  nomagique.Number(basisDyn),
			growthDynamics: growthDyn,
			growthPipeline: nomagique.Number(growthDyn),
		}
		ticker.states[point.Symbol] = state
	}

	basis := (last - index) / index

	state.basisPipeline.Step(types.Scalar(basis))
	basisDyn := state.basisDynamics

	id := fmt.Sprintf("derivatives:%s:%d", point.Symbol, point.Timestamp.UnixNano())
	measurement := data.NewMeasurement[float64](id, point.Symbol, "derivatives", point.Timestamp, point.Timestamp)
	measurement.Metadata = make(map[string]float64)

	putDerivMetric(measurement, "derivative_price", last, data.UnitRate)
	putDerivMetric(measurement, "reference_price", index, data.UnitRate)
	putDerivMetric(measurement, "spot_price", mark, data.UnitRate)
	putDerivMetric(measurement, "open_interest", oi, data.UnitCount)
	putDerivMetric(measurement, "basis", basis, data.UnitDimensionless)

	putDerivMetric(measurement, "basis_baseline", float64(basisDyn.Baseline()), data.UnitDimensionless)

	if basisDyn.HasPrior() {
		putDerivMetric(measurement, "basis_zscore", float64(basisDyn.ZScore()), data.UnitDimensionless)
	}

	if last > 0 && index > 0 {
		logBasis := math.Log(last / index)
		putDerivMetric(measurement, "log_basis", logBasis, data.UnitDimensionless)

		if mark > 0 {
			derivIndexBasis := math.Log(last / index)
			indexSpotBasis := math.Log(index / mark)
			derivSpotBasis := math.Log(last / mark)

			putDerivMetric(measurement, "derivative_index_log_basis", derivIndexBasis, data.UnitDimensionless)
			putDerivMetric(measurement, "index_spot_log_basis", indexSpotBasis, data.UnitDimensionless)
			putDerivMetric(measurement, "derivative_spot_log_basis", derivSpotBasis, data.UnitDimensionless)
			putDerivMetric(measurement, "basis_closure_error", 0.0, data.UnitDimensionless)
		}
	}

	if advanced && state.hasPrev {
		dt := stamped.Sub(state.prevTime).Seconds()

		oiChange := oi - state.prevOI
		putDerivMetric(measurement, "open_interest_change", oiChange, data.UnitCount)

		if state.prevOI > 0 && oi > 0 {
			oiLogChange := math.Log(oi / state.prevOI)
			putDerivMetric(measurement, "open_interest_log_change", oiLogChange, data.UnitDimensionless)

			if dt > 0 {
				oiGrowthRate := oiLogChange / dt
				putDerivMetric(measurement, "open_interest_growth_rate", oiGrowthRate, data.UnitPerSecond)

				state.growthPipeline.Step(types.Scalar(oiGrowthRate))
				putDerivMetric(measurement, "open_interest_growth_baseline", float64(state.growthDynamics.Baseline()), data.UnitPerSecond)
			}
		}

		if dt > 0 {
			basisChange := basis - state.prevBasis
			basisRate := basisChange / dt
			putDerivMetric(measurement, "basis_change", basisChange, data.UnitDimensionless)
			putDerivMetric(measurement, "basis_rate", basisRate, data.UnitPerSecond)
		}

		var hasDerivReturn, hasRefReturn bool
		var derivLogReturn, refLogReturn float64

		if state.prevLast > 0 && last > 0 {
			derivLogReturn = math.Log(last / state.prevLast)
			putDerivMetric(measurement, "derivative_log_return", derivLogReturn, data.UnitDimensionless)
			hasDerivReturn = true
		}

		if state.prevIndex > 0 && index > 0 {
			refLogReturn = math.Log(index / state.prevIndex)
			putDerivMetric(measurement, "reference_log_return", refLogReturn, data.UnitDimensionless)
			hasRefReturn = true
		}

		if hasDerivReturn && hasRefReturn {
			putDerivMetric(measurement, "return_gap", derivLogReturn-refLogReturn, data.UnitDimensionless)
		}
	}

	if advanced {
		state.prevTime = stamped
		state.prevLast = last
		state.prevIndex = index
		state.prevSpot = mark
		state.prevOI = oi
		state.prevBasis = basis
		state.hasPrev = true
	}

	// Quality is derived by Finalize from the measurement's own facts, never
	// assigned here. The basis z-score is scored against the moments held
	// BEFORE this observation, so the evidence backing it is the prior count —
	// one prior sample carries no dispersion of its own and is immature.
	// A measurement with no estimator behind it is a whole direct reading and
	// declares no support at all.
	if basisDyn.HasPrior() {
		measurement.Metadata[data.MetadataSupport] = basisDyn.PriorCount()

		// basis_zscore is this entity's headline reading, so its estimator
		// supplies the departure and the noise power Finalize turns into SNR.
		// Without them the measurement projected its metrics but reported no
		// SNR at all, which reads downstream as a kernel that never measured.
		dispersion := float64(basisDyn.PriorDispersion())

		if dispersion > 0 {
			measurement.Metadata[data.MetadataDivergence] =
				float64(basisDyn.Divergence())
			measurement.Metadata[data.MetadataNoiseVariance] = dispersion * dispersion
		}
	}

	measurement.Finalize()

	return measurement
}
