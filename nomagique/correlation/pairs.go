package correlation

import (
	"errors"
	"iter"
	"math"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Relation is the measured pair before cohort folding. Support counts overlapping
return pairs, not independent samples. EffectiveSupport and Authority are explicitly unavailable (nil):
the Hayashi estimator does not estimate independence-adjusted sample size.
FisherDefined and PValue retain the existing independent-return approximation;
consumers must not mistake that approximation for calibrated authority.
At is the older endpoint of the two paths, so stale peers remain visible.
*/
type Relation struct {
	EffectiveSupport, Authority *float64
	Left, Right                 string
	Signed, Absolute, Support   float64
	PValue, StandardError       float64
	Defined, FisherDefined      bool
	At                          time.Time
}

/*
Relations retains measured pair measurements. It stores data and answers
nothing else; consumers query it like any other store.
*/
type Relations struct {
	err   error
	pairs map[[2]string]Relation
}

func NewRelations() core.Primitive {
	return &Relations{}
}

/*
Next receives *Relation payloads and retains each under its ordered symbol
pair.
*/
func (op *Relations) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			relation := (*Relation)(arriving)

			if op.pairs == nil {
				op.pairs = make(map[[2]string]Relation)
			}

			op.pairs[[2]string{relation.Left, relation.Right}] = *relation

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *Relations) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
Pairs measures the focal path against every peer: one dependence estimate and
one Fisher significance per pair, every measured pair retained, and every
defined pair with support admitted to the cohort. When several peers
qualify, the lexicographically last one is the selected pair, so selection is
deterministic.
*/
type Pairs struct {
	err       error
	pairwise  core.Primitive
	fisher    core.Primitive
	relations core.Primitive
}

/*
NewPairs composes the pair stage over the supplied causal estimator, so the
algo dependency is injected at composition instead of imported here.
*/
func NewPairs(estimator core.Primitive) core.Primitive {
	return &Pairs{
		pairwise:  NewDependence(estimator),
		fisher:    NewFisher(),
		relations: NewRelations(),
	}
}

func (op *Pairs) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			reading := (*Reading)(arriving)

			if reading.State != StateTraded || !reading.Focal.Accepted {
				if !yield(arriving) {
					return
				}

				continue
			}

			for _, peer := range reading.Peers {
				pair := measure(op, reading, peer)
				retain(op, reading, peer, pair)

				if !pair.Dependence.Defined || pair.Dependence.Support < 2 {
					continue
				}

				reading.Admitted = append(reading.Admitted, Peer{
					Correlation: pair.Dependence.Correlation,
					Support:     pair.Dependence.Support,
					PeerEnergy:  pair.Dependence.RightEnergyRate,
				})
				reading.Selected = pair
				reading.SelectedSymbol = peer.Symbol
			}

			if !yield(arriving) {
				return
			}
		}
	}
}

/*
measure runs one dependence estimate and its Fisher significance.
*/
func measure(op *Pairs, reading *Reading, peer PeerReading) PairReading {
	input := LagProfileInput{Left: reading.Focal.Observations, Right: peer.Reading.Observations}
	dependence := drive[LagProfileInput, DependenceReading](op.pairwise, &input)

	sample := FisherSample{Correlation: dependence.Correlation, Support: dependence.Support}
	fisher := drive[FisherSample, FisherReading](op.fisher, &sample)

	return PairReading{Symbol: peer.Symbol, Dependence: dependence, Fisher: fisher}
}

/*
retain records one measured pair, whether or not it is defined. Undefined
Fisher fields remain explicitly unavailable; NaN is not serialized as though
it were a p-value.
*/
func retain(op *Pairs, reading *Reading, peer PeerReading, pair PairReading) {
	left, right := reading.Symbol, peer.Symbol

	if right < left {
		left, right = right, left
	}

	relation := Relation{
		Left:     left,
		Right:    right,
		Support:  pair.Dependence.Support,
		Defined:  pair.Dependence.Defined,
		At:       time.Unix(0, min(reading.Price.At, peer.Reading.To)),
	}

	if relation.Defined {
		relation.Signed = pair.Dependence.Correlation
		relation.Absolute = math.Abs(relation.Signed)
	}

	relation.FisherDefined = pair.Fisher.Defined

	if relation.FisherDefined {
		relation.PValue = pair.Fisher.PValue
		relation.StandardError = pair.Fisher.StandardError
	}

	for range op.relations.Next(transport.NewOne(unsafe.Pointer(&relation)).Next(nil)) {
	}
}

func (op *Pairs) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
