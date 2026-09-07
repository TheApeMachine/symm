package derivatives

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

type tickerState struct {
	basisGraph  core.Primitive
	growthGraph core.Primitive

	hasPrev   bool
	prevTime  time.Time
	prevLast  float64
	prevIndex float64
	prevSpot  float64
	prevOI    float64
	prevBasis float64

	prevBasisRate    float64
	hasPrevBasisRate bool
	prevReturnGap    float64
	hasPrevReturnGap bool
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
	if point.Last == nil || point.IndexPrice == nil || point.MarkPrice == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("derivatives: last, index, and mark prices required")}
	}
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
		state = &tickerState{
			basisGraph:  equation.NewCausalResidual(adaptive.NewBaseline(adaptive.NewWindow())),
			growthGraph: equation.NewCausalResidual(adaptive.NewBaseline(adaptive.NewWindow())),
		}
		ticker.states[point.Symbol] = state
	}

	basis := (last - index) / index

	fields, err := transport.Evaluate[map[string]core.Primitive](state.basisGraph, core.From(basis))
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}
	basisDyn := core.NewDecoder(fields)

	id := fmt.Sprintf("derivatives:%s:%d", point.Symbol, point.Timestamp.UnixNano())
	measurement := data.NewMeasurement[float64](id, point.Symbol, "derivatives", point.Timestamp, point.Timestamp)
	measurement.Metadata = make(map[string]float64)

	putDerivMetric(measurement, "derivative_price", last, data.UnitRate)
	putDerivMetric(measurement, "reference_price", index, data.UnitRate)
	putDerivMetric(measurement, "spot_price", mark, data.UnitRate)
	putDerivMetric(measurement, "open_interest", oi, data.UnitCount)
	putDerivMetric(measurement, "basis", basis, data.UnitDimensionless)

	putDerivMetric(measurement, "basis_baseline", core.Decode[float64](basisDyn, "baseline"), data.UnitDimensionless)

	if core.Decode[bool](basisDyn, "has_prior") {
		putDerivMetric(measurement, "basis_zscore", core.Decode[float64](basisDyn, "zscore"), data.UnitDimensionless)
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

				growth, err := transport.Evaluate[map[string]core.Primitive](state.growthGraph, core.From(oiGrowthRate))
				if err != nil {
					measurement.Err = err
					return measurement
				}
				baseline, err := core.Field[float64](growth, "baseline")
				if err != nil {
					measurement.Err = err
					return measurement
				}
				putDerivMetric(measurement, "open_interest_growth_baseline", baseline, data.UnitPerSecond)
			}
		}

		if dt > 0 {
			basisChange := basis - state.prevBasis
			basisRate := basisChange / dt
			putDerivMetric(measurement, "basis_change", basisChange, data.UnitDimensionless)
			putDerivMetric(measurement, "basis_rate", basisRate, data.UnitPerSecond)

			// Velocity is the change in the RATE between consecutive
			// observations: basis_rate says how fast the basis is moving,
			// basis_velocity says whether that movement is accelerating.
			if state.hasPrevBasisRate {
				putDerivMetric(
					measurement, "basis_velocity",
					basisRate-state.prevBasisRate, data.UnitPerSecond,
				)
			}

			state.prevBasisRate = basisRate
			state.hasPrevBasisRate = true
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
			returnGap := derivLogReturn - refLogReturn
			putDerivMetric(measurement, "return_gap", returnGap, data.UnitDimensionless)

			if state.hasPrevReturnGap {
				putDerivMetric(
					measurement, "return_gap_velocity",
					returnGap-state.prevReturnGap, data.UnitDimensionless,
				)
			}

			state.prevReturnGap = returnGap
			state.hasPrevReturnGap = true
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
	if core.Decode[bool](basisDyn, "has_prior") {
		measurement.Metadata[data.MetadataSupport] = core.Decode[float64](basisDyn, "prior_count")

		// basis_zscore is this entity's headline reading, so its estimator
		// supplies the departure and the noise power Finalize turns into SNR.
		// Without them the measurement projected its metrics but reported no
		// SNR at all, which reads downstream as a kernel that never measured.
		variance := core.Decode[float64](basisDyn, "prior_variance")

		if variance > 0 {
			measurement.Metadata[data.MetadataDivergence] =
				core.Decode[float64](basisDyn, "residual")
			measurement.Metadata[data.MetadataNoiseVariance] = variance
		}
	}

	measurement.Err = basisDyn.Error()
	measurement.Finalize()

	return measurement
}
