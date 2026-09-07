package equation

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
	"maps"
)

/* MomentSummary projects Bessel-corrected sample variance at a record boundary. */
type MomentSummary struct {
	core.PrimitiveError
	seed    *transport.IO
	current core.Primitive
}

func NewMomentSummary() core.Primitive {
	return &MomentSummary{seed: transport.NewIO(core.From(map[string]core.Primitive(nil)))}
}
func (summary *MomentSummary) Next(input core.Primitive) core.Primitive {
	result := core.Yield(summary.seed, input, func(_ map[string]core.Primitive, fields map[string]core.Primitive) map[string]core.Primitive {
		decoder := core.NewDecoder(fields)
		moments := Moments{Count: core.Decode[float64](decoder, "count"), M2: core.Decode[float64](decoder, "m2")}

		if err := decoder.Error(); err != nil {
			summary.Error(err)
			return nil
		}
		var reading MomentReading
		reading.Summarize(moments)
		output := maps.Clone(fields)
		output["variance_defined"] = core.From(reading.VarianceDefined)
		output["variance"], output["dispersion"] = core.From(reading.Variance), core.From(reading.Dispersion)
		return output
	}, summary)

	if result != nil {
		summary.current = result
	}
	return result
}
func (summary *MomentSummary) Read() any { return core.To[any](summary.current) }
