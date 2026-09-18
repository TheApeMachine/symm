package morphology

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
Reading is one book-shape observation of the displayed touch.
*/
type Reading struct {
	Distance, KS        float64
	ConcBid, ConcAsk    float64
	EntBid, EntAsk      float64
	Change              float64
	HasChange           bool
	ChangeBase, ChangeZ float64
	HasChangeBase       bool
}

type path struct {
	hasPrev  bool
	prev     float64
	baseline core.Primitive
}

/*
Shape measures folded bid/ask geometry of the current book.
*/
type Shape struct {
	*core.PrimitiveError

	paths map[string]*path
	out   Reading
}

func NewShape() *Shape {
	return &Shape{
		PrimitiveError: core.NewPrimitiveError(),
		paths:          make(map[string]*path),
	}
}

func (shape *Shape) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			quote := *(*Quote)(arriving)
			spread := quote.Ask - quote.Bid
			bidWeight := quote.Bid * quote.BidQty
			askWeight := quote.Ask * quote.AskQty

			if spread <= 0 || bidWeight <= 0 || askWeight <= 0 {
				continue
			}

			state := shape.paths[quote.Symbol]

			if state == nil {
				state = &path{baseline: adaptive.NewBaseline(adaptive.NewWindow())}
				shape.paths[quote.Symbol] = state
			}

			// One displayed level per side, both folded to r = 0.5, so the
			// two shapes occupy the same point and Wasserstein/KS are zero.
			reading := Reading{
				Distance: 0,
				KS:       0,
				ConcBid:  1,
				ConcAsk:  1,
				EntBid:   0,
				EntAsk:   0,
			}

			if state.hasPrev {
				reading.Change = math.Abs(reading.Distance - state.prev)
				reading.HasChange = true

				var baseline adaptive.BaselineReading

				for out := range state.baseline.Next(sequence.NewOne(unsafe.Pointer(&reading.Change)).Next(nil)) {
					baseline = *(*adaptive.BaselineReading)(out)
				}

				if err := state.baseline.Error(); err != nil {
					shape.Error(err)
					return
				}

				if baseline.HasPrior {
					reading.HasChangeBase = true
					reading.ChangeBase = baseline.Baseline
					reading.ChangeZ = baseline.ZScore
				}
			}

			state.prev = reading.Distance
			state.hasPrev = true
			shape.out = reading

			if !yield(unsafe.Pointer(&shape.out)) {
				return
			}
		}
	}
}
