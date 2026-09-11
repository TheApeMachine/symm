package sensorium

import (
	"fmt"
	"math"
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
	return PhysicsControls{Units: ModelUnits{hbarEff, 1}, CFL: .4, ParticleCells: .4, PhaseRadians: .5, EtaPressure: 1e-3, EtaSync: .1, MaxStep: dtMax, MaxSubsteps: 4096, MaxRetries: 16, RemapWidthCells: 1, RemapTolerance: 2e-5, RemapIterations: 4096, PilotTolerance: 2e-5}
}
func (p PhysicsControls) validate() error {
	if err := p.Contacts.validate(); err != nil {
		return err
	}
	if !isPositiveFinite(p.Units.Hbar) || !isPositiveFinite(p.Units.Boltzmann) ||
		!isPositiveFinite(p.CFL) || p.CFL > .5 || !isPositiveFinite(p.ParticleCells) || p.ParticleCells > .5 ||
		!isPositiveFinite(p.PhaseRadians) || p.PhaseRadians > 1 || !isPositiveFinite(p.EtaSync) || !isPositiveFinite(p.EtaPressure) || p.EtaPressure >= 1 || p.EtaSync < p.EtaPressure || p.EtaSync >= 1 ||
		!isPositiveFinite(p.MaxStep) || p.MaxSubsteps < 1 || p.MaxRetries < 0 || !finite(p.GravityG) || p.GravityG < 0 || !isPositiveFinite(p.RemapWidthCells) || !finite(p.RemapTolerance) || p.RemapTolerance < 8*math.Ldexp(1, -23) || p.RemapTolerance > 1e-3 || p.RemapIterations < 1 || !finite(p.PilotTolerance) || p.PilotTolerance < 8*math.Ldexp(1, -23) || p.PilotTolerance > .01 {
		return fmt.Errorf("sensorium: invalid physics controls: %+v", p)
	}
	return nil
}

// PlanckTarget excludes zero-point energy. omega==0 uses the CONTINUOUS kB*T
// limit, not the discontinuous zero used by the former helper.
func PlanckTarget(omega, temperature float64, units ModelUnits) (float64, error) {
	if !finite(omega) || !finite(temperature) || temperature < 0 ||
		!isPositiveFinite(units.Hbar) || !isPositiveFinite(units.Boltzmann) {
		return 0, fmt.Errorf("invalid Planck arguments omega=%g T=%g units=%+v", omega, temperature, units)
	}
	theta := units.Boltzmann * temperature
	epsilon := units.Hbar * math.Abs(omega)
	if !finite(theta) || !finite(epsilon) {
		return 0, fmt.Errorf("Planck scale overflow")
	}
	if theta == 0 {
		return 0, nil
	}
	if epsilon == 0 {
		return theta, nil
	}
	x := epsilon / theta
	if x == 0 {
		return theta, nil
	}
	var result float64
	if x > 50 {
		result = epsilon * math.Exp(-x)
	} else {
		result = theta * (x / math.Expm1(x))
	}
	if !finite(result) || result < 0 {
		return 0, fmt.Errorf("invalid Planck target %g", result)
	}
	return result, nil
}

// PlanckTransfer is a finite-reservoir, frozen-initial-temperature relaxation.
// The interval intersection bounds the TRANSFER, never repairs an invalid state.
// Both stores are validated before either is returned to a caller for commit.
func PlanckTransfer(q, osc, mass, omega, cv, conductivity, radius, dt float64, units ModelUnits) (float32, float32, float64, error) {
	bad := !finite(q) || q < 0 || !finite(osc) || osc < 0 || !isPositiveFinite(mass) ||
		!isPositiveFinite(cv) || !finite(conductivity) || conductivity < 0 || !isPositiveFinite(radius) || !finite(dt) || dt < 0
	if bad {
		return 0, 0, 0, fmt.Errorf("invalid reservoir: Q=%g osc=%g m=%g cv=%g k=%g r=%g dt=%g", q, osc, mass, cv, conductivity, radius, dt)
	}
	capacity := mass * cv
	if !isPositiveFinite(capacity) {
		return 0, 0, 0, fmt.Errorf("invalid heat capacity")
	}
	target, err := PlanckTarget(omega, q/capacity, units)
	if err != nil {
		return 0, 0, 0, err
	}
	rate := 4 * math.Pi * conductivity * radius / capacity
	if !finite(rate) {
		return 0, 0, 0, fmt.Errorf("Planck relaxation rate overflow")
	}
	fraction := -math.Expm1(-dt * rate)
	transfer := fraction * (target - osc)
	transfer = math.Max(-osc, math.Min(q, transfer))
	q1, e1 := float32(q-transfer), float32(osc+transfer)
	if !finite(float64(q1)) || !finite(float64(e1)) || q1 < 0 || e1 < 0 {
		return 0, 0, 0, fmt.Errorf("Planck poststate Q=%g osc=%g transfer=%g -> Q=%g osc=%g", q, osc, transfer, q1, e1)
	}
	residual := float64(q1) + float64(e1) - q - osc
	tolerance := 4 * math.Ldexp(1, -23) * (q + osc)
	if math.Abs(residual) > tolerance {
		return 0, 0, 0, fmt.Errorf("Planck conservation residual %g > %g", residual, tolerance)
	}
	return q1, e1, residual, nil
}

// CoupledStepError distinguishes a numerical retry from invalid input/model state.
type CoupledStepError struct {
	Operator string
	Index    int
	Retry    bool
	Detail   string
}

func (e *CoupledStepError) Error() string {
	return fmt.Sprintf("sensorium %s[%d]: %s", e.Operator, e.Index, e.Detail)
}

// advanceCoupled owns the entire macro interval. Every successful callback uses
// exactly the float32 interval passed to ALL its physical operators. A failed
// attempt is restored BEFORE a smaller interval is tried. Failure restores the
// macro snapshot too: a partially completed macro is never published as success.
// snapshot/restore also cover phase/RNG state and source ledgers, not just gas.
func advanceCoupled(request float64, controls PhysicsControls, snapshot func() func(),
	limit func() (float64, error), attempt func(float32) error) (IntegratorHealth, error) {
	h := IntegratorHealth{}
	if err := controls.validate(); err != nil {
		return h, err
	}
	requested := float64(float32(request))
	if !isPositiveFinite(request) || !isPositiveFinite(requested) {
		return h, fmt.Errorf("invalid macro step %g", request)
	}
	h.RequestedDT = request
	h.TargetDT = requested
	rollbackMacro := snapshot()
	remaining := requested
	for remaining > 0 {
		if h.Substeps >= controls.MaxSubsteps {
			rollbackMacro()
			return h, fmt.Errorf("macro interval not resolved: remaining=%g substeps=%d", remaining, h.Substeps)
		}
		bound, err := limit()
		if err != nil {
			rollbackMacro()
			return h, err
		}
		if !isPositiveFinite(bound) {
			rollbackMacro()
			return h, fmt.Errorf("nonpositive/nonfinite stability bound %g", bound)
		}
		dt := float32(math.Min(remaining, math.Min(controls.MaxStep, bound)))
		if float64(dt) > math.Min(remaining, math.Min(controls.MaxStep, bound)) {
			dt = math.Nextafter32(dt, 0)
		}
		rollbackAttempt := snapshot()
		for retry := 0; ; retry++ {
			if dt <= 0 || float64(dt) > remaining || remaining-float64(dt) == remaining {
				rollbackMacro()
				return h, fmt.Errorf("coupled timestep underflow dt=%g remaining=%g", dt, remaining)
			}
			err = attempt(dt)
			if err == nil {
				break
			}
			rollbackAttempt()
			numerical, ok := err.(*CoupledStepError)
			if !ok || !numerical.Retry || retry >= controls.MaxRetries {
				rollbackMacro()
				return h, err
			}
			h.Rejections++
			dt *= .5
		}
		used := float64(dt)
		if h.Substeps == 0 || used < h.MinDT {
			h.MinDT = used
		}
		h.LastDT = used
		h.AcceptedDT += used
		h.Substeps++
		remaining -= used
	}
	return h, nil
}

// DefaultPhysicsControls returns the same explicit policy used by NewManifold.
// Study configurations start here rather than guessing omitted physical constants.
func DefaultPhysicsControls() PhysicsControls { return defaultPhysicsControls() }
