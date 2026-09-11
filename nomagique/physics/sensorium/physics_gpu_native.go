//go:build cgo && (darwin || (linux && cuda))

package sensorium

/*
#include "bridge.h"
*/
import "C"
import (
	"fmt"
	"runtime"
)

func cHydro(p hydroParameters) C.MFHydroParamsV2 {
	return C.MFHydroParamsV2{n: C.uint(p.N), nx: C.uint(p.NX), ny: C.uint(p.NY), nz: C.uint(p.NZ), dx: C.float(p.DX), dt: C.float(p.DT), gamma: C.float(p.Gamma), cv: C.float(p.CV), mu: C.float(p.Mu), bulk_viscosity: C.float(p.Bulk), k_thermal: C.float(p.K), eta_pressure: C.float(p.EtaPressure), eta_sync: C.float(p.EtaSync), cfl: C.float(p.CFL), reconstruction: C.uint(p.Reconstruction), gravity: C.uint(p.Gravity)}
}
func (e *Engine) coupledResult(name string, ok C.bool, flags *Buffer, n int) error {
	runtime.KeepAlive(e)
	if bool(ok) {
		return nil
	}
	message := C.GoString(C.manifold_last_error(e.ctx))
	if message != "" {
		return fmt.Errorf("sensorium %s: %s", name, message)
	}
	if flags != nil {
		if err := checkFlags(name, flags, n); err != nil {
			return err
		}
	}
	return fmt.Errorf("sensorium %s: invalid native arguments or device failure", name)
}
func (e *Engine) DepositDual(x, v, m, q, u, flags *Buffer, n int, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_pic_deposit_dual(e.ctx, x.cBuf, v.cBuf, m.cBuf, q.cBuf, u.cBuf, flags.cBuf, C.uint(n), &c)
	return e.coupledResult("dual PIC deposit", ok, flags, n)
}
func (e *Engine) GatherDual(x, m, xo, vo, qo, u, flags *Buffer, n int, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_pic_gather_dual(e.ctx, x.cBuf, m.cBuf, xo.cBuf, vo.cBuf, qo.cBuf, u.cBuf, flags.cBuf, C.uint(n), &c)
	return e.coupledResult("dual PIC gather", ok, flags, n)
}
func (e *Engine) ExportDual(u, rho, mom, thermal, flags *Buffer, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_hydro_export_dual(e.ctx, u.cBuf, rho.cBuf, mom.cBuf, thermal.cBuf, flags.cBuf, &c)
	return e.coupledResult("dual export", ok, flags, int(p.N))
}
func (e *Engine) HydroAdvance(u, out, w1, w2, flags, g *Buffer, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_hydro_step_v2(e.ctx, u.cBuf, out.cBuf, w1.cBuf, w2.cBuf, flags.cBuf, g.cBuf, &c)
	return e.coupledResult("gas RK2", ok, flags, int(p.N))
}
func (e *Engine) HydroRates(u, g, diag, flags *Buffer, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_hydro_diagnostics_v2(e.ctx, u.cBuf, g.cBuf, diag.cBuf, flags.cBuf, &c)
	return e.coupledResult("gas diagnostics", ok, flags, int(p.N))
}
func (e *Engine) Poisson(u, spectrum, phi, g, flags *Buffer, p hydroParameters, G float32) error {
	c := cHydro(p)
	ok := C.manifold_periodic_poisson(e.ctx, u.cBuf, spectrum.cBuf, phi.cBuf, g.cBuf, flags.cBuf, &c, C.float(G))
	return e.coupledResult("periodic gravity", ok, flags, int(p.N))
}
func (e *Engine) GravityKick(u, g, flags *Buffer, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_gravity_kick(e.ctx, u.cBuf, g.cBuf, flags.cBuf, &c)
	return e.coupledResult("gravity kick", ok, flags, int(p.N))
}
func (e *Engine) ReadWaveLedger(out *Buffer, n int) error {
	ok := C.manifold_coherence_copy_ledger(e.ctx, out.cBuf, C.uint(n))
	return e.coupledResult("coherence ledger", ok, nil, 0)
}

func (e *Engine) HydroBudget(initial, stage, out, flags *Buffer, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_hydro_budget(e.ctx, initial.cBuf, stage.cBuf, out.cBuf, flags.cBuf, &c)
	return e.coupledResult("hydro work ledger", ok, flags, int(p.N))
}

func (e *Engine) ReadPhaseLedger(out *Buffer, n int) error {
	ok := C.manifold_phase_copy_ledger(e.ctx, out.cBuf, C.uint(n))
	return e.coupledResult("phase ledger", ok, nil, 0)
}

func (e *Engine) DepositMaterial(x, v, m, q, total, u, flags *Buffer, n int, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_pic_deposit_material(e.ctx, x.cBuf, v.cBuf, m.cBuf, q.cBuf, total.cBuf, u.cBuf, flags.cBuf, C.uint(n), &c)
	return e.coupledResult("material PIC deposit", ok, flags, n)
}
func (e *Engine) RemapConservative(u, x, m, vo, qo, eo, report, flags *Buffer, n int, p hydroParameters, width, tolerance float32, iterations int, gravityG float32) error {
	c := cHydro(p)
	ok := C.manifold_pic_remap_conservative(e.ctx, u.cBuf, x.cBuf, m.cBuf, vo.cBuf, qo.cBuf, eo.cBuf, report.cBuf, flags.cBuf, C.uint(n), &c, C.float(width), C.float(tolerance), C.uint(iterations), C.float(gravityG))
	return e.coupledResult("conservative PIC remap", ok, flags, n)
}

func (e *Engine) PilotChecked(x, m, prior, re, im, xo, guide, report, flags *Buffer, n int, p hydroParameters, hbar, tolerance, maxcells float32) error {
	c := cHydro(p)
	ok := C.manifold_pilot_checked(e.ctx, x.cBuf, m.cBuf, prior.cBuf, re.cBuf, im.cBuf, xo.cBuf, guide.cBuf, report.cBuf, flags.cBuf, C.uint(n), &c, C.float(hbar), C.float(tolerance), C.float(maxcells))
	return e.coupledResult("node-aware pilot", ok, flags, n)
}

func (e *Engine) ContactHash(x, v, m, q, vo, qo, report, flags *Buffer, p hydroParameters, c contactParameters, rates bool) error {
	h := cHydro(p)
	cp := C.MFContactParamsV2{n: C.uint(c.N), dt: C.float(c.DT), radius: C.float(c.Radius), young_modulus: C.float(c.Young), poisson_ratio: C.float(c.Poisson), normal_relaxation_time: C.float(c.Relaxation), conductivity: C.float(c.Conductivity), cv: C.float(c.CV), domain_x: C.float(c.X), domain_y: C.float(c.Y), domain_z: C.float(c.Z)}
	var mode C.uint
	if rates {
		mode = 1
	}
	ok := C.manifold_contact_hash_kick(e.ctx, x.cBuf, v.cBuf, m.cBuf, q.cBuf, vo.cBuf, qo.cBuf, report.cBuf, flags.cBuf, &h, &cp, mode)
	return e.coupledResult("Hertz hash kick", ok, flags, int(c.N))
}

func (e *Engine) GravityCompatible(initial, stage, state, phi0, phi1, kick, out, flags *Buffer, p hydroParameters) error {
	h := cHydro(p)
	ok := C.manifold_gravity_compatible(e.ctx, initial.cBuf, stage.cBuf, state.cBuf, phi0.cBuf, phi1.cBuf, kick.cBuf, out.cBuf, flags.cBuf, &h)
	return e.coupledResult("compatible gravity work", ok, flags, int(p.N))
}

func (e *Engine) ReciprocalHead(x, prior, amp, omega, modeOmega, linewidth, indices, weights, oldRe, oldIm, ledger, force, potential, flags *Buffer, n, modes, anchors int, sigma, domega float32, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_coherence_reciprocal(e.ctx, x.cBuf, prior.cBuf, amp.cBuf, omega.cBuf, modeOmega.cBuf, linewidth.cBuf, indices.cBuf, weights.cBuf, oldRe.cBuf, oldIm.cBuf, ledger.cBuf, force.cBuf, potential.cBuf, flags.cBuf, C.uint(n), C.uint(modes), C.uint(anchors), C.float(sigma), C.float(domega), &c)
	return e.coupledResult("reciprocal coherence", ok, flags, max(n, modes))
}

func (e *Engine) PilotCheckedTime(x, m, prior, re0, im0, re1, im1, xo, guide, report, flags *Buffer, n int, p hydroParameters, hbar, tolerance, maxcells float32) error {
	c := cHydro(p)
	ok := C.manifold_pilot_checked_time(e.ctx, x.cBuf, m.cBuf, prior.cBuf, re0.cBuf, im0.cBuf, re1.cBuf, im1.cBuf, xo.cBuf, guide.cBuf, report.cBuf, flags.cBuf, C.uint(n), &c, C.float(hbar), C.float(tolerance), C.float(maxcells))
	return e.coupledResult("space-time pilot", ok, flags, n)
}
