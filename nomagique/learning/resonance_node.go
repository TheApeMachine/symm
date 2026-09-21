package learning

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
Write decodes a wire command and executes it against the manifold. The schema
declares one command carrying whichever action the caller set, so the decode
selects on which action is present.
*/
func (m *ResonanceManifoldServer) Write(
	ctx context.Context, call ResonanceManifold_write,
) error {
	wire, err := call.Args().Command()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[learning.resonance.Write] failed to read command argument",
			err,
		))
	}

	command, err := decodeManifoldCommand(wire)

	if err != nil {
		return err
	}

	if _, err := m.Execute(command); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[learning.resonance.Write] manifold command failed",
			err,
		))
	}

	return nil
}

/*
Done completes the observation. The manifold's reading is obtained by
executing a reading command, so there is nothing to collect here.
*/
func (m *ResonanceManifoldServer) Done(
	ctx context.Context, call ResonanceManifold_done,
) error {
	return nil
}

/*
decodeManifoldCommand reads whichever action the wire command carries.
*/
func decodeManifoldCommand(wire WireManifoldCommand) (ManifoldCommand, error) {
	var command ManifoldCommand

	if wire.HasSettle() {
		settle, err := wire.Settle()

		if err != nil {
			return command, errnie.Error(errnie.Err(
				errnie.Validation, "[learning.resonance] failed to read settle action", err,
			))
		}

		features, err := floatsOf(settle.Features())

		if err != nil {
			return command, err
		}

		target, err := floatsOf(settle.Target())

		if err != nil {
			return command, err
		}

		command.Settle = &SettleIntent{
			Features:        features,
			Target:          target,
			AdvanceTemporal: settle.AdvanceTemporal(),
		}

		return command, nil
	}

	if wire.HasBatch() {
		batch, err := wire.Batch()

		if err != nil {
			return command, errnie.Error(errnie.Err(
				errnie.Validation, "[learning.resonance] failed to read batch action", err,
			))
		}

		input, err := floatsOf(batch.Input())

		if err != nil {
			return command, err
		}

		command.Batch = &BatchIntent{
			Input:           input,
			Learn:           batch.Learn(),
			AdvanceTemporal: batch.AdvanceTemporal(),
		}

		return command, nil
	}

	if wire.HasObserveTask() {
		task, err := wire.ObserveTask()

		if err != nil {
			return command, errnie.Error(errnie.Err(
				errnie.Validation, "[learning.resonance] failed to read task action", err,
			))
		}

		features, err := floatsOf(task.Features())

		if err != nil {
			return command, err
		}

		command.ObserveTask = &TaskIntent{
			Horizon:    int(task.Horizon()),
			Features:   features,
			Prediction: task.Prediction(),
			Target:     task.Target(),
		}

		return command, nil
	}

	if wire.HasForecast() {
		forecast, err := wire.Forecast()

		if err != nil {
			return command, errnie.Error(errnie.Err(
				errnie.Validation, "[learning.resonance] failed to read forecast action", err,
			))
		}

		command.Forecast = &ForecastIntent{Steps: int(forecast.Steps())}
		return command, nil
	}

	if wire.HasRetention() {
		retention, err := wire.Retention()

		if err != nil {
			return command, errnie.Error(errnie.Err(
				errnie.Validation, "[learning.resonance] failed to read retention action", err,
			))
		}

		command.Retention = &RetentionIntent{Steps: int(retention.Steps())}
		return command, nil
	}

	if wire.HasAlpha() {
		alpha, err := wire.Alpha()

		if err != nil {
			return command, errnie.Error(errnie.Err(
				errnie.Validation, "[learning.resonance] failed to read alpha action", err,
			))
		}

		command.Alpha = &AlphaIntent{Alpha: alpha.Alpha()}
		return command, nil
	}

	if wire.HasReading() {
		command.Reading = &ReadingIntent{}
		return command, nil
	}

	return command, errnie.Error(errnie.Err(
		errnie.Validation,
		"[learning.resonance] command carries no action",
		nil,
	))
}

/*
floatsOf reads a wire float list into a slice.
*/
func floatsOf(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "[learning.resonance] failed to read float list", err,
		))
	}

	if list.Len() == 0 {
		return nil, nil
	}

	values := make([]float64, list.Len())

	for index := range values {
		values[index] = list.At(index)
	}

	return values, nil
}
