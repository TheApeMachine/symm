package types

import (
	"bytes"
	"strconv"

	"github.com/google/flatbuffers/go"
	"github.com/theapemachine/symm/nomagique/data"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

// Measurements exposes signal and logic observations to the grid through one
// boundary. Category fields retain their measured meanings and maturity.
func (envelope *Envelope) Measurements() []*data.Measurement[float64] {
	signals := envelope.SignalMeasurements()
	output := make([]*data.Measurement[float64], 0, len(signals)+len(envelope.Categories)+1)
	for _, measurement := range signals {
		if measurement != nil {
			output = append(output, measurement)
		}
	}
	output = append(output, envelope.LogicMeasurements()...)
	return output
}

// LogicMeasurements preserves the numeric outputs of downstream solvers.
func (envelope *Envelope) LogicMeasurements() []*data.Measurement[float64] {
	var output []*data.Measurement[float64]
	for _, category := range envelope.Categories {
		measurement := data.NewMeasurement[float64](string(category.Type), category.Symbol, "category/"+string(category.Type), category.At, category.At)
		measurement.Metadata = map[string]float64{data.MetadataMaturity: category.Maturity}
		for label, value := range map[string]float64{"strength": category.Strength, "confidence": category.Confidence,
			"surprisal": category.Surprisal, "uncertainty": category.Uncertainty, "freshness": category.Freshness} {
			measurement.PutMetric(data.Metric[float64]{Label: label, Raw: value})
		}
		output = append(output, measurement)
	}

	if reading := envelope.Cognition; reading != nil {
		measurement := data.NewMeasurement[float64](reading.Sequence, reading.Symbol, "cognition", reading.At, reading.At)
		// Cohort is the producer's observed sequence support, retained as a
		// sufficient statistic so Finalize applies the canonical maturity rule.
		measurement.Metadata = map[string]float64{data.MetadataSupport: float64(reading.Cohort)}
		for label, value := range map[string]float64{"confidence": reading.Confidence, "contrast": reading.Contrast,
			"surprisal": reading.InterpolatedSurprisal, "lookahead_score": reading.LookaheadScore} {
			measurement.PutMetric(data.Metric[float64]{Label: label, Raw: value})
		}

		if reading.EntropyBits != nil {
			measurement.PutMetric(data.Metric[float64]{Label: "entropy_bits", Raw: *reading.EntropyBits})
		}
		output = append(output, measurement)
	}

	if reading := envelope.Resonance; reading != nil {
		measurement := data.NewMeasurement[float64]("resonance", reading.Symbol, "resonance", reading.At, reading.At)
		measurement.Metadata = map[string]float64{data.MetadataSupport: float64(reading.ResolvedSteps)}
		measurement.PutMetric(data.Metric[float64]{Label: "confidence", Raw: reading.Confidence})
		for index, value := range reading.Readout {
			measurement.PutMetric(data.Metric[float64]{Label: "readout/" + strconv.Itoa(index), Raw: value})
		}
		for index, value := range reading.ForwardCurve {
			measurement.PutMetric(data.Metric[float64]{Label: "forward/" + strconv.Itoa(index), Raw: value})
		}
		for index, value := range reading.ForwardRetention {
			measurement.PutMetric(data.Metric[float64]{Label: "retention/" + strconv.Itoa(index), Raw: value})
		}

		if forecast := reading.Forecast; forecast != nil {
			measurement.PutMetric(data.Metric[float64]{Label: "call", Raw: forecast.Call})
			measurement.PutMetric(data.Metric[float64]{Label: "switch_confidence", Raw: forecast.SwitchConfidence})
		}
		output = append(output, measurement)
	}

	if reading := envelope.Manifold; reading != nil && envelope.Symbol() != "" {
		// These are the field observations available to this symbol's event.
		// Particle buffers remain owned by the manifold and are never copied to the grid.
		measurement := data.NewMeasurement[float64]("field", envelope.Symbol(), "manifold", reading.At, reading.At)
		for label, value := range map[string]float64{
			"divergence": reading.Divergence, "guidance_speed": reading.GuidanceSpeed,
			"coherence_mag2": reading.CoherenceMag2, "pressure_gradient_norm": reading.PressureGradNorm,
			"viscosity_proxy": reading.ViscosityProxy, "kuramoto_r": reading.KuramotoR,
		} {
			measurement.PutMetric(data.Metric[float64]{Label: label, Raw: value})
		}
		output = append(output, measurement)
	}
	return output
}

// EncodePrecursor persists every numerical input at its capture coordinate.
// It omits wallet state, graph trees and resident particle/volume buffers.
func (envelope *Envelope) EncodePrecursor() []byte {
	measurements := envelope.Measurements()

	if len(measurements) == 0 {
		return nil
	}
	state := &wire.EnvelopeStateT{
		Key: envelope.Key, TypeId: byte(envelope.TypeID), Tick: envelope.Tick,
		CaptureRun: string(envelope.CaptureID.Run), CaptureSeq: uint64(envelope.CaptureID.Sequence),
		CaptureStream: string(envelope.CaptureID.Stream), CaptureEpoch: uint64(envelope.CaptureID.StreamEpoch),
		CaptureStreamSeq: envelope.CaptureID.StreamSequence, CaptureOrdinal: envelope.CaptureOrdinal,
	}
	for _, measurement := range measurements {
		state.LearningObservations = append(state.LearningObservations, encodeMeasurement(measurement))
	}
	builder := envelopeBuilders.Get().(*flatbuffers.Builder)
	defer func() { builder.Reset(); envelopeBuilders.Put(builder) }()
	wire.FinishEnvelopeStateBuffer(builder, state.Pack(builder))
	return bytes.Clone(builder.FinishedBytes())
}
