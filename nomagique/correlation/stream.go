package correlation

import (
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/logic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
State is what one arrival means to the correlation stream.
*/
type State uint8

const (
	// StateNeedsPrice: the measurement carries no last price at all.
	StateNeedsPrice State = iota
	// StateInvalid: the last price is not a finite non-negative number.
	StateInvalid
	// StateUnobserved: the market is quoted but no trade printed.
	StateUnobserved
	// StateRegressed: the observation's event time went backwards.
	StateRegressed
	// StateTraded: the observation advances the focal path.
	StateTraded
)

/*
PeerReading is one other symbol's most recent accepted path reading.
*/
type PeerReading struct {
	Symbol  string
	Reading PathReading
}

/*
PairReading is one focal/peer pair's dependence and Fisher significance.
*/
type PairReading struct {
	Symbol     string
	Dependence DependenceReading
	Fisher     FisherReading
}

/*
Reading is the one payload a correlation arrival flows through. Every stage
of the stream enriches it in place: the gate classifies it, the focal stage
advances the price path, the pair stage measures peers, the reductions fold
them, and the shape stage writes the measurement.
*/
type Reading struct {
	Measurement        *data.Measurement[float64]
	State              State
	Symbol             string
	At                 int64
	Price              temporal.Price
	Focal              PathReading
	Peers              []PeerReading
	Admitted           []Peer
	Selected           PairReading
	SelectedSymbol     string
	Cohort             CohortSummary
	Relative           float64
	History            FisherView
	RelativeHistory    adaptive.BaselineReading
	CorrelationVelocity temporal.VelocityReading
	EnergyVelocity      temporal.VelocityReading
}

/*
drive pushes one payload pointer through one primitive and returns the
answer the primitive yielded.
*/
func drive[From, To any](op core.Primitive, payload *From) To {
	var answer To

	for out := range op.Next(transport.NewOne(unsafe.Pointer(payload)).Next(nil)) {
		answer = *(*To)(out)
	}

	return answer
}

/*
Gate classifies the arrival: it reads the last price the feed wrote, consumes
that metric, and stamps the measurement's support baseline. Anything invalid
fails the measurement here and never reaches the path.
*/
type Gate struct {
	err    error
	finite core.Primitive
}

func NewGate() core.Primitive {
	return &Gate{finite: logic.NewFinite()}
}

func (op *Gate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			reading := (*Reading)(arriving)
			m := reading.Measurement

			metric, traded := m.Metrics["last_price"]

			if !traded {
				reading.State = StateNeedsPrice
				m.Err = fmt.Errorf("%w: correlation: ticker requires a last price", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			last := metric.Raw
			m.Metadata = map[string]float64{data.MetadataSupport: 0}

			if holds := drive[float64, bool](op.finite, &last); !holds || last < 0 {
				reading.State = StateInvalid
				m.Err = fmt.Errorf("%w: correlation: finite non-negative last price required", core.ErrDomain)

				if !yield(arriving) {
					return
				}

				continue
			}

			reading.Symbol = m.Label
			reading.At = m.At.UnixNano()
			reading.Price = temporal.Price{At: reading.At, Value: last}

			if last == 0 {
				reading.State = StateUnobserved
			} else {
				reading.State = StateTraded
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Gate) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
