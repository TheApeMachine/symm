package types

import (
	telemetry "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
EncodeWire sends the settled latent pair for the universe scatter. Only focused
readings include the full hierarchy, forecast and diagnostics.
*/
func (artifact *ResonanceArtifact) EncodeWire(focused bool) *telemetry.ResonanceT {
	if artifact == nil {
		return nil
	}

	var embedding []float64

	if artifact.Snapshot != nil && len(artifact.Snapshot.Latent) >= 2 {
		embedding = append([]float64(nil), artifact.Snapshot.Latent[:2]...)
	}

	if !focused {
		return &telemetry.ResonanceT{
			Symbol: artifact.Symbol, At: artifact.At.UnixNano(), Embedding: embedding,
		}
	}

	wire := &telemetry.ResonanceT{
		Symbol:                   artifact.Symbol,
		Embedding:                embedding,
		At:                       artifact.At.UnixNano(),
		ForwardCurve:             artifact.ForwardCurve,
		ForwardRetention:         artifact.ForwardRetention,
		SupportedHorizon:         int64(artifact.SupportedHorizon),
		Calibrated:               artifact.Calibrated,
		ResolvedSteps:            int64(artifact.ResolvedSteps),
		Readout:                  artifact.Readout,
		Confidence:               artifact.Confidence,
		LastResolutionPrediction: artifact.LastResolutionPrediction,
		LastResolutionTarget:     artifact.LastResolutionTarget,
		LastResolutionError:      artifact.LastResolutionError,
		Dynamics:                 artifact.Dynamics,
	}

	if artifact.Forecast != nil {
		wire.Forecast = &telemetry.ResonanceForecastT{
			Distribution: &telemetry.PosteriorT{
				Value:            artifact.Forecast.Distribution.Value,
				Scale:            artifact.Forecast.Distribution.Scale,
				DegreesOfFreedom: artifact.Forecast.Distribution.DegreesOfFreedom,
				Ready:            artifact.Forecast.Distribution.Ready,
				Innovation:       artifact.Forecast.Distribution.Innovation,
				Reset:            artifact.Forecast.Distribution.Reset,
			},
			Horizon:          int64(artifact.Forecast.Horizon),
			CandidateCall:    artifact.Forecast.CandidateCall,
			Call:             artifact.Forecast.Call,
			StableCall:       artifact.Forecast.StableCall,
			Held:             artifact.Forecast.Held,
			SwitchConfidence: artifact.Forecast.SwitchConfidence,
			SwitchThreshold:  artifact.Forecast.SwitchThreshold,
		}
	}

	if artifact.Snapshot != nil {
		wire.Energy = artifact.Snapshot.Energy
		wire.Surprise = artifact.Snapshot.Surprise
		wire.TaskSkill = artifact.Snapshot.SkillAverage
		wire.TaskSkillReady = artifact.Snapshot.SkillReadyAvg
		wire.TaskRelativePrecision = artifact.Snapshot.PrecisionAverage
		wire.TaskRelativePrecisionReady = artifact.Snapshot.PrecisionReadyAvg
		wire.TaskScale = artifact.Snapshot.ScaleAverage
		wire.TaskScaleReady = artifact.Snapshot.ScaleReadyAvg

		if len(artifact.Snapshot.Latent) > 0 {
			wire.Latent = make([]float64, len(artifact.Snapshot.Latent))
			copy(wire.Latent, artifact.Snapshot.Latent)
		}

		if len(artifact.Snapshot.Layers) > 0 {
			wire.Layers = make([]*telemetry.ResonanceLayerT, len(artifact.Snapshot.Layers))
			for i, layer := range artifact.Snapshot.Layers {
				wire.Layers[i] = &telemetry.ResonanceLayerT{
					State:      layer.State,
					Prediction: layer.Prediction,
					ErrorNorm:  layer.ErrorNorm,
					Temporal:   layer.Temporal,
				}
			}
		}
	}

	return wire
}
