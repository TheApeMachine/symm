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
		measurement := data.NewMeasurement[float64]("category/"+string(category.Type), nil)
		measurement.Label, measurement.At, measurement.From = category.Symbol, category.At, category.At
		measurement.Metadata = map[string]float64{data.MetadataMaturity: category.Maturity}
		measurement.Metrics["strength"] = data.Metric[float64]{Label: "strength", Raw: category.Strength}
		measurement.Metrics["confidence"] = data.Metric[float64]{Label: "confidence", Raw: category.Confidence}
		measurement.Metrics["surprisal"] = data.Metric[float64]{Label: "surprisal", Raw: category.Surprisal}
		measurement.Metrics["uncertainty"] = data.Metric[float64]{Label: "uncertainty", Raw: category.Uncertainty}
		measurement.Metrics["freshness"] = data.Metric[float64]{Label: "freshness", Raw: category.Freshness}
		output = append(output, measurement)
	}

	if reading := envelope.Cognition; reading != nil {
		measurement := data.NewMeasurement[float64]("cognition", nil)
		measurement.Label, measurement.At, measurement.From = reading.Symbol, reading.At, reading.At
		// Cohort is the producer's observed sequence support, retained as a
		// sufficient statistic so Finalize applies the canonical maturity rule.
		measurement.Metadata = map[string]float64{data.MetadataSupport: float64(reading.Cohort)}
		measurement.Metrics["confidence"] = data.Metric[float64]{Label: "confidence", Raw: reading.Confidence}
		measurement.Metrics["contrast"] = data.Metric[float64]{Label: "contrast", Raw: reading.Contrast}
		measurement.Metrics["surprisal"] = data.Metric[float64]{Label: "surprisal", Raw: reading.InterpolatedSurprisal}
		measurement.Metrics["lookahead_score"] = data.Metric[float64]{Label: "lookahead_score", Raw: reading.LookaheadScore}

		if reading.EntropyBits != nil {
			measurement.Metrics["entropy_bits"] = data.Metric[float64]{Label: "entropy_bits", Raw: *reading.EntropyBits}
		}
		output = append(output, measurement)
	}

	if reading := envelope.Resonance; reading != nil {
		measurement := data.NewMeasurement[float64]("resonance", nil)
		measurement.Label, measurement.At, measurement.From = reading.Symbol, reading.At, reading.At
		measurement.Metadata = map[string]float64{data.MetadataSupport: float64(reading.ResolvedSteps)}
		measurement.Metrics["confidence"] = data.Metric[float64]{Label: "confidence", Raw: reading.Confidence}

		for index, value := range reading.Readout {
			measurement.Metrics[seriesLabel("readout/", index)] = data.Metric[float64]{Label: seriesLabel("readout/", index), Raw: value}
		}

		for index, value := range reading.ForwardCurve {
			measurement.Metrics[seriesLabel("forward/", index)] = data.Metric[float64]{Label: seriesLabel("forward/", index), Raw: value}
		}

		for index, value := range reading.ForwardRetention {
			measurement.Metrics[seriesLabel("retention/", index)] = data.Metric[float64]{Label: seriesLabel("retention/", index), Raw: value}
		}

		if forecast := reading.Forecast; forecast != nil {
			measurement.Metrics["call"] = data.Metric[float64]{Label: "call", Raw: forecast.Call}
			measurement.Metrics["switch_confidence"] = data.Metric[float64]{Label: "switch_confidence", Raw: forecast.SwitchConfidence}
		}
		output = append(output, measurement)
	}

	if reading := envelope.Manifold; reading != nil && envelope.Symbol() != "" {
		// These are the field observations available to this symbol's event.
		// Particle buffers remain owned by the manifold and are never copied to the grid.
		measurement := data.NewMeasurement[float64]("manifold", nil)
		measurement.Label, measurement.At, measurement.From = envelope.Symbol(), reading.At, reading.At
		measurement.Metrics["divergence"] = data.Metric[float64]{Label: "divergence", Raw: reading.Divergence}
		measurement.Metrics["guidance_speed"] = data.Metric[float64]{Label: "guidance_speed", Raw: reading.GuidanceSpeed}
		measurement.Metrics["coherence_mag2"] = data.Metric[float64]{Label: "coherence_mag2", Raw: reading.CoherenceMag2}
		measurement.Metrics["pressure_gradient_norm"] = data.Metric[float64]{Label: "pressure_gradient_norm", Raw: reading.PressureGradNorm}
		measurement.Metrics["viscosity_proxy"] = data.Metric[float64]{Label: "viscosity_proxy", Raw: reading.ViscosityProxy}
		measurement.Metrics["kuramoto_r"] = data.Metric[float64]{Label: "kuramoto_r", Raw: reading.KuramotoR}

		// Gas hydrodynamics
		h := reading.Health
		measurement.Metrics["gas_vorticity_rms"] = data.Metric[float64]{Label: "gas_vorticity_rms", Raw: h.Gas.VorticityRMS}
		measurement.Metrics["gas_vorticity_max"] = data.Metric[float64]{Label: "gas_vorticity_max", Raw: h.Gas.VorticityMax}
		measurement.Metrics["gas_strain_rms"] = data.Metric[float64]{Label: "gas_strain_rms", Raw: h.Gas.StrainRMS}
		measurement.Metrics["gas_strain_max"] = data.Metric[float64]{Label: "gas_strain_max", Raw: h.Gas.StrainMax}
		measurement.Metrics["gas_max_mach"] = data.Metric[float64]{Label: "gas_max_mach", Raw: h.Gas.MaxMach}
		measurement.Metrics["gas_max_speed"] = data.Metric[float64]{Label: "gas_max_speed", Raw: h.Gas.MaxSpeed}
		measurement.Metrics["gas_internal_energy"] = data.Metric[float64]{Label: "gas_internal_energy", Raw: h.Gas.Internal}
		measurement.Metrics["gas_kinetic_energy"] = data.Metric[float64]{Label: "gas_kinetic_energy", Raw: h.Gas.Kinetic}
		measurement.Metrics["gas_viscous_power"] = data.Metric[float64]{Label: "gas_viscous_power", Raw: h.Gas.ViscousPower}
		measurement.Metrics["gas_min_pressure"] = data.Metric[float64]{Label: "gas_min_pressure", Raw: h.Gas.MinPressure}
		measurement.Metrics["gas_min_density"] = data.Metric[float64]{Label: "gas_min_density", Raw: h.Gas.MinDensity}
		measurement.Metrics["gas_min_temperature"] = data.Metric[float64]{Label: "gas_min_temperature", Raw: h.Gas.MinTemperature}

		// Quantum spatial wave field
		measurement.Metrics["wave_norm"] = data.Metric[float64]{Label: "wave_norm", Raw: h.Wave.Norm}
		measurement.Metrics["wave_kinetic_energy"] = data.Metric[float64]{Label: "wave_kinetic_energy", Raw: h.Wave.Kinetic}
		measurement.Metrics["wave_nonlinear_energy"] = data.Metric[float64]{Label: "wave_nonlinear_energy", Raw: h.Wave.Nonlinear}
		measurement.Metrics["wave_potential_energy"] = data.Metric[float64]{Label: "wave_potential_energy", Raw: h.Wave.Potential}
		measurement.Metrics["wave_chemical_potential"] = data.Metric[float64]{Label: "wave_chemical_potential", Raw: h.Wave.Chemical}
		measurement.Metrics["wave_projected_norm"] = data.Metric[float64]{Label: "wave_projected_norm", Raw: h.Wave.ProjectedNorm}
		measurement.Metrics["wave_phase_potential"] = data.Metric[float64]{Label: "wave_phase_potential", Raw: h.Wave.PhasePotential}

		// Bohmian pilot wave guidance
		measurement.Metrics["pilot_speed_rms"] = data.Metric[float64]{Label: "pilot_speed_rms", Raw: h.Pilot.SpeedRMS}
		measurement.Metrics["pilot_speed_max"] = data.Metric[float64]{Label: "pilot_speed_max", Raw: h.Pilot.SpeedMax}
		measurement.Metrics["pilot_displacement_rms"] = data.Metric[float64]{Label: "pilot_displacement_rms", Raw: h.Pilot.DisplacementRMS}
		measurement.Metrics["pilot_displacement_max"] = data.Metric[float64]{Label: "pilot_displacement_max", Raw: h.Pilot.DisplacementMax}
		measurement.Metrics["pilot_min_density"] = data.Metric[float64]{Label: "pilot_min_density", Raw: h.Pilot.MinDensity}
		measurement.Metrics["pilot_integration_error_max"] = data.Metric[float64]{Label: "pilot_integration_error_max", Raw: h.Pilot.IntegrationErrorMax}

		// Hamiltonian coherence, work & dissipative phase flow
		measurement.Metrics["coherence_mechanical_work"] = data.Metric[float64]{Label: "coherence_mechanical_work", Raw: h.Sources.CoherenceMechanicalWork}
		measurement.Metrics["coherence_potential_work"] = data.Metric[float64]{Label: "coherence_potential_work", Raw: h.Sources.CoherencePotentialWork}
		measurement.Metrics["coherence_drive_work"] = data.Metric[float64]{Label: "coherence_drive_work", Raw: h.Sources.CoherenceDriveWork}
		measurement.Metrics["coherence_damping_work"] = data.Metric[float64]{Label: "coherence_damping_work", Raw: h.Sources.CoherenceDampingWork}
		measurement.Metrics["pilot_work"] = data.Metric[float64]{Label: "pilot_work", Raw: h.Sources.PilotWork}
		measurement.Metrics["phase_drive_work"] = data.Metric[float64]{Label: "phase_drive_work", Raw: h.Sources.PhaseDriveWork}
		measurement.Metrics["phase_dissipation"] = data.Metric[float64]{Label: "phase_dissipation", Raw: h.Sources.PhaseDissipation}
		measurement.Metrics["thermal_conduction_net"] = data.Metric[float64]{Label: "thermal_conduction_net", Raw: h.Sources.ThermalConductionNet}
		measurement.Metrics["viscous_to_heat"] = data.Metric[float64]{Label: "viscous_to_heat", Raw: h.Sources.ViscousToHeat}

		// Resident particle energies & integrator time step
		measurement.Metrics["particle_material_total"] = data.Metric[float64]{Label: "particle_material_total", Raw: h.ParticleMaterialTotal}
		measurement.Metrics["particle_thermal"] = data.Metric[float64]{Label: "particle_thermal", Raw: h.ParticleThermal}
		measurement.Metrics["particle_oscillator"] = data.Metric[float64]{Label: "particle_oscillator", Raw: h.ParticleOscillator}
		measurement.Metrics["particle_kinetic"] = data.Metric[float64]{Label: "particle_kinetic", Raw: h.ParticleKinetic}
		measurement.Metrics["integrator_accepted_dt"] = data.Metric[float64]{Label: "integrator_accepted_dt", Raw: h.Integrator.AcceptedDT}

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
	measurement := data.NewMeasurement[float64](reading.Source, nil)
	measurement.Label, measurement.At, measurement.From = reading.Label, time.Unix(0, reading.AtNs), from
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
