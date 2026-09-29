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
	err     error
	returns []LogReturn
	out     ReturnPath
}

/*
NewPathReturns creates a new PathReturns primitive.
*/
func NewPathReturns() core.Primitive {
	return &PathReturns{
		returns: make([]LogReturn, 0, 64),
	}
}

/*
Decode decodes prices into log returns and energy directly without iterator overhead.
*/
func (op *PathReturns) Decode(prices []Price) (ReturnPath, error) {
	if len(prices) < 2 {
		return ReturnPath{}, nil
	}

	needCap := len(prices) - 1
	if cap(op.returns) < needCap {
		op.returns = make([]LogReturn, 0, needCap)
	} else {
		op.returns = op.returns[:0]
	}

	energy := 0.0
	prevPrice := prices[0]
	prevLog := math.Log(prevPrice.Value)

	for index := 1; index < len(prices); index++ {
		currPrice := prices[index]

		if currPrice.At <= prevPrice.At {
			err := fmt.Errorf("%w: log return time %d must follow %d", core.ErrShape, currPrice.At, prevPrice.At)
			op.err = errors.Join(op.err, err)
			return ReturnPath{}, err
		}

		currLog := math.Log(currPrice.Value)
		retVal := currLog - prevLog
		op.returns = append(op.returns, LogReturn{
			From:  prevPrice.At,
			To:    currPrice.At,
			Value: retVal,
		})
		energy += retVal * retVal
		prevPrice = currPrice
		prevLog = currLog
	}

	op.out = ReturnPath{
		Returns: op.returns,
		Energy:  energy,
	}

	return op.out, nil
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

			out, err := op.Decode(path.Prices)
			if err != nil {
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
