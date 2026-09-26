package store

import (
	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/learning"
)

/* write preserves every measured manifold field in its existing native schema. */
func (reading resonanceReading) write(target learning.WireManifoldReading) error {
	if err := writeFloats(reading.Reconstruction, target.NewReconstruction); err != nil {
		return err
	}
	target.SetEnergy(reading.Energy)
	target.SetPredictionEnergy(reading.PredictionEnergy)
	target.SetReconstructionError(reading.ReconstructionError)
	target.SetEnergyDensity(reading.EnergyDensity)
	target.SetSurprise(reading.Surprise)
	target.SetTemporalError(reading.TemporalError)
	target.SetHasTemporalError(reading.HasTemporalError)
	target.SetReadoutDimension(int64(reading.ReadoutDimension))
	target.SetSkillAverage(reading.SkillAverage)
	target.SetSkillReadyAvg(reading.SkillReadyAvg)
	target.SetPrecisionAverage(reading.PrecisionAverage)
	target.SetPrecisionReadyAvg(reading.PrecisionReadyAvg)
	target.SetScaleAverage(reading.ScaleAverage)
	target.SetScaleReadyAvg(reading.ScaleReadyAvg)
	for _, field := range []struct {
		values  []float64
		newList func(int32) (capnp.Float64List, error)
	}{
		{reading.Readout, target.NewReadout}, {reading.Latent, target.NewLatent},
		{reading.TaskPrediction, target.NewTaskPrediction}, {reading.Skill, target.NewSkill}, {reading.Retention, target.NewRetention},
	} {
		if err := writeFloats(field.values, field.newList); err != nil {
			return err
		}
	}
	for _, field := range []struct {
		values  []bool
		newList func(int32) (capnp.BitList, error)
	}{
		{reading.SkillReady, target.NewSkillReady}, {reading.PrecisionReady, target.NewPrecisionReady},
	} {
		values, err := field.newList(int32(len(field.values)))
		if err != nil {
			return errnie.Error(err)
		}
		for index, value := range field.values {
			values.Set(index, value)
		}
	}
	layers, err := target.NewLayers(int32(len(reading.Layers)))
	if err != nil {
		return errnie.Error(err)
	}
	for index, layer := range reading.Layers {
		target := layers.At(index)
		target.SetErrorNorm(layer.ErrorNorm)
		target.SetTemporal(layer.Temporal)
		if err := writeFloats(layer.State, target.NewState); err != nil {
			return err
		}
		if err := writeFloats(layer.Prediction, target.NewPrediction); err != nil {
			return err
		}
	}
	return writeForecast(reading.Forecast, target.NewForecast)
}

/* writeFloats copies measured vectors into their typed Cap'n Proto result list. */
func writeFloats(values []float64, newList func(int32) (capnp.Float64List, error)) error {
	target, err := newList(int32(len(values)))
	if err != nil {
		return errnie.Error(err)
	}
	for index, value := range values {
		target.Set(index, value)
	}
	return nil
}

/* writeForecast retains readiness and uncertainty with every horizon. */
func writeForecast(values []resonanceForecast, newList func(int32) (learning.WireRLSOutput_List, error)) error {
	target, err := newList(int32(len(values)))
	if err != nil {
		return errnie.Error(err)
	}
	for index, value := range values {
		reading := target.At(index)
		reading.SetValue(value.Value)
		reading.SetScale(value.Scale)
		reading.SetDegreesOfFreedom(value.DegreesOfFreedom)
		reading.SetReady(value.Ready)
		reading.SetInnovation(value.Innovation)
		reading.SetReset(value.Reset)
	}
	return nil
}
