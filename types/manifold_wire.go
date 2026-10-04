package types

import (
	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/* EncodeManifold serializes one immutable published state, including its physics diagnostics. */
func EncodeManifold(manifold *ManifoldState) ([]byte, error) {
	if manifold == nil || manifold.State == nil {
		return nil, nil
	}
	builder := measurementsBuilderPool.Get().(*flatbuffers.Builder)
	builder.Reset()
	defer measurementsBuilderPool.Put(builder)
	state := manifold.State
	reading := manifold.Reading
	frame := &wire.ManifoldFrameT{
		Sequence:      manifold.Version,
		At:            manifold.At.UnixNano(),
		Version:       manifold.Version,
		N:             int64(state.N),
		Bytes:         state.Bytes,
		Seqs:          state.Seqs,
		TokenIds:      state.TokenIDs,
		ContentIds:    state.ContentIDs,
		Phase:         state.Phase,
		Omega:         state.Omega,
		Energy:        state.Energy,
		Mass:          state.Mass,
		Heat:          state.Heat,
		Amp:           state.Amp,
		Pos:           state.Pos,
		Vel:           state.Vel,
		PilotVel:      state.PilotVel,
		Clamped:       state.Clamped,
		Dark:          state.Dark,
		GridX:         int32(manifold.GridX),
		GridY:         int32(manifold.GridY),
		GridZ:         int32(manifold.GridZ),
		GridSpacing:   manifold.GridSpacing,
		MomRho:        manifold.MomRho,
		FieldEnergy:   manifold.FieldEnergy,
		WaveReal:      manifold.WaveReal,
		WaveImag:      manifold.WaveImag,
		DensityScale:  manifold.DensityScale,
		MomentumScale: manifold.MomentumScale,
		EnergyScale:   manifold.EnergyScale,
		WaveScale:     manifold.WaveScale,
		Reading: &wire.ManifoldReadingT{
			Divergence:       reading.Divergence,
			GuidanceSpeed:    reading.GuidanceSpeed,
			CoherenceMag2:    reading.CoherenceMag2,
			PressureGradNorm: reading.PressureGradNorm,
			ViscosityProxy:   reading.ViscosityProxy,
			KuramotoR:        reading.KuramotoR,
			KuramotoPsi:      reading.KuramotoPsi,
			Health:           encodePhysicsHealth(reading.Health),
		},
		Modes:      make([]*wire.WaveModeT, len(manifold.Modes)),
		Resultants: make([]*wire.PhaseResultantT, len(manifold.Resultants)),
	}
	for index, mode := range manifold.Modes {
		frame.Modes[index] = &wire.WaveModeT{Omega: mode.Omega, Real: mode.Real, Imaginary: mode.Imag, Linewidth: mode.Linewidth}
	}
	for index, resultant := range manifold.Resultants {
		frame.Resultants[index] = &wire.PhaseResultantT{Side: resultant.Side, Count: int32(resultant.Count), TotalAmplitude: resultant.TotalAmplitude, Coherence: resultant.Coherence, Phase: resultant.Phase}
	}
	offset := frame.Pack(builder)
	wire.MessageStart(builder)
	wire.MessageAddFrameType(builder, wire.FrameManifoldFrame)
	wire.MessageAddFrame(builder, offset)
	message := wire.MessageEnd(builder)
	builder.FinishWithFileIdentifier(message, []byte("SYMM"))
	return append([]byte(nil), builder.FinishedBytes()...), nil
}

func encodePhysicsHealth(health sensorium.PhysicsHealth) *wire.PhysicsHealthT {
	return &wire.PhysicsHealthT{
		Integrator: &wire.IntegratorHealthT{
			ContactDt:    health.Integrator.ContactDT,
			RequestedDt:  health.Integrator.RequestedDT,
			TargetDt:     health.Integrator.TargetDT,
			AcceptedDt:   health.Integrator.AcceptedDT,
			LastDt:       health.Integrator.LastDT,
			MinDt:        health.Integrator.MinDT,
			Time:         health.Integrator.Time,
			HyperbolicDt: health.Integrator.HyperbolicDT,
			ViscousDt:    health.Integrator.ViscousDT,
			ThermalDt:    health.Integrator.ThermalDT,
			ParticleDt:   health.Integrator.ParticleDT,
			PhaseDt:      health.Integrator.PhaseDT,
			CombinedDt:   health.Integrator.CombinedDT,
			Substeps:     int32(health.Integrator.Substeps),
			Rejections:   int32(health.Integrator.Rejections),
		},
		Gas: &wire.GasHealthT{
			Mass:           health.Gas.Mass,
			Internal:       health.Gas.Internal,
			Kinetic:        health.Gas.Kinetic,
			Total:          health.Gas.Total,
			Momentum:       health.Gas.Momentum[:],
			MinDensity:     health.Gas.MinDensity,
			MinPressure:    health.Gas.MinPressure,
			MinTemperature: health.Gas.MinTemperature,
			MaxSpeed:       health.Gas.MaxSpeed,
			MaxSound:       health.Gas.MaxSound,
			MaxMach:        health.Gas.MaxMach,
			VorticityRms:   health.Gas.VorticityRMS,
			VorticityMax:   health.Gas.VorticityMax,
			StrainRms:      health.Gas.StrainRMS,
			StrainMax:      health.Gas.StrainMax,
			ViscousPower:   health.Gas.ViscousPower,
		},
		Wave: &wire.WaveHealthT{
			Norm:           health.Wave.Norm,
			Kinetic:        health.Wave.Kinetic,
			Potential:      health.Wave.Potential,
			Nonlinear:      health.Wave.Nonlinear,
			Chemical:       health.Wave.Chemical,
			ProjectedNorm:  health.Wave.ProjectedNorm,
			PhasePotential: health.Wave.PhasePotential,
		},
		Pilot: &wire.PilotHealthT{
			DensityP01:          health.Pilot.DensityP01,
			DensityP10:          health.Pilot.DensityP10,
			DensityMedian:       health.Pilot.DensityMedian,
			IntegrationErrorMax: health.Pilot.IntegrationErrorMax,
			SpeedRms:            health.Pilot.SpeedRMS,
			SpeedMax:            health.Pilot.SpeedMax,
			DisplacementRms:     health.Pilot.DisplacementRMS,
			DisplacementMax:     health.Pilot.DisplacementMax,
			MinDensity:          health.Pilot.MinDensity,
		},
		Sources: &wire.SourceLedgerT{
			GasEnergyResidual:        health.Sources.GasEnergyResidual,
			ConservativeWaveError:    health.Sources.ConservativeWaveError,
			PicDepositEnergyResidual: health.Sources.PICDepositEnergyResidual,
			ParticleBalanceResidual:  health.Sources.ParticleBalanceResidual,
			GravityBalanceResidual:   health.Sources.GravityBalanceResidual,
		},
		ParticleThermal:       health.ParticleThermal,
		ParticleOscillator:    health.ParticleOscillator,
		ParticleKinetic:       health.ParticleKinetic,
		ParticleMaterialTotal: health.ParticleMaterialTotal,
		SpatialSigmaRaw:       health.SpatialSigmaRaw,
		SpatialSigmaUsed:      health.SpatialSigmaUsed,
		SigmaUniformLimit:     health.SigmaUniformLimit,
	}
}
