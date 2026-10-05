package derivatives

import (
	"errors"
	"iter"
	"math"
	"strconv"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
basisState is one symbol's derivative/reference state: its causal timeline,
the previous observation its differences are measured against, and the two
estimators that baseline the basis and the open-interest growth.
*/
type basisState struct {
	clock map[string]time.Time

	hasPrev   bool
	prevTime  time.Time
	prevLast  float64
	prevIndex float64
	prevOI    float64
	prevBasis float64

	prevBasisRate    float64
	hasPrevBasisRate bool
	prevReturnGap    float64
	hasPrevReturnGap bool

	basis  core.Primitive
	growth core.Primitive
}

/*
Basis owns the derivative/reference measurement for every symbol. It measures
basis, open-interest growth, and cross-instrument relative returns, writing
every metric into the measurement where it computes it.
*/
type Basis struct {
	err    error
	states map[string]*basisState
}

func NewBasis() core.Primitive {
	return &Basis{states: make(map[string]*basisState)}
}

func (op *Basis) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement)(arriving)

			if m.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			state, existing := op.states[m.Label]

			if !existing {
				state = &basisState{
					clock:  make(map[string]time.Time),
					basis:  adaptive.NewBaseline(adaptive.NewWindow()),
					growth: adaptive.NewBaseline(adaptive.NewWindow()),
				}
				op.states[m.Label] = state
			}

			op.observe(m, state)

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Basis) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
observe measures one ticker snapshot: the instantaneous geometry always
publishes, while everything derived from the event clock publishes only when
the symbol's causal timeline advanced.
*/
func (op *Basis) observe(m *data.Measurement, state *basisState) {
	stamped, advanced := stamp(state.clock, m.Label, m.At, func() bool { v, _ := m.GetProvenance("synthetic_timestamp"); return v == "true" }())
	m.At = stamped

	m.EnsureMetadata()

	last := m.GetMetric("last").Raw
	index := m.GetMetric("index_price").Raw
	mark := m.GetMetric("mark_price").Raw
	oi := m.GetMetric("open_interest").Raw

	basis := (last - index) / index
	basisReading := drive[float64, adaptive.BaselineReading](state.basis, &basis)

	m.From = stamped

	lastMetric, _ := m.LookupMetric("last")
	indexMetric, _ := m.LookupMetric("index_price")
	markMetric, _ := m.LookupMetric("mark_price")
	oiMetric, _ := m.LookupMetric("open_interest")

	priceSpread := math.Max(math.Abs(last-index), 1e-4)

	derivCenter, derivScale := lastMetric.Center, lastMetric.Scale
	if derivScale == 0 {
		derivCenter, derivScale = last, priceSpread
	}
	refCenter, refScale := indexMetric.Center, indexMetric.Scale
	if refScale == 0 {
		refCenter, refScale = index, priceSpread
	}
	spotCenter, spotScale := markMetric.Center, markMetric.Scale
	if spotScale == 0 {
		spotCenter, spotScale = mark, priceSpread
	}

	m.SetMetric("derivative_price", data.NewMetric(
		"derivative_price",
		data.UnitPrice,
		data.TimescaleInstantaneous,
		derivCenter,
		derivScale,
	).Write(last))
	m.SetMetric("reference_price", data.NewMetric(
		"reference_price",
		data.UnitPrice,
		data.TimescaleInstantaneous,
		refCenter,
		refScale,
	).Write(index))
	m.SetMetric("spot_price", data.NewMetric(
		"spot_price",
		data.UnitPrice,
		data.TimescaleInstantaneous,
		spotCenter,
		spotScale,
	).Write(mark))

	oiCenter := oiMetric.Center
	oiScale := oiMetric.Scale
	if oiScale == 0 {
		oiCenter = oi
		oiScale = math.Max(oi, 1.0)
	}
	m.SetMetric("open_interest", data.NewMetric(
		"open_interest",
		data.UnitQuantity,
		data.TimescaleInstantaneous,
		oiCenter,
		oiScale,
	).Write(oi))

	basisDispersion := math.Max(basisReading.Dispersion, 1e-4)
	m.SetMetric("basis", data.NewMetric(
		"basis",
		data.UnitRatio,
		data.TimescaleInstantaneous,
		0.0,
		basisDispersion,
	).Write(basis))
	m.SetMetric("basis_baseline", data.NewMetric(
		"basis_baseline",
		data.UnitRatio,
		data.TimescaleRollingWindow,
		0.0,
		basisDispersion,
	).Write(basisReading.Baseline))

	if basisReading.HasPrior {
		m.SetMetric("basis_zscore", data.NewMetric(
			"basis_zscore",
			data.UnitZScore,
			data.TimescaleRollingWindow,
			0.0,
			1.0,
		).Write(basisReading.ZScore))
	}

	if last > 0 && index > 0 {
		logBasis := math.Log(last / index)
		m.SetMetric("log_basis", data.NewMetric(
			"log_basis",
			data.UnitLogReturn,
			data.TimescaleInstantaneous,
			0.0,
			basisDispersion,
		).Write(logBasis))

		if mark > 0 {
			m.SetMetric("derivative_index_log_basis", data.NewMetric(
				"derivative_index_log_basis",
				data.UnitLogReturn,
				data.TimescaleInstantaneous,
				0.0,
				basisDispersion,
			).Write(math.Log(last/index)))
			m.SetMetric("index_spot_log_basis", data.NewMetric(
				"index_spot_log_basis",
				data.UnitLogReturn,
				data.TimescaleInstantaneous,
				0.0,
				basisDispersion,
			).Write(math.Log(index/mark)))
			m.SetMetric("derivative_spot_log_basis", data.NewMetric(
				"derivative_spot_log_basis",
				data.UnitLogReturn,
				data.TimescaleInstantaneous,
				0.0,
				basisDispersion,
			).Write(math.Log(last/mark)))
			m.SetMetric("basis_closure_error", data.NewMetric(
				"basis_closure_error",
				data.UnitRatio,
				data.TimescaleInstantaneous,
				0.0,
				1.0,
			).Write(0.0))
		}
	}

	if advanced && state.hasPrev {
		op.differences(m, state, stamped, last, index, oi, basis)
	}

	if advanced {
		state.prevTime = stamped
		state.prevLast = last
		state.prevIndex = index
		state.prevOI = oi
		state.prevBasis = basis
		state.hasPrev = true
	}

	// The basis z-score is scored against the moments held BEFORE this
	// observation, so the evidence backing it is the prior count — one prior
	// sample carries no dispersion of its own and is immature. A measurement
	// with no estimator behind it is a whole direct reading and declares no
	// support at all.
	if basisReading.HasPrior {
		m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(basisReading.Prior.Count, 'f', -1, 64))

		// basis_zscore is this entity's headline reading, so its estimator
		// supplies the departure and the noise power Finalize turns into SNR.
		if basisReading.PriorVariance > 0 {
			m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(basisReading.Residual, 'f', -1, 64))
			m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(basisReading.PriorVariance, 'f', -1, 64))
		}
	}
}

/*
differences derives the event-clock facts: open-interest growth, basis rate
and velocity, and the relative return gap.
*/
func (op *Basis) differences(
	m *data.Measurement,
	state *basisState,
	stamped time.Time,
	last, index, oi, basis float64,
) {
	dt := stamped.Sub(state.prevTime).Seconds()

	oiChange := oi - state.prevOI
	m.SetMetric("open_interest_change", data.NewMetric(
		"open_interest_change",
		data.UnitQuantity,
		data.TimescaleInstantaneous,
		0.0,
		math.Max(math.Abs(oiChange), 1.0),
	).Write(oiChange))

	if state.prevOI > 0 && oi > 0 {
		oiLogChange := math.Log(oi / state.prevOI)
		m.SetMetric("open_interest_log_change", data.NewMetric(
			"open_interest_log_change",
			data.UnitLogReturn,
			data.TimescaleInstantaneous,
			0.0,
			math.Max(math.Abs(oiLogChange), 1e-4),
		).Write(oiLogChange))

		if dt > 0 {
			oiGrowthRate := oiLogChange / dt
			m.SetMetric("open_interest_growth_rate", data.NewMetric(
				"open_interest_growth_rate",
				data.UnitRate,
				data.TimescalePerSecond,
				0.0,
				math.Max(math.Abs(oiGrowthRate), 1e-4),
			).Write(oiGrowthRate))

			growth := drive[float64, adaptive.BaselineReading](state.growth, &oiGrowthRate)
			m.SetMetric("open_interest_growth_baseline", data.NewMetric(
				"open_interest_growth_baseline",
				data.UnitRate,
				data.TimescaleRollingWindow,
				0.0,
				math.Max(growth.Dispersion, 1e-4),
			).Write(growth.Baseline))
		}
	}

	if dt > 0 {
		basisChange := basis - state.prevBasis
		basisRate := basisChange / dt
		m.SetMetric("basis_change", data.NewMetric(
			"basis_change",
			data.UnitRatio,
			data.TimescaleInstantaneous,
			0.0,
			math.Max(math.Abs(basisChange), 1e-4),
		).Write(basisChange))
		m.SetMetric("basis_rate", data.NewMetric(
			"basis_rate",
			data.UnitRate,
			data.TimescalePerSecond,
			0.0,
			math.Max(math.Abs(basisRate), 1e-4),
		).Write(basisRate))

		if state.hasPrevBasisRate {
			basisVel := basisRate - state.prevBasisRate
			m.SetMetric("basis_velocity", data.NewMetric(
				"basis_velocity",
				data.UnitVelocity,
				data.TimescalePerSecond,
				0.0,
				math.Max(math.Abs(basisVel), 1e-4),
			).Write(basisVel))
		}

		state.prevBasisRate = basisRate
		state.hasPrevBasisRate = true
	}

	op.returns(m, state, last, index)
}

/*
returns derives the two legs' log returns and the gap between them.
*/
func (op *Basis) returns(m *data.Measurement, state *basisState, last, index float64) {
	var hasDerivReturn, hasRefReturn bool
	var derivLogReturn, refLogReturn float64

	if state.prevLast > 0 && last > 0 {
		derivLogReturn = math.Log(last / state.prevLast)
		m.SetMetric("derivative_log_return", data.NewMetric(
			"derivative_log_return",
			data.UnitLogReturn,
			data.TimescaleInstantaneous,
			0.0,
			math.Max(math.Abs(derivLogReturn), 1e-4),
		).Write(derivLogReturn))
		hasDerivReturn = true
	}

	if state.prevIndex > 0 && index > 0 {
		refLogReturn = math.Log(index / state.prevIndex)
		m.SetMetric("reference_log_return", data.NewMetric(
			"reference_log_return",
			data.UnitLogReturn,
			data.TimescaleInstantaneous,
			0.0,
			math.Max(math.Abs(refLogReturn), 1e-4),
		).Write(refLogReturn))
		hasRefReturn = true
	}

	if hasDerivReturn && hasRefReturn {
		returnGap := derivLogReturn - refLogReturn
		m.SetMetric("return_gap", data.NewMetric(
			"return_gap",
			data.UnitLogReturn,
			data.TimescaleInstantaneous,
			0.0,
			math.Max(math.Abs(returnGap), 1e-4),
		).Write(returnGap))

		if state.hasPrevReturnGap {
			gapVel := returnGap - state.prevReturnGap
			m.SetMetric("return_gap_velocity", data.NewMetric(
				"return_gap_velocity",
				data.UnitVelocity,
				data.TimescalePerSecond,
				0.0,
				math.Max(math.Abs(gapVel), 1e-4),
			).Write(gapVel))
		}

		state.prevReturnGap = returnGap
		state.hasPrevReturnGap = true
	}
}

/*
stamp folds one event onto the symbol's causal timeline.

It returns the event time to use and whether the timeline advanced. A false
`advanced` means the caller holds a genuinely late event: its facts are still
real and should still be accounted, but must not advance the latest event time
or event-time derivatives.

A SYNTHETIC timestamp holds no truth about when the event happened, so it is
folded forward onto the timeline. A REAL timestamp that regresses is a
genuinely late event and is reported as late. A zero timestamp is never
stamped: it would read as a regression after any real observation, poisoning
the first valid event.
*/
func stamp(clock map[string]time.Time, symbol string, timestamp time.Time, synthetic bool) (stamped time.Time, advanced bool) {
	if timestamp.IsZero() {
		return timestamp, true
	}

	previous := clock[symbol]

	if timestamp.Before(previous) {
		if synthetic {
			return previous, true
		}

		return timestamp, false
	}

	clock[symbol] = timestamp

	return timestamp, true
}
