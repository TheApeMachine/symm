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
func (engine *Engine) coupledResult(name string, ok C.bool, flags *Buffer, n int) error {
	runtime.KeepAlive(engine)
	if bool(ok) {
		return nil
	}
	message := C.GoString(C.manifold_last_error(engine.ctx))
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
func (engine *Engine) DepositDual(x, v, m, q, u, flags *Buffer, n int, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_pic_deposit_dual(engine.ctx, x.cBuf, v.cBuf, m.cBuf, q.cBuf, u.cBuf, flags.cBuf, C.uint(n), &c)
	return engine.coupledResult("dual PIC deposit", ok, flags, n)
}
func (engine *Engine) GatherDual(x, m, xo, vo, qo, u, flags *Buffer, n int, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_pic_gather_dual(engine.ctx, x.cBuf, m.cBuf, xo.cBuf, vo.cBuf, qo.cBuf, u.cBuf, flags.cBuf, C.uint(n), &c)
	return engine.coupledResult("dual PIC gather", ok, flags, n)
}
func (engine *Engine) ExportDual(u, rho, mom, thermal, flags *Buffer, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_hydro_export_dual(engine.ctx, u.cBuf, rho.cBuf, mom.cBuf, thermal.cBuf, flags.cBuf, &c)
	return engine.coupledResult("dual export", ok, flags, int(p.N))
}
func (engine *Engine) HydroAdvance(u, out, w1, w2, flags, g *Buffer, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_hydro_step_v2(engine.ctx, u.cBuf, out.cBuf, w1.cBuf, w2.cBuf, flags.cBuf, g.cBuf, &c)
	return engine.coupledResult("gas RK2", ok, flags, int(p.N))
}
func (engine *Engine) HydroRates(u, g, diag, flags *Buffer, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_hydro_diagnostics_v2(engine.ctx, u.cBuf, g.cBuf, diag.cBuf, flags.cBuf, &c)
	return engine.coupledResult("gas diagnostics", ok, flags, int(p.N))
}
func (engine *Engine) Poisson(u, spectrum, phi, g, flags *Buffer, p hydroParameters, G float32) error {
	c := cHydro(p)
	ok := C.manifold_periodic_poisson(engine.ctx, u.cBuf, spectrum.cBuf, phi.cBuf, g.cBuf, flags.cBuf, &c, C.float(G))
	return engine.coupledResult("periodic gravity", ok, flags, int(p.N))
}
func (engine *Engine) GravityKick(u, g, flags *Buffer, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_gravity_kick(engine.ctx, u.cBuf, g.cBuf, flags.cBuf, &c)
	return engine.coupledResult("gravity kick", ok, flags, int(p.N))
}
func (engine *Engine) ReadWaveLedger(out *Buffer, n int) error {
	ok := C.manifold_coherence_copy_ledger(engine.ctx, out.cBuf, C.uint(n))
	return engine.coupledResult("coherence ledger", ok, nil, 0)
}

func (engine *Engine) HydroBudget(initial, stage, out, flags *Buffer, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_hydro_budget(engine.ctx, initial.cBuf, stage.cBuf, out.cBuf, flags.cBuf, &c)
	return engine.coupledResult("hydro work ledger", ok, flags, int(p.N))
}

func (engine *Engine) ReadPhaseLedger(out *Buffer, n int) error {
	ok := C.manifold_phase_copy_ledger(engine.ctx, out.cBuf, C.uint(n))
	return engine.coupledResult("phase ledger", ok, nil, 0)
}

func (engine *Engine) DepositMaterial(x, v, m, q, total, u, flags *Buffer, n int, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_pic_deposit_material(engine.ctx, x.cBuf, v.cBuf, m.cBuf, q.cBuf, total.cBuf, u.cBuf, flags.cBuf, C.uint(n), &c)
	return engine.coupledResult("material PIC deposit", ok, flags, n)
}
func (engine *Engine) RemapConservative(u, x, m, vo, qo, eo, report, flags *Buffer, n int, p hydroParameters, width, tolerance float32, iterations int, gravityG float32) error {
	c := cHydro(p)
	ok := C.manifold_pic_remap_conservative(engine.ctx, u.cBuf, x.cBuf, m.cBuf, vo.cBuf, qo.cBuf, eo.cBuf, report.cBuf, flags.cBuf, C.uint(n), &c, C.float(width), C.float(tolerance), C.uint(iterations), C.float(gravityG))
	return engine.coupledResult("conservative PIC remap", ok, flags, n)
}

func (engine *Engine) PilotChecked(x, m, prior, re, im, xo, guide, report, flags *Buffer, n int, p hydroParameters, hbar, tolerance, maxcells float32) error {
	c := cHydro(p)
	ok := C.manifold_pilot_checked(engine.ctx, x.cBuf, m.cBuf, prior.cBuf, re.cBuf, im.cBuf, xo.cBuf, guide.cBuf, report.cBuf, flags.cBuf, C.uint(n), &c, C.float(hbar), C.float(tolerance), C.float(maxcells))
	return engine.coupledResult("node-aware pilot", ok, flags, n)
}

func (engine *Engine) ContactHash(x, v, m, q, vo, qo, report, flags *Buffer, p hydroParameters, c contactParameters, rates bool) error {
	h := cHydro(p)
	cp := C.MFContactParamsV2{n: C.uint(c.N), dt: C.float(c.DT), radius: C.float(c.Radius), young_modulus: C.float(c.Young), poisson_ratio: C.float(c.Poisson), normal_relaxation_time: C.float(c.Relaxation), conductivity: C.float(c.Conductivity), cv: C.float(c.CV), domain_x: C.float(c.X), domain_y: C.float(c.Y), domain_z: C.float(c.Z)}
	var mode C.uint
	if rates {
		mode = 1
	}
	ok := C.manifold_contact_hash_kick(engine.ctx, x.cBuf, v.cBuf, m.cBuf, q.cBuf, vo.cBuf, qo.cBuf, report.cBuf, flags.cBuf, &h, &cp, mode)
	return engine.coupledResult("Hertz hash kick", ok, flags, int(c.N))
}

func (engine *Engine) GravityCompatible(initial, stage, state, phi0, phi1, kick, out, flags *Buffer, p hydroParameters) error {
	h := cHydro(p)
	ok := C.manifold_gravity_compatible(engine.ctx, initial.cBuf, stage.cBuf, state.cBuf, phi0.cBuf, phi1.cBuf, kick.cBuf, out.cBuf, flags.cBuf, &h)
	return engine.coupledResult("compatible gravity work", ok, flags, int(p.N))
}

func (engine *Engine) ReciprocalHead(x, prior, amp, omega, modeOmega, linewidth, indices, weights, oldRe, oldIm, ledger, force, potential, flags *Buffer, n, modes, anchors int, sigma, domega float32, p hydroParameters) error {
	c := cHydro(p)
	ok := C.manifold_coherence_reciprocal(engine.ctx, x.cBuf, prior.cBuf, amp.cBuf, omega.cBuf, modeOmega.cBuf, linewidth.cBuf, indices.cBuf, weights.cBuf, oldRe.cBuf, oldIm.cBuf, ledger.cBuf, force.cBuf, potential.cBuf, flags.cBuf, C.uint(n), C.uint(modes), C.uint(anchors), C.float(sigma), C.float(domega), &c)
	return engine.coupledResult("reciprocal coherence", ok, flags, max(n, modes))
}

func (engine *Engine) PilotCheckedTime(x, m, prior, re0, im0, re1, im1, xo, guide, report, flags *Buffer, n int, p hydroParameters, hbar, tolerance, maxcells float32) error {
	c := cHydro(p)
	ok := C.manifold_pilot_checked_time(engine.ctx, x.cBuf, m.cBuf, prior.cBuf, re0.cBuf, im0.cBuf, re1.cBuf, im1.cBuf, xo.cBuf, guide.cBuf, report.cBuf, flags.cBuf, C.uint(n), &c, C.float(hbar), C.float(tolerance), C.float(maxcells))
	return engine.coupledResult("space-time pilot", ok, flags, n)
}
