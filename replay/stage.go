package replay

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type Stage struct {
	*runtime.System
	client broker.Transport
}

func NewStage(ctx context.Context, name string, client broker.Transport) *Stage {
	stage := &Stage{
		client: client,
	}

	stage.System = runtime.NewSystem(ctx, name)
	return stage
}

func (stage *Stage) Step(prior *data.Measurement) *data.Measurement {
	var keys []string

	if prior != nil && prior.Label != "" {
		keys = []string{stage.Name(), prior.Label}
	} else {
		keys = []string{stage.Name()}
	}

	buf, err := stage.client.Read(keys...)

	if err != nil {
		return nil
	}

	measurement := &data.Measurement{}

	if err := measurement.UnmarshalBinary(buf); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation, "[replay.stage] failed to unmarshal binary", err,
		))

		return nil
	}

	return measurement
}
