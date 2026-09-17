package correlation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
PriceObservation pairs an identified symbol with its timestamped price.
*/
type PriceObservation struct {
	Symbol string
	temporal.Price
}

/*
PairsReading is the structured fact yielded by one explicit oriented pair.
*/
type PairsReading struct {
	Observation PriceObservation
	Path        PathReading
	Selected    DependenceReading
	Fisher      FisherReading
	PeerSymbol  string
	Peers       []Peer
}

/*
Pairs owns one explicit oriented pair: measured Y against reference X.
It does not scan undeclared symbols.
*/
type Pairs struct {
	*core.PrimitiveError

	measured  string
	reference string
	left      core.Primitive
	right     core.Primitive
	heldLeft  PathReading
	heldRight PathReading
	hasLeft   bool
	hasRight  bool
	pairwise  core.Primitive
	fisher    core.Primitive
}

func NewPairs(estimator core.Primitive, measured, reference string) *Pairs {
	return &Pairs{
		PrimitiveError: core.NewPrimitiveError(),
		measured:       measured,
		reference:      reference,
		left:           NewPath(adaptive.NewWindow()),
		right:          NewPath(adaptive.NewWindow()),
		pairwise:       NewDependence(estimator),
		fisher:         NewFisher(),
	}
}

func (pairs *Pairs) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			observation := *(*PriceObservation)(arriving)

			if observation.Value <= 0 {
				continue
			}

			if observation.Symbol != pairs.measured && observation.Symbol != pairs.reference {
				continue
			}

			path := pairs.left
			held := &pairs.heldLeft
			has := &pairs.hasLeft

			if observation.Symbol == pairs.reference {
				path = pairs.right
				held = &pairs.heldRight
				has = &pairs.hasRight
			}

			price := observation.Price
			var focal PathReading

			for out := range path.Next(sequence.NewOne(unsafe.Pointer(&price)).Next(nil)) {
				focal = *(*PathReading)(out)
			}

			if err := path.Error(); err != nil {
				pairs.Error(err)
				return
			}

			reading := PairsReading{
				Observation: observation,
				Path:        focal,
				PeerSymbol:  pairs.reference,
			}

			if observation.Symbol == pairs.reference {
				reading.PeerSymbol = pairs.measured
			}

			if !focal.Accepted {
				if !yield(unsafe.Pointer(&reading)) {
					return
				}

				continue
			}

			*held = focal
			*has = true

			if !pairs.hasLeft || !pairs.hasRight {
				if !yield(unsafe.Pointer(&reading)) {
					return
				}

				continue
			}

			input := LagProfileInput{
				Left:  pairs.heldLeft.Observations,
				Right: pairs.heldRight.Observations,
			}
			var dependence DependenceReading

			for out := range pairs.pairwise.Next(sequence.NewOne(unsafe.Pointer(&input)).Next(nil)) {
				dependence = *(*DependenceReading)(out)
			}

			if err := pairs.pairwise.Error(); err != nil {
				pairs.Error(err)
				return
			}

			reading.Selected = dependence

			if dependence.Defined {
				reading.Peers = []Peer{{
					Correlation: dependence.Correlation,
					Support:     dependence.Support,
					PeerEnergy:  dependence.RightEnergyRate,
				}}
			}

			sample := FisherSample{Correlation: dependence.Correlation, Support: dependence.Support}
			var significance FisherReading

			for out := range pairs.fisher.Next(sequence.NewOne(unsafe.Pointer(&sample)).Next(nil)) {
				significance = *(*FisherReading)(out)
			}

			if err := pairs.fisher.Error(); err != nil {
				pairs.Error(err)
				return
			}

			reading.Fisher = significance

			if !yield(unsafe.Pointer(&reading)) {
				return
			}
		}
	}
}
