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
			m := *(**data.Measurement[float64])(arriving)

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
func (op *Basis) observe(m *data.Measurement[float64], state *basisState) {
	stamped, advanced := stamp(state.clock, m.Label, m.At, m.Provenance["synthetic_timestamp"] == "true")
	m.At = stamped

	m.EnsureMetadata()

	last := m.GetMetric("last").Raw
	index := m.GetMetric("index_price").Raw
	mark := m.GetMetric("mark_price").Raw
	oi := m.GetMetric("open_interest").Raw

	basis := (last - index) / index
	basisReading := drive[float64, adaptive.BaselineReading](state.basis, &basis)

	m.From = stamped

	m.WriteMetric("derivative_price", last)
	m.WriteMetric("reference_price", index)
	m.WriteMetric("spot_price", mark)
	m.WriteMetric("open_interest", oi)
	m.WriteMetric("basis", basis)
	m.WriteMetric("basis_baseline", basisReading.Baseline)

	if basisReading.HasPrior {
		m.WriteMetric("basis_zscore", basisReading.ZScore)
	}

	if last > 0 && index > 0 {
		m.WriteMetric("log_basis", math.Log(last / index))

		if mark > 0 {
			m.WriteMetric("derivative_index_log_basis", math.Log(last / index))
			m.WriteMetric("index_spot_log_basis", math.Log(index / mark))
			m.WriteMetric("derivative_spot_log_basis", math.Log(last / mark))
			m.WriteMetric("basis_closure_error", 0.0)
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
	m *data.Measurement[float64],
	state *basisState,
	stamped time.Time,
	last, index, oi, basis float64,
) {
	dt := stamped.Sub(state.prevTime).Seconds()

	oiChange := oi - state.prevOI
	m.WriteMetric("open_interest_change", oiChange)

	if state.prevOI > 0 && oi > 0 {
		oiLogChange := math.Log(oi / state.prevOI)
		m.WriteMetric("open_interest_log_change", oiLogChange)

		if dt > 0 {
			oiGrowthRate := oiLogChange / dt
			m.WriteMetric("open_interest_growth_rate", oiGrowthRate)

			growth := drive[float64, adaptive.BaselineReading](state.growth, &oiGrowthRate)
			m.WriteMetric("open_interest_growth_baseline", growth.Baseline)
		}
	}

	if dt > 0 {
		basisChange := basis - state.prevBasis
		basisRate := basisChange / dt
		m.WriteMetric("basis_change", basisChange)
		m.WriteMetric("basis_rate", basisRate)

		// Velocity is the change in the RATE between consecutive
		// observations: basis_rate says how fast the basis is moving,
		// basis_velocity says whether that movement is accelerating.
		if state.hasPrevBasisRate {
			m.WriteMetric("basis_velocity", basisRate - state.prevBasisRate)
		}

		state.prevBasisRate = basisRate
		state.hasPrevBasisRate = true
	}

	op.returns(m, state, last, index)
}

/*
returns derives the two legs' log returns and the gap between them.
*/
func (op *Basis) returns(m *data.Measurement[float64], state *basisState, last, index float64) {
	var hasDerivReturn, hasRefReturn bool
	var derivLogReturn, refLogReturn float64

	if state.prevLast > 0 && last > 0 {
		derivLogReturn = math.Log(last / state.prevLast)
		m.WriteMetric("derivative_log_return", derivLogReturn)
		hasDerivReturn = true
	}

	if state.prevIndex > 0 && index > 0 {
		refLogReturn = math.Log(index / state.prevIndex)
		m.WriteMetric("reference_log_return", refLogReturn)
		hasRefReturn = true
	}

	if hasDerivReturn && hasRefReturn {
		returnGap := derivLogReturn - refLogReturn
		m.WriteMetric("return_gap", returnGap)

		if state.hasPrevReturnGap {
			m.WriteMetric("return_gap_velocity", returnGap - state.prevReturnGap)
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
