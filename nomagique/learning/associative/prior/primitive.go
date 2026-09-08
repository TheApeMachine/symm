package prior

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* Primitive binds configured storage to the canonical numeric prior recurrence. */
type Primitive struct {
	core.PrimitiveError
	memorySize, memory core.Primitive
	seed               *transport.IO
	current            core.Primitive
}

/* NewMemory supplies only the sufficient statistics, without transition scratch. */
func NewMemory() *store.Retained {
	return store.NewRetained(core.From(priorFields(equation.PriorMoments{})))
}

/*
New consumes optional value/authority and epoch fields. Configured storage
remains caller-owned; queries age evidence without replaying the previous value.
The Model calls the same PriorMoments recurrence directly over its typed state.
*/
func New(memorySize, memory core.Primitive) core.Primitive {
	return transport.NewMap(&Primitive{
		memorySize: memorySize, memory: memory,
		seed: transport.NewIO(core.From(map[string]core.Primitive{})),
	})
}

func (prior *Primitive) Next(input core.Primitive) core.Primitive {
	result := core.Yield(prior.seed, input,
		func(_ map[string]core.Primitive, fields map[string]core.Primitive) map[string]core.Primitive {
			output, err := prior.Transition(fields)
			prior.Error(err)
			return output
		}, prior)

	if result != nil {
		prior.current = result
	}
	return result
}

/* Transition reads configuration once and commits only sufficient statistics. */
func (prior *Primitive) Transition(fields map[string]core.Primitive) (map[string]core.Primitive, error) {
	memory, err := transport.Evaluate[float64](prior.memorySize, core.From(fields))

	if err != nil {
		return nil, err
	}
	retained, err := transport.Evaluate[map[string]core.Primitive](prior.memory, nil)

	if err != nil {
		return nil, err
	}
	decoder := core.NewDecoder(retained)
	moments := equation.PriorMoments{
		Samples: core.Decode[uint64](decoder, "samples"), Pending: core.Decode[uint64](decoder, "pending"),
		LastEpoch: core.Decode[uint64](decoder, "last_epoch"), Mean: core.Decode[float64](decoder, "mean"),
		Weight: core.Decode[float64](decoder, "weight"), Support: core.Decode[float64](decoder, "support"),
		Moment: core.Decode[float64](decoder, "moment"),
	}

	if err := decoder.Error(); err != nil {
		return nil, err
	}

	if err := priorObservation(&moments, fields, memory); err != nil {
		return nil, err
	}
	sufficient := priorFields(moments)

	if _, err := transport.Evaluate[map[string]core.Primitive](prior.memory, core.From(sufficient)); err != nil {
		return nil, err
	}
	reading := moments.Summary(memory)
	output := priorFields(moments)
	output["defined"], output["variance_defined"] = core.From(reading.Defined), core.From(reading.VarianceDefined)
	output["mean"], output["support"] = core.From(reading.Mean), core.From(reading.Support)
	output["variance"], output["maturity"] = core.From(reading.Variance), core.From(reading.Maturity)
	output["evidence_authority"], output["authority"] = core.From(reading.EvidenceAuthority), core.From(reading.Authority)
	output["memory"] = core.From(memory)
	return output, nil
}

/* priorObservation translates the named input protocol without retaining input fields. */
func priorObservation(moments *equation.PriorMoments, fields map[string]core.Primitive, memory float64) error {
	decoder := core.NewDecoder(fields)
	var epoch []uint64

	if _, supplied := fields["epoch"]; supplied {
		epoch = []uint64{core.Decode[uint64](decoder, "epoch")}
	}

	if _, observed := fields["value"]; observed {
		value, authority := core.Decode[float64](decoder, "value"), core.Decode[float64](decoder, "authority")

		if err := decoder.Error(); err != nil {
			return err
		}
		return moments.Observe(value, authority, memory, epoch...)
	}

	if err := decoder.Error(); err != nil {
		return err
	}

	if len(epoch) != 0 {
		moments.Age(epoch[0], memory)
	}
	return nil
}

/* priorFields translates sufficient statistics at the generic storage boundary. */
func priorFields(moments equation.PriorMoments) map[string]core.Primitive {
	return map[string]core.Primitive{
		"samples": core.From(moments.Samples), "pending": core.From(moments.Pending),
		"last_epoch": core.From(moments.LastEpoch), "mean": core.From(moments.Mean),
		"weight": core.From(moments.Weight), "support": core.From(moments.Support), "moment": core.From(moments.Moment),
	}
}

func (prior *Primitive) Read() any { return core.To[any](prior.current) }
