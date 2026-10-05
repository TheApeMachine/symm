package temporal

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
LogReturns computes adjacent log differences of sequential price points.
*/
type LogReturns struct {
	*core.PrimitiveError
	seen    bool
	through float64
	prevLog float64
	input   data.Map[string]
	output  data.Map[float64]
}

func NewLogReturns() *LogReturns {
	output := data.NewOutputMap()
	output.Values["return"] = 0
	output.Values["from"] = 0
	output.Values["to"] = 0

	return &LogReturns{
		PrimitiveError: core.NewPrimitiveError(),
		input: data.NewMap(
			"price", "price",
			"at", "at",
		),
		output: output,
	}
}

func (op *LogReturns) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			price, priceOK := values.Values["price"]
			at, atOK := values.Values["at"]

			if !priceOK || !atOK {
				op.Error(core.ErrNotHeld)
				return
			}

			if price <= 0 {
				op.Error(core.ErrDomain)
				return
			}

			logVal := math.Log(price)

			if !op.seen {
				op.seen = true
				op.through = at
				op.prevLog = logVal
				op.output.Values["return"] = 0
				op.output.Values["from"] = at
				op.output.Values["to"] = at

				for range adapter.Next(data.NewValue(op.output)) {
				}

				if err := adapter.Error(); err != nil {
					op.Error(err)
					return
				}

				if !yield(arriving) {
					return
				}

				continue
			}

			if at <= op.through {
				op.Error(core.ErrShape)
				return
			}

			from := op.through
			op.through = at
			retVal := logVal - op.prevLog
			op.prevLog = logVal

			op.output.Values["return"] = retVal
			op.output.Values["from"] = from
			op.output.Values["to"] = at

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
