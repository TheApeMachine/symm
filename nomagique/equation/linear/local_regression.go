package linear

import (
	"iter"
	"time"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
LocalRegression uses elapsed seconds from an exact int64 nanosecond origin as
x and the observed value as y. The first origin is retained. The estimator is
cumulative; no silent retention policy is added.
*/
type LocalRegression struct {
	core.Base[equation.Price, Summary]
	moments   *RegressionMoments
	summary   *RegressionSummary
	origin    int64
	hasOrigin bool
}

func NewLocalRegression() *LocalRegression {
	return &LocalRegression{
		moments: NewRegressionMoments(),
		summary: NewRegressionSummary(),
	}
}

func (op *LocalRegression) Next(
	in iter.Seq[core.Primitive[equation.Price, equation.Price]],
) iter.Seq[core.Primitive[Summary, Summary]] {
	return func(yield func(core.Primitive[Summary, Summary]) bool) {
		for arriving := range in {
			price := arriving.Read()

			if !op.hasOrigin {
				op.origin = price.At
				op.hasOrigin = true
			}

			point := Point{
				X: float64(price.At-op.origin) / float64(time.Second),
				Y: price.Value,
			}

			var moments Moments

			for state := range op.moments.Next(transport.Values(point)) {
				moments = state.Read()
			}

			if err := op.moments.Error(); err != nil {
				op.Error(err)
				return
			}

			for summary := range op.summary.Next(transport.Values(moments)) {
				if !yield(op.Carrier(summary.Read())) {
					return
				}
			}
		}
	}
}
