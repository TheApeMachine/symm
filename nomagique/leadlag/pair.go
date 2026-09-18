package leadlag

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	nmcorrelation "github.com/theapemachine/symm/nomagique/statistic/correlation"
)

/*
Pair owns one explicit oriented lead-lag pair.
*/
type Pair struct {
	*core.PrimitiveError

	measured  string
	reference string
	left      core.Primitive
	right     core.Primitive
	heldLeft  nmcorrelation.PathReading
	heldRight nmcorrelation.PathReading
	hasLeft   bool
	hasRight  bool
	search    core.Primitive
}

func NewPair(measured, reference string) *Pair {
	return &Pair{
		PrimitiveError: core.NewPrimitiveError(),
		measured:       measured,
		reference:      reference,
		left:           nmcorrelation.NewPath(adaptive.NewWindow()),
		right:          nmcorrelation.NewPath(adaptive.NewWindow()),
		search:         nmcorrelation.NewLeadLag(algo.NewHayashiYoshida()),
	}
}

func (pair *Pair) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			observation := *(*nmcorrelation.PriceObservation)(arriving)

			if observation.Value <= 0 {
				continue
			}

			if observation.Symbol != pair.measured && observation.Symbol != pair.reference {
				continue
			}

			path := pair.left
			held := &pair.heldLeft
			has := &pair.hasLeft

			if observation.Symbol == pair.reference {
				path = pair.right
				held = &pair.heldRight
				has = &pair.hasRight
			}

			price := observation.Price
			var focal nmcorrelation.PathReading

			for out := range path.Next(sequence.NewOne(unsafe.Pointer(&price)).Next(nil)) {
				focal = *(*nmcorrelation.PathReading)(out)
			}

			if err := path.Error(); err != nil {
				pair.Error(err)
				return
			}

			if !focal.Accepted {
				continue
			}

			*held = focal
			*has = true

			if !pair.hasLeft || !pair.hasRight {
				continue
			}

			input := nmcorrelation.LagProfileInput{
				Left:  pair.heldLeft.Observations,
				Right: pair.heldRight.Observations,
			}
			var reading nmcorrelation.LeadLagReading

			for out := range pair.search.Next(sequence.NewOne(unsafe.Pointer(&input)).Next(nil)) {
				reading = *(*nmcorrelation.LeadLagReading)(out)
			}

			if err := pair.search.Error(); err != nil {
				pair.Error(err)
				return
			}

			if !yield(unsafe.Pointer(&reading)) {
				return
			}
		}
	}
}
