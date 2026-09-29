package temporal

import (
	"errors"
	"fmt"
	"iter"
	"math"
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

/*
Next decodes each arriving price path with a fresh LogReturns run and yields
the collected returns with their summed squared values.
*/
func (op *PathReturns) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			path := (*PricePath)(arriving)
			if path == nil || len(path.Prices) < 2 {
				continue
			}

			returns := make([]LogReturn, 0, len(path.Prices)-1)
			energy := 0.0
			prevPrice := path.Prices[0]
			prevLog := math.Log(prevPrice.Value)
			var err error

			for index := 1; index < len(path.Prices); index++ {
				currPrice := path.Prices[index]

				if currPrice.At <= prevPrice.At {
					err = fmt.Errorf("%w: log return time %d must follow %d", core.ErrShape, currPrice.At, prevPrice.At)
					break
				}

				currLog := math.Log(currPrice.Value)
				retVal := currLog - prevLog
				returns = append(returns, LogReturn{
					From:  prevPrice.At,
					To:    currPrice.At,
					Value: retVal,
				})
				energy += retVal * retVal
				prevPrice = currPrice
				prevLog = currLog
			}

			if err != nil {
				op.err = errors.Join(op.err, err)
				return
			}

			out := ReturnPath{
				Returns: returns,
				Energy:  energy,
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
