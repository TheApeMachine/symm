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
	*core.PrimitiveError

	out     LogReturn
	seen    bool
	through int64
	prevLog float64
}

func NewLogReturns() *LogReturns {
	return &LogReturns{PrimitiveError: core.NewPrimitiveError()}
}

func (logReturns *LogReturns) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			price := (*Price)(arriving)
			logVal := math.Log(price.Value)

			if !logReturns.seen {
				logReturns.seen = true
				logReturns.through = price.At
				logReturns.prevLog = logVal
				continue
			}

			if price.At <= logReturns.through {
				logReturns.Error(fmt.Errorf("%w: log return time %d must follow %d", core.ErrShape, price.At, logReturns.through))
				return
			}

			logReturns.out = LogReturn{
				From:  logReturns.through,
				To:    price.At,
				Value: logVal - logReturns.prevLog,
			}
			logReturns.through = price.At
			logReturns.prevLog = logVal

			if !yield(unsafe.Pointer(&logReturns.out)) {
				return
			}
		}
	}
}
