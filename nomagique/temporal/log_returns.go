package temporal

import (
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
LogReturn is one adjacent log-price difference on an open-left time interval.
*/
type LogReturn struct {
	From  int64
	To    int64
	Value float64
}

/*
LogReturns computes adjacent log differences of sequential price points.
*/
type LogReturns struct {
	err     error
	out     LogReturn
	seen    bool
	through int64
	prevLog float64
}

func NewLogReturns() core.Primitive {
	return &LogReturns{}
}

func (op *LogReturns) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			price := (*Price)(arriving)
			logVal := math.Log(price.Value)

			if !op.seen {
				op.seen = true
				op.through = price.At
				op.prevLog = logVal
				continue
			}

			if price.At <= op.through {
				op.err = fmt.Errorf("%w: log return time %d must follow %d", core.ErrShape, price.At, op.through)
				return
			}

			op.out = LogReturn{
				From:  op.through,
				To:    price.At,
				Value: logVal - op.prevLog,
			}
			op.through = price.At
			op.prevLog = logVal

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *LogReturns) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
