package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewMomentUpdate is one Welford update over a supplied record. The caller
// supplies count, mean, m2 and value. This graph has no retained statistical
// state; the named algorithm supplies feedback through its configured store.
func NewMomentUpdate() core.Primitive {
	return transport.NewPipe(
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(store.NewGet("count"), store.NewKey("prior_count")),
			transport.NewPipe(store.NewGet("mean"), store.NewKey("prior_mean")),
			transport.NewPipe(store.NewGet("m2"), store.NewKey("prior_m2")),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				NewSum[float64](store.NewGet("count"), store.NewConstant(core.From(1.0))), store.NewKey("count")),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				NewDifference[float64](store.NewGet("value"), store.NewGet("mean")), store.NewKey("delta")),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				NewSum[float64](store.NewGet("mean"), NewRatio[float64](store.NewGet("delta"), store.NewGet("count"))),
				store.NewKey("mean"),
			),
		),
		store.NewRecord(
			transport.NewPipe(),
			transport.NewPipe(
				NewSum[float64](
					store.NewGet("m2"),
					NewProduct[float64](store.NewGet("delta"),
						NewDifference[float64](store.NewGet("value"), store.NewGet("mean"))),
				),
				store.NewKey("m2"),
			),
		),
	)
}
