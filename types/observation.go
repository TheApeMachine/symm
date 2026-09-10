package types

import (
	"bytes"
	"fmt"
	"strconv"
	"time"

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
		measurement.PutMetric(data.Metric[float64]{Label: "strength", Raw: category.Strength})
		measurement.PutMetric(data.Metric[float64]{Label: "confidence", Raw: category.Confidence})
		measurement.PutMetric(data.Metric[float64]{Label: "surprisal", Raw: category.Surprisal})
		measurement.PutMetric(data.Metric[float64]{Label: "uncertainty", Raw: category.Uncertainty})
		measurement.PutMetric(data.Metric[float64]{Label: "freshness", Raw: category.Freshness})
		output = append(output, measurement)
	}

	if reading := envelope.Cognition; reading != nil {
		measurement := data.NewMeasurement[float64](reading.Sequence, reading.Symbol, "cognition", reading.At, reading.At)
		// Cohort is the producer's observed sequence support, retained as a
		// sufficient statistic so Finalize applies the canonical maturity rule.
		measurement.Metadata = map[string]float64{data.MetadataSupport: float64(reading.Cohort)}
		measurement.PutMetric(data.Metric[float64]{Label: "confidence", Raw: reading.Confidence})
		measurement.PutMetric(data.Metric[float64]{Label: "contrast", Raw: reading.Contrast})
		measurement.PutMetric(data.Metric[float64]{Label: "surprisal", Raw: reading.InterpolatedSurprisal})
		measurement.PutMetric(data.Metric[float64]{Label: "lookahead_score", Raw: reading.LookaheadScore})

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
			measurement.PutMetric(data.Metric[float64]{Label: seriesLabel("readout/", index), Raw: value})
		}

		for index, value := range reading.ForwardCurve {
			measurement.PutMetric(data.Metric[float64]{Label: seriesLabel("forward/", index), Raw: value})
		}

		for index, value := range reading.ForwardRetention {
			measurement.PutMetric(data.Metric[float64]{Label: seriesLabel("retention/", index), Raw: value})
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
		measurement.PutMetric(data.Metric[float64]{Label: "divergence", Raw: reading.Divergence})
		measurement.PutMetric(data.Metric[float64]{Label: "guidance_speed", Raw: reading.GuidanceSpeed})
		measurement.PutMetric(data.Metric[float64]{Label: "coherence_mag2", Raw: reading.CoherenceMag2})
		measurement.PutMetric(data.Metric[float64]{Label: "pressure_gradient_norm", Raw: reading.PressureGradNorm})
		measurement.PutMetric(data.Metric[float64]{Label: "viscosity_proxy", Raw: reading.ViscosityProxy})
		measurement.PutMetric(data.Metric[float64]{Label: "kuramoto_r", Raw: reading.KuramotoR})
		output = append(output, measurement)
	}

	return output
}

func seriesLabel(prefix string, index int) string {
	if index >= 0 && index < len(seriesIndex) {
		return prefix + seriesIndex[index]
	}

	return prefix + strconv.Itoa(index)
}

var seriesIndex = [...]string{
	"0", "1", "2", "3", "4", "5", "6", "7",
	"8", "9", "10", "11", "12", "13", "14", "15",
	"16", "17", "18", "19", "20", "21", "22", "23",
	"24", "25", "26", "27", "28", "29", "30", "31",
}

func (envelope *Envelope) hasNumericalInput() bool {
	if envelope == nil {
		return false
	}

	for _, measurement := range envelope.SignalMeasurements() {
		if measurement != nil {
			return true
		}
	}

	if len(envelope.Categories) > 0 || envelope.Cognition != nil || envelope.Resonance != nil {
		return true
	}

	return envelope.Manifold != nil && envelope.Symbol() != ""
}

// EncodePrecursor persists every numerical input at its capture coordinate.
// It omits wallet state, graph trees and resident particle/volume buffers.
func (envelope *Envelope) EncodePrecursor() []byte {
	if !envelope.hasNumericalInput() {
		return nil
	}
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

/*
MeasurementsFromState reads back the observations one captured envelope held.

It is the inverse of EncodePrecursor, which writes exactly Measurements() into
the stored state, so what comes back is what the running binary was looking at
at that capture identity — in the same representation every other stage speaks,
with no second shape in between.
*/
func MeasurementsFromState(payload []byte) (measurements []*data.Measurement[float64], err error) {
	defer func() {
		if invalid := recover(); invalid != nil {
			measurements, err = nil, fmt.Errorf("envelope: malformed captured state: %v", invalid)
		}
	}()
	state := wire.GetRootAsEnvelopeState(payload, 0)

	for index := range state.LearningObservationsLength() {
		encoded := new(wire.EnvelopeMeasurement)

		if !state.LearningObservations(encoded, index) {
			return nil, fmt.Errorf("envelope: absent observation %d of captured state", index)
		}
		measurements = append(measurements, decodeMeasurement(encoded.UnPack()))
	}

	return measurements, nil
}

/*
decodeMeasurement rebuilds one reading from its stored form. Absent optional
components stay absent: a normalization that was never computed is not a zero.
*/
func decodeMeasurement(reading *wire.EnvelopeMeasurementT) *data.Measurement[float64] {
	from := time.Time{}

	if reading.HasFrom {
		from = time.Unix(0, reading.FromNs)
	}
	measurement := data.NewMeasurement[float64](
		reading.Id, reading.Label, reading.Source, time.Unix(0, reading.AtNs), from,
	)
	measurement.SeqIdx, measurement.Maturity = reading.SeqIdx, reading.Maturity
	measurement.SNR, measurement.SNRDefined = reading.Snr, reading.SnrDefined
	measurement.Metadata = make(map[string]float64, len(reading.Metadata))
	measurement.Provenance = make(map[string]string, len(reading.Provenance))

	for _, fact := range reading.Metadata {
		measurement.Metadata[fact.Name] = fact.Value
	}

	for _, fact := range reading.Provenance {
		measurement.Provenance[fact.Name] = fact.Value
	}

	for _, metric := range reading.Metrics {
		if metric == nil || metric.Value == nil {
			continue
		}
		value := metric.Value
		decoded := data.Metric[float64]{
			Label: value.Label, Raw: value.Raw,
			Unit: data.Unit(value.Unit), Timescale: data.Timescale(value.Timescale),
		}

		if value.HasNormalized {
			decoded.Normalized = &value.Normalized
		}

		if value.HasStandardized {
			decoded.Standardized = &value.Standardized
		}
		measurement.Metrics[metric.Key] = decoded
	}

	return measurement
}
