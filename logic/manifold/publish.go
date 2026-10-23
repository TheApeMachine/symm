package manifold

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/types"
)

const (
	readingOrigin = "manifold"
	readingRole   = "reading"
)

func appendScalar(value *data.Value, key string, reading float64) {
	value.Append([]float64{reading}, key)
}

func appendBoolScalar(value *data.Value, key string, on bool) {
	if on {
		appendScalar(value, key, 1)
		return
	}
	appendScalar(value, key, 0)
}

func appendIntegrator(value *data.Value, integrator sensorium.IntegratorHealth) {
	appendScalar(value, "integrator:contact_dt", integrator.ContactDT)
	appendScalar(value, "integrator:requested_dt", integrator.RequestedDT)
	appendScalar(value, "integrator:target_dt", integrator.TargetDT)
	appendScalar(value, "integrator:accepted_dt", integrator.AcceptedDT)
	appendScalar(value, "integrator:last_dt", integrator.LastDT)
	appendScalar(value, "integrator:min_dt", integrator.MinDT)
	appendScalar(value, "integrator:time", integrator.Time)
	appendScalar(value, "integrator:hyperbolic_dt", integrator.HyperbolicDT)
	appendScalar(value, "integrator:viscous_dt", integrator.ViscousDT)
	appendScalar(value, "integrator:thermal_dt", integrator.ThermalDT)
	appendScalar(value, "integrator:particle_dt", integrator.ParticleDT)
	appendScalar(value, "integrator:phase_dt", integrator.PhaseDT)
	appendScalar(value, "integrator:combined_dt", integrator.CombinedDT)
	appendScalar(value, "integrator:substeps", float64(integrator.Substeps))
	appendScalar(value, "integrator:rejections", float64(integrator.Rejections))
}

func appendGas(value *data.Value, gas sensorium.GasHealth) {
	appendScalar(value, "gas:mass", gas.Mass)
	appendScalar(value, "gas:internal", gas.Internal)
	appendScalar(value, "gas:kinetic", gas.Kinetic)
	appendScalar(value, "gas:total", gas.Total)
	appendScalar(value, "gas:min_density", gas.MinDensity)
	appendScalar(value, "gas:min_pressure", gas.MinPressure)
	appendScalar(value, "gas:min_temperature", gas.MinTemperature)
	appendScalar(value, "gas:max_speed", gas.MaxSpeed)
	appendScalar(value, "gas:max_sound", gas.MaxSound)
	appendScalar(value, "gas:max_mach", gas.MaxMach)
	appendScalar(value, "gas:vorticity_rms", gas.VorticityRMS)
	appendScalar(value, "gas:vorticity_max", gas.VorticityMax)
	appendScalar(value, "gas:strain_rms", gas.StrainRMS)
	appendScalar(value, "gas:strain_max", gas.StrainMax)
	appendScalar(value, "gas:viscous_power", gas.ViscousPower)
}

func appendWave(value *data.Value, wave sensorium.WaveHealth) {
	appendScalar(value, "wave:norm", wave.Norm)
	appendScalar(value, "wave:kinetic", wave.Kinetic)
	appendScalar(value, "wave:potential", wave.Potential)
	appendScalar(value, "wave:nonlinear", wave.Nonlinear)
	appendScalar(value, "wave:chemical", wave.Chemical)
	appendScalar(value, "wave:projected_norm", wave.ProjectedNorm)
	appendScalar(value, "wave:phase_potential", wave.PhasePotential)
}

func appendPilot(value *data.Value, pilot sensorium.PilotHealth) {
	appendScalar(value, "pilot:density_p01", pilot.DensityP01)
	appendScalar(value, "pilot:density_p10", pilot.DensityP10)
	appendScalar(value, "pilot:density_median", pilot.DensityMedian)
	appendScalar(value, "pilot:integration_error_max", pilot.IntegrationErrorMax)
	appendScalar(value, "pilot:speed_rms", pilot.SpeedRMS)
	appendScalar(value, "pilot:speed_max", pilot.SpeedMax)
	appendScalar(value, "pilot:displacement_rms", pilot.DisplacementRMS)
	appendScalar(value, "pilot:displacement_max", pilot.DisplacementMax)
	appendScalar(value, "pilot:min_density", pilot.MinDensity)
}

func appendSources(value *data.Value, sources sensorium.SourceLedger) {
	appendScalar(value, "sources:gas_energy_residual", sources.GasEnergyResidual)
	appendScalar(value, "sources:conservative_wave_error", sources.ConservativeWaveError)
	appendScalar(value, "sources:pic_deposit_energy_residual", sources.PICDepositEnergyResidual)
	appendScalar(value, "sources:particle_balance_residual", sources.ParticleBalanceResidual)
	appendScalar(value, "sources:gravity_balance_residual", sources.GravityBalanceResidual)
}

func appendHealth(value *data.Value, health sensorium.PhysicsHealth) {
	appendIntegrator(value, health.Integrator)
	appendGas(value, health.Gas)
	appendWave(value, health.Wave)
	appendPilot(value, health.Pilot)
	appendSources(value, health.Sources)
	appendScalar(value, "particle_thermal", health.ParticleThermal)
	appendScalar(value, "particle_oscillator", health.ParticleOscillator)
	appendScalar(value, "particle_kinetic", health.ParticleKinetic)
	appendScalar(value, "particle_material_total", health.ParticleMaterialTotal)
	appendScalar(value, "spatial_sigma_raw", health.SpatialSigmaRaw)
	appendScalar(value, "spatial_sigma_used", health.SpatialSigmaUsed)
	appendBoolScalar(value, "sigma_uniform_limit", health.SigmaUniformLimit)
}

func appendResultant(value *data.Value, side string, resultant types.PhaseChannelResultant) {
	prefix := fmt.Sprintf("resultant:%s:", side)
	appendScalar(value, prefix+"count", float64(resultant.Count))
	appendScalar(value, prefix+"amplitude", resultant.TotalAmplitude)
	appendScalar(value, prefix+"coherence", resultant.Coherence)
	appendScalar(value, prefix+"phase", resultant.Phase)
}

func appendParticles(value *data.Value, state *sensorium.State) {
	if state == nil || state.N <= 0 {
		appendScalar(value, "particle_count", 0)
		return
	}

	appendScalar(value, "particle_count", float64(state.N))

	for index := 0; index < state.N; index++ {
		prefix := fmt.Sprintf("osc:%d:", index)
		posBase := index * 3
		appendScalar(value, prefix+"pos_x", float64(state.Pos[posBase+0]))
		appendScalar(value, prefix+"pos_y", float64(state.Pos[posBase+1]))
		appendScalar(value, prefix+"pos_z", float64(state.Pos[posBase+2]))
		appendScalar(value, prefix+"vel_x", float64(state.Vel[posBase+0]))
		appendScalar(value, prefix+"vel_y", float64(state.Vel[posBase+1]))
		appendScalar(value, prefix+"vel_z", float64(state.Vel[posBase+2]))
		appendScalar(value, prefix+"mass", float64(state.Mass[index]))
		appendScalar(value, prefix+"heat", float64(state.Heat[index]))
		appendScalar(value, prefix+"energy", float64(state.Energy[index]))
		appendScalar(value, prefix+"phase", float64(state.Phase[index]))
		appendScalar(value, prefix+"omega", float64(state.Omega[index]))
		appendScalar(value, prefix+"amp", float64(state.Amp[index]))

		side := 1.0
		if (state.TokenIDs[index] & 1) == 0 {
			side = 0
		}
		appendScalar(value, prefix+"side", side)

		if len(state.PilotVel) >= posBase+3 {
			appendScalar(value, prefix+"pilot_vx", float64(state.PilotVel[posBase+0]))
			appendScalar(value, prefix+"pilot_vy", float64(state.PilotVel[posBase+1]))
			appendScalar(value, prefix+"pilot_vz", float64(state.PilotVel[posBase+2]))
		}
	}
}

func appendModes(value *data.Value, modes []WaveMode) {
	for index, mode := range modes {
		prefix := fmt.Sprintf("mode:%d:", index)
		appendScalar(value, prefix+"omega", float64(mode.Omega))
		appendScalar(value, prefix+"real", float64(mode.Real))
		appendScalar(value, prefix+"imag", float64(mode.Imag))
		appendScalar(value, prefix+"linewidth", float64(mode.Linewidth))
	}
}

/*
PublishReading encodes the latest manifold advance as one dashboard Value.
Scalar layers use the same keys the fluid-3d wire decoder expects.
*/
func PublishReading(state *State, scope string, epoch, tick, timestamp uint64) *data.Value {
	if state == nil || state.State == nil {
		return nil
	}

	if scope == "" {
		scope = types.Focus()
	}

	value := data.NewValue(readingOrigin, readingRole, scope)
	value.Epoch = epoch
	value.Tick = tick
	value.Timestamp = timestamp

	reading := state.Reading
	appendScalar(value, "divergence", reading.Divergence)
	appendScalar(value, "guidance_speed", reading.GuidanceSpeed)
	appendScalar(value, "coherence_mag2", reading.CoherenceMag2)
	appendScalar(value, "pressure_grad_norm", reading.PressureGradNorm)
	appendScalar(value, "viscosity_proxy", reading.ViscosityProxy)
	appendScalar(value, "kuramoto_r", reading.KuramotoR)
	appendScalar(value, "kuramoto_psi", reading.KuramotoPsi)

	appendHealth(value, reading.Health)

	appendScalar(value, "grid_x", float64(state.GridX))
	appendScalar(value, "grid_y", float64(state.GridY))
	appendScalar(value, "grid_z", float64(state.GridZ))
	appendScalar(value, "grid_spacing", state.GridSpacing)
	appendScalar(value, "density_scale", float64(state.DensityScale))
	appendScalar(value, "momentum_scale", float64(state.MomentumScale))
	appendScalar(value, "energy_scale", float64(state.EnergyScale))
	appendScalar(value, "wave_scale", float64(state.WaveScale))

	appendParticles(value, state.State)
	appendModes(value, state.Modes)

	for _, resultant := range state.Resultants {
		if resultant.Side == "" {
			continue
		}
		appendResultant(value, resultant.Side, resultant)
	}

	if !reading.IsFinite() || math.IsNaN(float64(state.Version)) {
		return nil
	}

	return value
}
