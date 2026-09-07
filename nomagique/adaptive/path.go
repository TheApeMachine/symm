package adaptive

import (
	"slices"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* pathRetention owns the configured mean-shift policy for accepted observations. */
type pathRetention struct {
	core.PrimitiveError
	window  *Window
	seed    *transport.IO
	current core.Primitive
}

/*
NewPath composes timestamp acceptance with adaptive observation retention. The
configured window determines retained support from changes in the observed
values. No wall-clock expiry or fixed history length is imposed. Regressed
observations never reach the policy; restatements are accepted observations.
*/
func NewPath(window *Window) core.Primitive {
	return correlation.NewPath(transport.NewMap(&pathRetention{
		window: window, seed: transport.NewIO(core.From([]core.Primitive{})),
	}))
}

func (retention *pathRetention) Next(input core.Primitive) core.Primitive {
	result := core.Yield(retention.seed, input,
		func(_ []core.Primitive, observations []core.Primitive) []core.Primitive {
			if len(observations) == 0 {
				retention.Error(core.ErrShape)
				return nil
			}
			last := observations[len(observations)-1]
			fields := core.To[map[string]core.Primitive](last)

			if err := last.Error(); err != nil {
				retention.Error(err)
				return nil
			}
			value, err := core.Field[float64](fields, "value")

			if err != nil {
				retention.Error(err)
				return nil
			}
			reading := retention.window.Observe(value)
			start := max(0, len(observations)-int(reading.Capacity))

			if start == 0 {
				return observations
			}
			// Release the discarded prefix when the policy actually sheds support.
			return slices.Clone(observations[start:])
		}, retention)

	if result != nil {
		retention.current = result
	}
	return result
}

func (retention *pathRetention) Read() any { return core.To[any](retention.current) }
