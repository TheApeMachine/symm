package temporal

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
)

/*
PricePath is one decoded price path.
*/
type PricePath struct {
	Prices []Price
}

/*
ReturnPath is the log returns of one price path and their cumulative energy.
*/
type ReturnPath struct {
	Returns []LogReturn
	Energy  float64
}

/*
PathReturns owns decoding one arriving price path into its returns and energy
by composing LogReturns over the path's arrivals.
*/
type PathReturns struct {
	*core.PrimitiveError
}

/*
NewPathReturns creates a new PathReturns primitive.
*/
func NewPathReturns() *PathReturns {
	return &PathReturns{PrimitiveError: core.NewPrimitiveError()}
}

/*
Next decodes each arriving price path with a fresh LogReturns run and yields
the collected returns with their summed squared values.
*/
func (pathReturns *PathReturns) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			path := (*PricePath)(arriving)
			decoder := NewLogReturns()
			out := ReturnPath{}

			for returnPtr := range decoder.Next(sequence.NewValues(path.Prices...).Next(nil)) {
				value := *(*LogReturn)(returnPtr)
				out.Returns = append(out.Returns, value)
				out.Energy += value.Value * value.Value
			}

			if err := decoder.Error(); err != nil {
				pathReturns.Error(err)
				return
			}

			if !yield(unsafe.Pointer(&out)) {
				return
			}
		}
	}
}
