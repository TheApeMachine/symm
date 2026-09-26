package store

import (
	capnp "capnproto.org/go/capnp/v3"
	"context"
	"github.com/theapemachine/errnie"
)

/* Done publishes the full causal prediction, measured dynamics and resolutions. */
func (server *ResonanceServer) Done(ctx context.Context, call Resonance_done) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(err)
	}
	output := server.out
	if output.Reading != nil {
		reading, err := result.NewReading()
		if err != nil {
			return errnie.Error(err)
		}
		if err := output.Reading.write(reading); err != nil {
			return err
		}
	}
	if err := writeForecast(output.Forecast, result.NewForecast); err != nil {
		return err
	}
	for _, field := range []struct {
		values  []float64
		newList func(int32) (capnp.Float64List, error)
	}{
		{output.ForwardCurve, result.NewForwardCurve}, {output.ForwardRetention, result.NewForwardRetention},
		{output.Readout, result.NewReadout},
	} {
		if err := writeFloats(field.values, field.newList); err != nil {
			return err
		}
	}
	result.SetSupportedHorizon(int64(output.SupportedHorizon))
	result.SetCalibrated(output.Calibrated)
	result.SetResolvedSteps(int64(output.ResolvedSteps))
	result.SetPending(int64(output.Pending))
	result.SetConfidence(output.Confidence)
	if output.Dynamics != nil {
		result.SetAlpha(output.Dynamics.Alpha)
	}
	if output.LastResolution != nil {
		resolution, err := result.NewLastResolution()
		if err != nil {
			return errnie.Error(err)
		}
		resolution.SetPrediction(output.LastResolution.Prediction)
		resolution.SetTarget(output.LastResolution.Target)
		resolution.SetError(output.LastResolution.Error)
		resolution.SetHorizon(int64(output.LastResolution.Horizon))
		resolution.SetStep(output.LastResolution.Step)
	}
	result.SetEpoch(server.epoch)
	result.SetSequence(server.sequence)
	if output.Reading != nil {
		reading := output.Reading
		values := []float64{reading.Energy, reading.PredictionEnergy, reading.ReconstructionError, reading.EnergyDensity, reading.Surprise, reading.TemporalError, server.alpha, float64(server.resolved), float64(len(server.pending)), float64(reading.ReadoutDimension)}
		if err := writeFloats(values, result.NewValues); err != nil {
			return err
		}
		present, err := result.NewPresent(int32(len(values)))
		if err != nil {
			return errnie.Error(err)
		}
		for index := range values {
			present.Set(index, index != 5 || server.observations > 1)
		}
	}
	server.out = resonanceOutput{}
	return nil
}
