package temporal

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
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
	err error
}

/*
NewPathReturns creates a new PathReturns primitive.
*/
func NewPathReturns() core.Primitive {
	return &PathReturns{}
}

func priceSequence(prices []Price) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for index := range prices {
			if !yield(unsafe.Pointer(&prices[index])) {
				return
			}
		}
	}
}

/*
Next decodes each arriving price path with a fresh LogReturns run and yields
the collected returns with their summed squared values.
*/
func (op *PathReturns) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			path := (*PricePath)(arriving)
			var decoder LogReturns
			out := ReturnPath{
				Returns: make([]LogReturn, 0, max(len(path.Prices)-1, 0)),
			}

			for returnPtr := range decoder.Next(priceSequence(path.Prices)) {
				value := *(*LogReturn)(returnPtr)
				out.Returns = append(out.Returns, value)
				out.Energy += value.Value * value.Value
			}

			if err := decoder.Error(); err != nil {
				op.err = errors.Join(op.err, err)
				return
			}

			if !yield(unsafe.Pointer(&out)) {
				return
			}
		}
	}
}

/*
Error joins every error it observes.
*/
func (op *PathReturns) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
