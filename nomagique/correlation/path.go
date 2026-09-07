package correlation

import (
	"maps"
	"slices"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* Path owns timestamp acceptance and the retained observation sequence. */
type Path struct {
	core.PrimitiveError
	retention    core.Primitive
	observations []core.Primitive
	seed         *transport.IO
	current      core.Primitive
}

/*
NewPath retains timestamped observations. Equal timestamps restate the last
observation; regressions return accepted=false without editing the path.
Optional collection-to-collection retention remains caller-configured. Output
records and their observation slices remain immutable across later updates.
*/
func NewPath(retention ...core.Primitive) core.Primitive {
	path := &Path{seed: transport.NewIO(core.From(map[string]core.Primitive{}))}

	if len(retention) > 0 {
		path.retention = transport.NewPipe(retention...)
	}
	return transport.NewMap(path)
}

func (path *Path) Next(input core.Primitive) core.Primitive {
	result := core.Yield(path.seed, input,
		func(_ map[string]core.Primitive, fields map[string]core.Primitive) map[string]core.Primitive {
			output, err := path.Update(fields)
			path.Error(err)
			return output
		}, path)

	if result != nil {
		path.current = result
	}
	return result
}

/*
Update appends outside previously emitted slice lengths. Restatements copy the
slice before replacing a visible observation; retention consumes immutable input.
*/
func (path *Path) Update(fields map[string]core.Primitive) (map[string]core.Primitive, error) {
	decoder := core.NewDecoder(fields)
	at := core.Decode[int64](decoder, "at")

	if _, found := fields["value"]; !found {
		return nil, core.ErrShape
	}

	if err := decoder.Error(); err != nil {
		return nil, err
	}
	priorCount := len(path.observations)
	last := int64(0)

	if priorCount > 0 {
		var err error
		last, err = core.Field[int64](core.To[map[string]core.Primitive](path.observations[priorCount-1]), "at")

		if err != nil {
			return nil, err
		}
	}
	accepted, restated := priorCount == 0 || at >= last, priorCount > 0 && at == last

	if accepted {
		observations := path.observations
		observation := core.From(maps.Clone(fields))

		if restated {
			observations = slices.Clone(observations)
			observations[priorCount-1] = observation
		}

		if !restated {
			observations = append(observations, observation)
		}

		if path.retention != nil {
			var err error
			observations, err = transport.Evaluate[[]core.Primitive](path.retention, core.From(observations))

			if err != nil {
				return nil, err
			}
		}
		path.observations = observations
	}
	output := maps.Clone(fields)
	output["observations"] = core.From(path.observations[:len(path.observations):len(path.observations)])
	output["prior_count"], output["count"] = core.From(float64(priorCount)), core.From(float64(len(path.observations)))
	output["accepted"], output["restated"] = core.From(accepted), core.From(restated)
	output["has_span"] = core.From(len(path.observations) != 0)
	from, through := int64(0), int64(0)

	if len(path.observations) > 0 {
		firstFields := core.To[map[string]core.Primitive](path.observations[0])
		lastFields := core.To[map[string]core.Primitive](path.observations[len(path.observations)-1])
		var err error
		from, err = core.Field[int64](firstFields, "at")

		if err != nil {
			return nil, err
		}
		through, err = core.Field[int64](lastFields, "at")

		if err != nil {
			return nil, err
		}
	}
	output["from"], output["to"] = core.From(from), core.From(through)
	return output, nil
}

func (path *Path) Read() any { return core.To[any](path.current) }
