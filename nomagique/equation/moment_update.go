package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
	"maps"
)

/* MomentUpdate applies the canonical typed recurrence at a named-record boundary. */
type MomentUpdate struct {
	core.PrimitiveError
	seed    *transport.IO
	current core.Primitive
}

func NewMomentUpdate() core.Primitive {
	return &MomentUpdate{seed: transport.NewIO(core.From(map[string]core.Primitive(nil)))}
}

func (update *MomentUpdate) Next(input core.Primitive) core.Primitive {
	result := core.Yield(update.seed, input, func(_ map[string]core.Primitive, fields map[string]core.Primitive) map[string]core.Primitive {
		decoder := core.NewDecoder(fields)
		moments := Moments{
			Count: core.Decode[float64](decoder, "count"), Mean: core.Decode[float64](decoder, "mean"), M2: core.Decode[float64](decoder, "m2"),
		}
		value := core.Decode[float64](decoder, "value")

		if err := decoder.Error(); err != nil {
			update.Error(err)
			return nil
		}
		reading := moments.Update(value)
		output := maps.Clone(fields)
		output["prior_count"], output["prior_mean"], output["prior_m2"] = core.From(reading.Prior.Count), core.From(reading.Prior.Mean), core.From(reading.Prior.M2)
		output["count"], output["mean"], output["m2"] = core.From(reading.Count), core.From(reading.Mean), core.From(reading.M2)
		output["delta"] = core.From(reading.Delta)
		return output
	}, update)

	if result != nil {
		update.current = result
	}
	return result
}
func (update *MomentUpdate) Read() any { return core.To[any](update.current) }
