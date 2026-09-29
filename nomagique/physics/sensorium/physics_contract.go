package sensorium

import (
	"errors"
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
)

// ModelUnits is the nondimensional action/thermal contract used by every operator.
// L0, t0, m0 fix E0=m0*L0^2/t0^2; temperature is fixed by kB*T0=E0.
// Market-to-state injection is external work, not a fundamental law of markets.
type ModelUnits struct{ Hbar, Boltzmann float64 }

// PhysicsControls contains numerical policies, not replacements for state variables.
// GravityG=0 explicitly disables gravity. No material-contact defaults are invented.
type PhysicsControls struct {
	Contacts                                               ContactMaterial
	Units                                                  ModelUnits
	CFL, ParticleCells, PhaseRadians, EtaPressure, EtaSync float64
	MaxStep                                                float64
	MaxSubsteps, MaxRetries                                int
	GravityG                                               float64
	RemapWidthCells, RemapTolerance                        float64
	RemapIterations                                        int
	PilotTolerance                                         float64
}

func defaultPhysicsControls() PhysicsControls {
	return PhysicsControls{
		Units:           ModelUnits{hbarEff, 1},
		CFL:             .4,
		ParticleCells:   .4,
		PhaseRadians:    .5,
		EtaPressure:     1e-3,
		EtaSync:         .1,
		MaxStep:         dtMax,
		MaxSubsteps:     4096,
		MaxRetries:      16,
		RemapWidthCells: 1,
		RemapTolerance:  2e-5,
		RemapIterations: 4096,
		PilotTolerance:  2e-5,
	}
}

func (controls PhysicsControls) validate() error {
	if err := controls.Contacts.validate(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[sensorium:physics_contract:validate] invalid contacts",
			err,
		))
	}

	if !isPositiveFinite(controls.Units.Hbar) || !isPositiveFinite(controls.Units.Boltzmann) ||
		!isPositiveFinite(controls.CFL) || controls.CFL > .5 || !isPositiveFinite(controls.ParticleCells) || controls.ParticleCells > .5 ||
		!isPositiveFinite(controls.PhaseRadians) || controls.PhaseRadians > 1 || !isPositiveFinite(controls.EtaSync) || !isPositiveFinite(controls.EtaPressure) || controls.EtaPressure >= 1 || controls.EtaSync < controls.EtaPressure || controls.EtaSync >= 1 ||
		!isPositiveFinite(controls.MaxStep) || controls.MaxSubsteps < 1 || controls.MaxRetries < 0 || !finite(controls.GravityG) || controls.GravityG < 0 || !isPositiveFinite(controls.RemapWidthCells) || !finite(controls.RemapTolerance) || controls.RemapTolerance < 8*math.Ldexp(1, -23) || controls.RemapTolerance > 1e-3 || controls.RemapIterations < 1 || !finite(controls.PilotTolerance) || controls.PilotTolerance < 8*math.Ldexp(1, -23) || controls.PilotTolerance > .01 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[sensorium:physics_contract:validate] invalid physics controls: %+v", controls),
			nil,
		))
	}

	return nil
}

// PlanckTarget excludes zero-point energy. omega==0 uses the CONTINUOUS kB*T
// limit, not the discontinuous zero used by the former helper.
func PlanckTarget(omega, temperature float64, units ModelUnits) (float64, error) {
	if !finite(omega) || !finite(temperature) || temperature < 0 ||
		!isPositiveFinite(units.Hbar) || !isPositiveFinite(units.Boltzmann) {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[sensorium:physics_contract:PlanckTarget] invalid Planck arguments omega=%g T=%g units=%+v", omega, temperature, units),
			nil,
		))
	}

	theta := units.Boltzmann * temperature
	epsilon := units.Hbar * math.Abs(omega)

	if !finite(theta) || !finite(epsilon) {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"[sensorium:physics_contract:PlanckTarget] Planck scale overflow",
			nil,
		))
	}

	if theta == 0 {
		return 0, nil
	}

	if epsilon == 0 {
		return theta, nil
	}

	ratio := epsilon / theta

	if ratio == 0 {
		return theta, nil
	}

	result := theta * (ratio / math.Expm1(ratio))

	if ratio > 50 {
		result = epsilon * math.Exp(-ratio)
	}

	if !finite(result) || result < 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[sensorium:physics_contract:PlanckTarget] invalid Planck target %g", result),
			nil,
		))
	}

	return result, nil
}

// PlanckTransfer is a finite-reservoir, frozen-initial-temperature relaxation.
// The interval intersection bounds the TRANSFER, never repairs an invalid state.
// Both stores are validated before either is returned to a caller for commit.
func PlanckTransfer(heat, osc, mass, omega, heatCapacity, conductivity, radius, dt float64, units ModelUnits) (float32, float32, float64, error) {
	bad := !finite(heat) || heat < 0 || !finite(osc) || osc < 0 || !isPositiveFinite(mass) ||
		!isPositiveFinite(heatCapacity) || !finite(conductivity) || conductivity < 0 || !isPositiveFinite(radius) || !finite(dt) || dt < 0

	if bad {
		return 0, 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[sensorium:physics_contract:PlanckTransfer] invalid reservoir: Q=%g osc=%g m=%g cv=%g k=%g r=%g dt=%g", heat, osc, mass, heatCapacity, conductivity, radius, dt),
			nil,
		))
	}

	capacity := mass * heatCapacity

	if !isPositiveFinite(capacity) {
		return 0, 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"[sensorium:physics_contract:PlanckTransfer] invalid heat capacity",
			nil,
		))
	}

	target, err := PlanckTarget(omega, heat/capacity, units)

	if err != nil {
		return 0, 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"[sensorium:physics_contract:PlanckTransfer] target resolution failed",
			err,
		))
	}

	rate := 4 * math.Pi * conductivity * radius / capacity

	if !finite(rate) {
		return 0, 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"[sensorium:physics_contract:PlanckTransfer] Planck relaxation rate overflow",
			nil,
		))
	}

	fraction := -math.Expm1(-dt * rate)
	transfer := fraction * (target - osc)
	transfer = math.Max(-osc, math.Min(heat, transfer))
	heatNext, oscNext := float32(heat-transfer), float32(osc+transfer)

	if !finite(float64(heatNext)) || !finite(float64(oscNext)) || heatNext < 0 || oscNext < 0 {
		return 0, 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[sensorium:physics_contract:PlanckTransfer] Planck poststate Q=%g osc=%g transfer=%g -> Q=%g osc=%g", heat, osc, transfer, heatNext, oscNext),
			nil,
		))
	}

	residual := float64(heatNext) + float64(oscNext) - heat - osc
	tolerance := 4 * math.Ldexp(1, -23) * (heat + osc)

	if math.Abs(residual) > tolerance {
		return 0, 0, 0, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[sensorium:physics_contract:PlanckTransfer] Planck conservation residual %g > %g", residual, tolerance),
			nil,
		))
	}

	return heatNext, oscNext, residual, nil
}

// CoupledStepError distinguishes a numerical retry from invalid input/model state.
type CoupledStepError struct {
	Operator string
	Index    int
	Retry    bool
	Detail   string
}

func (stepErr *CoupledStepError) Error() string {
	return fmt.Sprintf("sensorium %s[%d]: %s", stepErr.Operator, stepErr.Index, stepErr.Detail)
}

// advanceCoupled owns the entire macro interval. Every successful callback uses
// exactly the float32 interval passed to ALL its physical operators. A failed
// attempt is restored BEFORE a smaller interval is tried. Failure restores the
// macro snapshot too: a partially completed macro is never published as success.
// snapshot/restore also cover phase/RNG state and source ledgers, not just gas.
func advanceCoupled(request float64, controls PhysicsControls, snapshot func() func(),
	limit func() (float64, error), attempt func(float32) error) (IntegratorHealth, error) {
	health := IntegratorHealth{}

	if err := controls.validate(); err != nil {
		return health, errnie.Error(errnie.Err(
			errnie.Validation,
			"[sensorium:physics_contract:advanceCoupled] invalid physics controls",
			err,
		))
	}

	requested := float64(float32(request))

	if !isPositiveFinite(request) || !isPositiveFinite(requested) {
		return health, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[sensorium:physics_contract:advanceCoupled] invalid macro step %g", request),
			nil,
		))
	}

	health.RequestedDT = request
	health.TargetDT = requested
	rollbackMacro := snapshot()
	remaining := requested

	for remaining > 0 {
		if health.Substeps >= controls.MaxSubsteps {
			rollbackMacro()

			return health, errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("[sensorium:physics_contract:advanceCoupled] macro interval not resolved: remaining=%g substeps=%d", remaining, health.Substeps),
				nil,
			))
		}

		bound, err := limit()

		if err != nil {
			rollbackMacro()

			return health, errnie.Error(errnie.Err(
				errnie.Validation,
				"[sensorium:physics_contract:advanceCoupled] limit failed",
				err,
			))
		}

		if !isPositiveFinite(bound) {
			rollbackMacro()

			return health, errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("[sensorium:physics_contract:advanceCoupled] nonpositive/nonfinite stability bound %g", bound),
				nil,
			))
		}

		dt := float32(math.Min(remaining, math.Min(controls.MaxStep, bound)))

		if float64(dt) > math.Min(remaining, math.Min(controls.MaxStep, bound)) {
			dt = math.Nextafter32(dt, 0)
		}

		rollbackAttempt := snapshot()

		for retry := 0; ; retry++ {
			if dt <= 0 || float64(dt) > remaining || remaining-float64(dt) == remaining {
				rollbackMacro()

				return health, errnie.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf("[sensorium:physics_contract:advanceCoupled] coupled timestep underflow dt=%g remaining=%g", dt, remaining),
					nil,
				))
			}

			err = attempt(dt)

			if err == nil {
				break
			}

			rollbackAttempt()

			var numerical *CoupledStepError

			if !errors.As(err, &numerical) || !numerical.Retry || retry >= controls.MaxRetries {
				rollbackMacro()

				return health, errnie.Error(errnie.Err(
					errnie.Validation,
					"[sensorium:physics_contract:advanceCoupled] numerical step rejected",
					err,
				))
			}

			health.Rejections++
			dt *= .5
		}

		used := float64(dt)

		if health.Substeps == 0 || used < health.MinDT {
			health.MinDT = used
		}

		health.LastDT = used
		health.AcceptedDT += used
		health.Substeps++
		remaining -= used
	}

	return health, nil
}

// DefaultPhysicsControls returns the same explicit policy used by NewManifold.
// Study configurations start here rather than guessing omitted physical constants.
func DefaultPhysicsControls() PhysicsControls { return defaultPhysicsControls() }
