package derivatives

import (
	"errors"
	"iter"
	"math"
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

	if m.Metadata == nil {
		m.Metadata = make(map[string]float64)
	}

	last := m.Metrics["last"].Raw
	index := m.Metrics["index_price"].Raw
	mark := m.Metrics["mark_price"].Raw
	oi := m.Metrics["open_interest"].Raw

	basis := (last - index) / index
	basisReading := drive[float64, adaptive.BaselineReading](state.basis, &basis)

	m.From = stamped

	m.Metrics["derivative_price"] = m.Metrics["derivative_price"].Write(last)
	m.Metrics["reference_price"] = m.Metrics["reference_price"].Write(index)
	m.Metrics["spot_price"] = m.Metrics["spot_price"].Write(mark)
	m.Metrics["open_interest"] = m.Metrics["open_interest"].Write(oi)
	m.Metrics["basis"] = m.Metrics["basis"].Write(basis)
	m.Metrics["basis_baseline"] = m.Metrics["basis_baseline"].Write(basisReading.Baseline)

	if basisReading.HasPrior {
		m.Metrics["basis_zscore"] = m.Metrics["basis_zscore"].Write(basisReading.ZScore)
	}

	if last > 0 && index > 0 {
		m.Metrics["log_basis"] = m.Metrics["log_basis"].Write(math.Log(last / index))

		if mark > 0 {
			m.Metrics["derivative_index_log_basis"] = m.Metrics["derivative_index_log_basis"].Write(math.Log(last / index))
			m.Metrics["index_spot_log_basis"] = m.Metrics["index_spot_log_basis"].Write(math.Log(index / mark))
			m.Metrics["derivative_spot_log_basis"] = m.Metrics["derivative_spot_log_basis"].Write(math.Log(last / mark))
			m.Metrics["basis_closure_error"] = m.Metrics["basis_closure_error"].Write(0.0)
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
		m.Metadata[data.MetadataSupport] = basisReading.Prior.Count

		// basis_zscore is this entity's headline reading, so its estimator
		// supplies the departure and the noise power Finalize turns into SNR.
		if basisReading.PriorVariance > 0 {
			m.Metadata[data.MetadataDivergence] = basisReading.Residual
			m.Metadata[data.MetadataNoiseVariance] = basisReading.PriorVariance
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
	m.Metrics["open_interest_change"] = m.Metrics["open_interest_change"].Write(oiChange)

	if state.prevOI > 0 && oi > 0 {
		oiLogChange := math.Log(oi / state.prevOI)
		m.Metrics["open_interest_log_change"] = m.Metrics["open_interest_log_change"].Write(oiLogChange)

		if dt > 0 {
			oiGrowthRate := oiLogChange / dt
			m.Metrics["open_interest_growth_rate"] = m.Metrics["open_interest_growth_rate"].Write(oiGrowthRate)

			growth := drive[float64, adaptive.BaselineReading](state.growth, &oiGrowthRate)
			m.Metrics["open_interest_growth_baseline"] = m.Metrics["open_interest_growth_baseline"].Write(growth.Baseline)
		}
	}

	if dt > 0 {
		basisChange := basis - state.prevBasis
		basisRate := basisChange / dt
		m.Metrics["basis_change"] = m.Metrics["basis_change"].Write(basisChange)
		m.Metrics["basis_rate"] = m.Metrics["basis_rate"].Write(basisRate)

		// Velocity is the change in the RATE between consecutive
		// observations: basis_rate says how fast the basis is moving,
		// basis_velocity says whether that movement is accelerating.
		if state.hasPrevBasisRate {
			m.Metrics["basis_velocity"] = m.Metrics["basis_velocity"].Write(basisRate - state.prevBasisRate)
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
		m.Metrics["derivative_log_return"] = m.Metrics["derivative_log_return"].Write(derivLogReturn)
		hasDerivReturn = true
	}

	if state.prevIndex > 0 && index > 0 {
		refLogReturn = math.Log(index / state.prevIndex)
		m.Metrics["reference_log_return"] = m.Metrics["reference_log_return"].Write(refLogReturn)
		hasRefReturn = true
	}

	if hasDerivReturn && hasRefReturn {
		returnGap := derivLogReturn - refLogReturn
		m.Metrics["return_gap"] = m.Metrics["return_gap"].Write(returnGap)

		if state.hasPrevReturnGap {
			m.Metrics["return_gap_velocity"] = m.Metrics["return_gap_velocity"].Write(returnGap - state.prevReturnGap)
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
