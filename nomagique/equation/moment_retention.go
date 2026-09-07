package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* MomentRetention changes sample mass while preserving corrected dispersion. */
type MomentRetention struct {
	core.PrimitiveError
	seed    *transport.IO
	current core.Primitive
}

func NewMomentRetention() core.Primitive {
	return &MomentRetention{seed: transport.NewIO(core.From(map[string]core.Primitive(nil)))}
}
func (retention *MomentRetention) Next(input core.Primitive) core.Primitive {
	result := core.Yield(retention.seed, input, func(_ map[string]core.Primitive, fields map[string]core.Primitive) map[string]core.Primitive {
		decoder := core.NewDecoder(fields)
		moments := Moments{Count: core.Decode[float64](decoder, "count"), Mean: core.Decode[float64](decoder, "mean"), M2: core.Decode[float64](decoder, "m2")}
		retain := core.Decode[float64](decoder, "retain")

		if err := decoder.Error(); err != nil {
			retention.Error(err)
			return nil
		}
		moments.Retain(retain)
		return map[string]core.Primitive{
			"count": core.From(moments.Count), "mean": core.From(moments.Mean), "m2": core.From(moments.M2),
		}
	}, retention)

	if result != nil {
		retention.current = result
	}
	return result
}
func (retention *MomentRetention) Read() any { return core.To[any](retention.current) }
