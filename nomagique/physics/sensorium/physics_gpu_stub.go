//go:build !cgo || (!darwin && (!linux || !cuda))

package sensorium

import "fmt"

func unsupportedPhysics() error {
	return fmt.Errorf("sensorium: physics requires a native Metal or CUDA engine; CPU reference is tests-only")
}
func (e *Engine) DepositDual(x, v, m, q, u, flags *Buffer, n int, p hydroParameters) error {
	return unsupportedPhysics()
}
func (e *Engine) GatherDual(x, m, xo, vo, qo, u, flags *Buffer, n int, p hydroParameters) error {
	return unsupportedPhysics()
}
func (e *Engine) ExportDual(u, rho, mom, thermal, flags *Buffer, p hydroParameters) error {
	return unsupportedPhysics()
}
func (e *Engine) HydroAdvance(u, out, w1, w2, flags, g *Buffer, p hydroParameters) error {
	return unsupportedPhysics()
}
func (e *Engine) HydroRates(u, g, diag, flags *Buffer, p hydroParameters) error {
	return unsupportedPhysics()
}
func (e *Engine) Poisson(u, spectrum, phi, g, flags *Buffer, p hydroParameters, G float32) error {
	return unsupportedPhysics()
}
func (e *Engine) GravityKick(u, g, flags *Buffer, p hydroParameters) error {
	return unsupportedPhysics()
}
func (e *Engine) ReadWaveLedger(out *Buffer, n int) error { return unsupportedPhysics() }

func (e *Engine) HydroBudget(initial, stage, out, flags *Buffer, p hydroParameters) error {
	return fmt.Errorf("native physics backend unavailable")
}

func (e *Engine) ReadPhaseLedger(out *Buffer, n int) error {
	return fmt.Errorf("sensorium: phase ledger requires a native GPU backend")
}

func (e *Engine) DepositMaterial(x, v, m, q, total, u, flags *Buffer, n int, p hydroParameters) error {
	return unsupportedPhysics()
}
func (e *Engine) RemapConservative(u, x, m, vo, qo, eo, report, flags *Buffer, n int, p hydroParameters, width, tolerance float32, iterations int, gravityG float32) error {
	return unsupportedPhysics()
}

func (e *Engine) PilotChecked(x, m, prior, re, im, xo, guide, report, flags *Buffer, n int, p hydroParameters, hbar, tolerance, maxcells float32) error {
	return fmt.Errorf("node-aware pilot: native backend unavailable")
}

func (e *Engine) ContactHash(x, v, m, q, vo, qo, report, flags *Buffer, p hydroParameters, c contactParameters, rates bool) error {
	return fmt.Errorf("Hertz hash kick: native backend unavailable")
}

func (e *Engine) GravityCompatible(initial, stage, state, phi0, phi1, kick, out, flags *Buffer, p hydroParameters) error {
	return fmt.Errorf("compatible gravity work: native backend unavailable")
}

func (e *Engine) ReciprocalHead(x, prior, amp, omega, modeOmega, linewidth, indices, weights, oldRe, oldIm, ledger, force, potential, flags *Buffer, n, modes, anchors int, sigma, domega float32, p hydroParameters) error {
	return fmt.Errorf("sensorium: reciprocal coherence requires a native backend")
}

func (e *Engine) PilotCheckedTime(x, m, prior, re0, im0, re1, im1, xo, guide, report, flags *Buffer, n int, p hydroParameters, hbar, tolerance, maxcells float32) error {
	return fmt.Errorf("sensorium: space-time pilot requires a native backend")
}
