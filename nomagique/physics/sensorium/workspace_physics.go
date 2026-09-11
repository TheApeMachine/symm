package sensorium

import (
	"fmt"
	"math"
)

// hydroParameters mirrors the scalar 64-byte physics-v2 POD. Native wrappers
// construct its C counterpart explicitly, so Go/C alignment is not presumed.
type hydroParameters struct {
	N, NX, NY, NZ                                             uint32
	DX, DT, Gamma, CV, Mu, Bulk, K, EtaPressure, EtaSync, CFL float32
	Reconstruction, Gravity                                   uint32
}

func (fluid *workspace) hydroParams(dt float32) hydroParameters {
	d := fluid.domain
	p := fluid.physics
	return hydroParameters{uint32(d.CellCount()), uint32(d.GridX), uint32(d.GridY), uint32(d.GridZ),
		float32(d.GridSpacing()), dt, float32(d.Gamma), float32(d.CV), float32(d.Mu), 0, float32(d.KThermal),
		float32(p.EtaPressure), float32(p.EtaSync), float32(p.CFL), 1, 0}
}

func (fluid *workspace) snapshotPhysics() func() {
	fluid.engine.Synchronize()
	buffers := fluid.allBuffers()
	saved := make([][]uint32, len(buffers))
	for i, b := range buffers {
		if b != nil {
			saved[i] = append([]uint32(nil), b.UInt32Slice()...)
		}
	}
	waveReady := fluid.waveProjectionReady
	rng := fluid.rngSeed
	health := fluid.health
	clock := fluid.physicalTime
	phaseRate := fluid.lastPhaseRate
	potential := append([]float32(nil), fluid.previousPotential...)
	return func() {
		fluid.engine.Synchronize()
		for i, b := range buffers {
			if b != nil {
				copy(b.UInt32Slice(), saved[i])
			}
		}
		fluid.waveProjectionReady = waveReady
		fluid.rngSeed = rng
		fluid.health = health
		fluid.physicalTime = clock
		fluid.lastPhaseRate = phaseRate
		copy(fluid.previousPotential, potential)
	}
}
func (fluid *workspace) validateInputs() error {
	if err := fluid.physics.validate(); err != nil {
		return err
	}
	d := fluid.domain
	if !isPositiveFinite(d.CV) || !isPositiveFinite(d.Gamma-1) || !finite(d.Mu) || d.Mu < 0 || !finite(d.KThermal) || d.KThermal < 0 ||
		math.Abs(d.RSpecific-(d.Gamma-1)*d.CV) > 16*math.Ldexp(1, -23)*math.Max(1, math.Abs(d.RSpecific)) {
		return fmt.Errorf("inconsistent ideal-gas/material model")
	}
	if fluid.particles == 0 {
		return nil
	}
	m, q, e := fluid.mass.Float32Slice(), fluid.heat.Float32Slice(), fluid.oscEnergy.Float32Slice()
	pos, vel := fluid.pos.Float32Slice(), fluid.vel.Float32Slice()
	phase, omega := fluid.phase.Float32Slice(), fluid.omega.Float32Slice()
	total, priorPosition := fluid.materialEnergy.Float32Slice(), fluid.coherencePosition.Float32Slice()
	pilot, prior := fluid.pilotPrevious.Float32Slice(), fluid.phasePrior.Float32Slice()
	for i := 0; i < fluid.particles; i++ {
		if !isPositiveFinite(float64(m[i])) || !finite(float64(q[i])) || q[i] < 0 || !finite(float64(e[i])) || e[i] < 0 ||
			!finite(float64(phase[i])) || !finite(float64(omega[i])) || !finite(float64(prior[i])) || !finite(float64(total[i])) || total[i] < 0 {
			return &CoupledStepError{"input", i, false, fmt.Sprintf("m=%g Q=%g Eosc=%g phase=%g omega=%g", m[i], q[i], e[i], phase[i], omega[i])}
		}
		for a := 0; a < 3; a++ {
			if !finite(float64(pos[3*i+a])) || !finite(float64(vel[3*i+a])) || !finite(float64(pilot[3*i+a])) || !finite(float64(priorPosition[3*i+a])) {
				return &CoupledStepError{"input", i, false, "nonfinite position/velocity"}
			}
		}
	}
	return nil
}
func checkFlags(label string, b *Buffer, n int) error {
	for i, s := range b.UInt32Slice()[:n] {
		if s != 0 {
			return &CoupledStepError{label, i, s == 3 || s == 4, fmt.Sprintf("physics status=%d", s)}
		}
	}
	return nil
}
func (fluid *workspace) depositDual() error {
	fluid.engine.Synchronize()
	fluid.hydro.Zero()
	if fluid.particles > 0 {
		if err := fluid.engine.DepositMaterial(fluid.pos, fluid.vel, fluid.mass, fluid.heat, fluid.materialEnergy, fluid.hydro, fluid.particleStatus, fluid.particles, fluid.hydroParams(1)); err != nil {
			return err
		}
		if err := checkFlags("PIC deposit", fluid.particleStatus, fluid.particles); err != nil {
			return err
		}
	}
	fluid.health.Sources.PICDepositEnergyResidual += sumHydroEnergy(fluid.hydro.Float32Slice(), fluid.domain.GridSpacing()) - fluid.materialTotal()
	return fluid.exportDual()
}
func (fluid *workspace) exportDual() error {
	if err := fluid.engine.ExportDual(fluid.hydro, fluid.rho, fluid.mom, fluid.energy, fluid.hydroStatus, fluid.hydroParams(1)); err != nil {
		return err
	}
	return checkFlags("hydro primitive export", fluid.hydroStatus, fluid.domain.CellCount())
}
func diagnosticBound(rate, factor float64) float64 {
	if rate == 0 {
		return 0
	}
	return factor / rate
}
func (fluid *workspace) stabilityLimit() (float64, error) {
	if err := fluid.validateInputs(); err != nil {
		return 0, err
	}
	if err := fluid.depositDual(); err != nil {
		return 0, err
	}
	if err := fluid.engine.HydroRates(fluid.hydro, fluid.acceleration, fluid.hydroDiagnostics, fluid.hydroStatus, fluid.hydroParams(1)); err != nil {
		return 0, err
	}
	if err := checkFlags("hydro stability", fluid.hydroStatus, fluid.domain.CellCount()); err != nil {
		return 0, err
	}
	d := fluid.domain
	dx := d.GridSpacing()
	rho, mom, e := fluid.rho.Float32Slice(), fluid.mom.Float32Slice(), fluid.energy.Float32Slice()
	diag := fluid.hydroDiagnostics.Float32Slice()
	maxRate, hyp, visc, thermal := 0.0, 0.0, 0.0, 0.0
	for c := 0; c < d.CellCount(); c++ {
		maxRate = math.Max(maxRate, float64(diag[8*c]))
		r := float64(rho[c])
		if r == 0 {
			continue
		}
		sound := math.Sqrt(d.Gamma * (d.Gamma - 1) * float64(e[c]) / r)
		rate := 0.0
		for a := 0; a < 3; a++ {
			rate += (math.Abs(float64(mom[3*c+a])/r) + sound) / dx
		}
		hyp = math.Max(hyp, rate)
		// These use the same conservative coefficient family as the shared RHS.
		visc = math.Max(visc, 24*d.Mu/(r*dx*dx))
		thermal = math.Max(thermal, 12*d.KThermal/(r*d.CV*dx*dx))
	}
	speed := 0.0
	if fluid.particles > 0 {
		v := fluid.vel.Float32Slice()
		for i := 0; i < fluid.particles; i++ {
			s := 0.0
			for a := 0; a < 3; a++ {
				s += float64(v[3*i+a]) * float64(v[3*i+a])
			}
			speed = math.Max(speed, math.Sqrt(s))
		}
	}
	phaseRate := fluid.lastPhaseRate
	if fluid.particles > 0 {
		for _, w := range fluid.omega.Float32Slice()[:fluid.particles] {
			phaseRate = math.Max(phaseRate, math.Abs(float64(w)))
		}
	}
	p := fluid.physics
	h := &fluid.health.Integrator
	h.HyperbolicDT = diagnosticBound(hyp, p.CFL)
	h.ViscousDT = diagnosticBound(visc, p.CFL)
	h.ThermalDT = diagnosticBound(thermal, p.CFL)
	h.ParticleDT = diagnosticBound(speed, p.ParticleCells*dx)
	h.PhaseDT = diagnosticBound(phaseRate, p.PhaseRadians)
	h.CombinedDT = diagnosticBound(maxRate, p.CFL)
	bound := p.MaxStep
	if phaseRate > 0 {
		bound = math.Min(bound, p.PhaseRadians/phaseRate)
	}
	if maxRate > 0 {
		bound = math.Min(bound, p.CFL/maxRate)
	}
	if speed > 0 {
		bound = math.Min(bound, p.ParticleCells*dx/speed)
	}
	if fluid.physics.Contacts.Enabled {
		limit, err := fluid.contactRates()
		if err != nil {
			return 0, err
		}
		bound = math.Min(bound, limit)
		fluid.health.Integrator.ContactDT = limit
	}
	return bound, nil
}
func (fluid *workspace) gasDual(dt float32) error {
	gravityBefore := fluid.health.Sources.GravityWork
	fieldBefore := 0.0
	before := sumHydroEnergy(fluid.hydro.Float32Slice(), fluid.domain.GridSpacing())
	if fluid.physics.GravityG > 0 {
		fluid.gravityKickWork.Zero()
		priorField := fluid.health.Sources.GravityFieldEnergy
		if err := fluid.gravityKick(.5 * dt); err != nil {
			return err
		}
		fieldBefore = fluid.health.Sources.GravityFieldEnergy
		copy(fluid.gravityPrior.UInt32Slice(), fluid.gravity.UInt32Slice())
		copy(fluid.gravityFluxBase.UInt32Slice(), fluid.hydro.UInt32Slice())
		// Rebuilding after particle transport/remapping or exogenous changes is
		// recorded separately from the self-gravity hydro bracket residual.
		fluid.health.Sources.GravityRebuildChange += fieldBefore - priorField
	}
	if err := fluid.engine.HydroAdvance(fluid.hydro, fluid.hydroOut, fluid.hydroWork1, fluid.hydroWork2, fluid.hydroStatus, fluid.acceleration, fluid.hydroParams(dt)); err != nil {
		return err
	}
	if err := checkFlags("gas RK2", fluid.hydroStatus, fluid.domain.CellCount()); err != nil {
		return err
	}
	if err := fluid.engine.HydroBudget(fluid.hydro, fluid.hydroWork1, fluid.hydroDiagnostics, fluid.hydroStatus, fluid.hydroParams(dt)); err != nil {
		return err
	}
	budget := fluid.hydroDiagnostics.Float32Slice()
	vol := math.Pow(fluid.domain.GridSpacing(), 3)
	for i := 0; i < fluid.domain.CellCount(); i++ {
		fluid.health.Sources.ViscousToHeat += float64(budget[2*i]) * vol
		fluid.health.Sources.ThermalConductionNet += float64(budget[2*i+1]) * vol
	}
	copy(fluid.hydro.UInt32Slice(), fluid.hydroOut.UInt32Slice())
	if fluid.physics.GravityG > 0 {
		if err := fluid.gravityKick(.5 * dt); err != nil {
			return err
		}
		beforeCompatible := sumHydroEnergy(fluid.hydro.Float32Slice(), fluid.domain.GridSpacing())
		if err := fluid.engine.GravityCompatible(fluid.gravityFluxBase, fluid.hydroWork1, fluid.hydro, fluid.gravityPrior, fluid.gravity, fluid.gravityKickWork, fluid.hydroOut, fluid.hydroStatus, fluid.hydroParams(dt)); err != nil {
			return err
		}
		copy(fluid.hydro.UInt32Slice(), fluid.hydroOut.UInt32Slice())
		fluid.health.Sources.GravityWork += sumHydroEnergy(fluid.hydro.Float32Slice(), fluid.domain.GridSpacing()) - beforeCompatible
	}
	after := sumHydroEnergy(fluid.hydro.Float32Slice(), fluid.domain.GridSpacing())
	fluid.health.Sources.GasEnergyResidual += after - before - (fluid.health.Sources.GravityWork - gravityBefore)
	if fluid.physics.GravityG > 0 {
		fluid.health.Sources.GravityBalanceResidual += fluid.health.Sources.GravityWork - gravityBefore + fluid.health.Sources.GravityFieldEnergy - fieldBefore
	}
	return fluid.exportDual()
}
func sumHydroEnergy(u []float32, dx float64) float64 {
	s := 0.0
	for i := 4; i < len(u); i += 6 {
		s += float64(u[i])
	}
	return s * dx * dx * dx
}
func (fluid *workspace) particleTotals() (mass, thermal, osc, kinetic float64, momentum [3]float64) {
	if fluid.particles == 0 {
		return
	}
	m, q, e, v := fluid.mass.Float32Slice(), fluid.heat.Float32Slice(), fluid.oscEnergy.Float32Slice(), fluid.vel.Float32Slice()
	for i := 0; i < fluid.particles; i++ {
		mi := float64(m[i])
		mass += mi
		thermal += float64(q[i])
		osc += float64(e[i])
		for a := 0; a < 3; a++ {
			vi := float64(v[3*i+a])
			kinetic += .5 * mi * vi * vi
			momentum[a] += mi * vi
		}
	}
	return
}
func (fluid *workspace) gatherDual(dt float32) error {
	if fluid.particles == 0 {
		return nil
	}
	if err := fluid.engine.GatherDual(fluid.pos, fluid.mass, fluid.posOut, fluid.velOut, fluid.heatOut, fluid.hydro, fluid.particleStatus, fluid.particles, fluid.hydroParams(dt)); err != nil {
		return err
	}
	if err := checkFlags("PIC gather", fluid.particleStatus, fluid.particles); err != nil {
		return err
	}
	if err := fluid.engine.RemapConservative(fluid.hydro, fluid.posOut, fluid.mass, fluid.velOut, fluid.heatOut, fluid.materialEnergyOut, fluid.remapReport, fluid.particleStatus, fluid.particles, fluid.hydroParams(dt), float32(fluid.physics.RemapWidthCells), float32(fluid.physics.RemapTolerance), fluid.physics.RemapIterations, float32(fluid.physics.GravityG)); err != nil {
		return err
	}
	report := fluid.remapReport.Float32Slice()
	fluid.health.Remap = RemapHealth{MaxMarginalResidual: float64(report[0]), Iterations: int(report[1]), MassRoundoffScale: float64(report[2]), MixingToAuxiliary: float64(report[3]), EnergyResidual: float64(report[4]), MomentumResidual: [3]float64{float64(report[5]), float64(report[6]), float64(report[7])}, WidthCells: fluid.physics.RemapWidthCells}
	fluid.health.Sources.RemapMixingToAuxiliary += float64(report[3])
	if fluid.physics.GravityG > 0 {
		fluid.health.Sources.GravityRemapWork += float64(report[8])
		fluid.health.Sources.GravityFieldEnergy = float64(report[10])
		fluid.health.Sources.GravityRemapResidual += float64(report[11])
	}
	n := fluid.particles
	// Material grid state is an intermediate representation. Its remap error is
	// exposed as NUMERICAL discrepancy, never booked as an external source.
	gridMass, gridEnergy := 0.0, 0.0
	var gridP [3]float64
	u := fluid.hydro.Float32Slice()
	vol := math.Pow(fluid.domain.GridSpacing(), 3)
	for i := 0; i < len(u); i += 6 {
		gridMass += float64(u[i]) * vol
		gridEnergy += float64(u[i+4]) * vol
		for a := 0; a < 3; a++ {
			gridP[a] += float64(u[i+1+a]) * vol
		}
	}
	copy(fluid.pos.Float32Slice()[:3*n], fluid.posOut.Float32Slice()[:3*n])
	copy(fluid.vel.Float32Slice()[:3*n], fluid.velOut.Float32Slice()[:3*n])
	copy(fluid.heat.Float32Slice()[:n], fluid.heatOut.Float32Slice()[:n])
	copy(fluid.materialEnergy.Float32Slice()[:n], fluid.materialEnergyOut.Float32Slice()[:n])
	mass, _, _, _, mom := fluid.particleTotals()
	s := &fluid.health.Sources
	s.PICRemapMass += mass - gridMass
	s.PICRemapEnergy += fluid.materialTotal() - gridEnergy - float64(report[8])
	for a := 0; a < 3; a++ {
		s.PICRemapMomentum[a] += mom[a] - gridP[a]
	}
	return nil
}

// Gravity uses a solved periodic mean-subtracted Poisson field. Kicks update
// total energy by the EXACT kinetic-energy increment and leave auxiliary heat
// unchanged. Two kicks bracket hydro; no hidden forcing in PIC gather.
func (fluid *workspace) gravityKick(dt float32) error {
	p := fluid.hydroParams(dt)
	if err := fluid.engine.Poisson(fluid.hydro, fluid.poissonState, fluid.gravity, fluid.acceleration, fluid.hydroStatus, p, float32(fluid.physics.GravityG)); err != nil {
		return err
	}
	if err := checkFlags("Poisson", fluid.hydroStatus, fluid.domain.CellCount()); err != nil {
		return err
	}
	u, g := fluid.hydro.Float32Slice(), fluid.acceleration.Float32Slice()
	phi := fluid.gravity.Float32Slice()
	fieldEnergy := 0.0
	d := fluid.domain
	vol := math.Pow(d.GridSpacing(), 3)
	for x := 0; x < d.GridX; x++ {
		for y := 0; y < d.GridY; y++ {
			for z := 0; z < d.GridZ; z++ {
				i := z + d.GridZ*(y+d.GridY*x)
				j := x + d.GridX*(y+d.GridY*z)
				fieldEnergy += .5 * vol * float64(u[6*i]) * float64(phi[j])
			}
		}
	}
	fluid.health.Sources.GravityFieldEnergy = fieldEnergy
	rate := 0.0
	for i := 0; i < fluid.domain.CellCount(); i++ {
		a2 := 0.0
		for a := 0; a < 3; a++ {
			a2 += float64(g[3*i+a]) * float64(g[3*i+a])
		}
		rate = math.Max(rate, math.Sqrt(a2))
	}
	if .5*rate*float64(2*dt)*float64(2*dt) > fluid.physics.ParticleCells*fluid.domain.GridSpacing() {
		return &CoupledStepError{"gravity displacement", -1, true, "gravity kick exceeds cell displacement criterion"}
	}
	before := sumHydroEnergy(u, fluid.domain.GridSpacing())
	old := make([]float32, fluid.domain.CellCount())
	for i := range old {
		old[i] = u[6*i+4]
	}
	if err := fluid.engine.GravityKick(fluid.hydro, fluid.acceleration, fluid.hydroStatus, p); err != nil {
		return err
	}
	if err := checkFlags("gravity kick", fluid.hydroStatus, fluid.domain.CellCount()); err != nil {
		return err
	}
	fluid.health.Sources.GravityWork += sumHydroEnergy(u, fluid.domain.GridSpacing()) - before
	work := fluid.gravityKickWork.Float32Slice()
	for i := range old {
		work[i] += u[6*i+4] - old[i]
	}
	return nil
}

// SetPhysicsControls changes only explicit policy/model constants. It is not a
// backend selection or a CPU fallback. Control changes happen under the owner lock.
func (manifold *Manifold) SetPhysicsControls(p PhysicsControls) error {
	if err := p.validate(); err != nil {
		return err
	}
	if manifold == nil {
		return fmt.Errorf("sensorium: nil manifold")
	}
	manifold.mu.Lock()
	defer manifold.mu.Unlock()
	if manifold.work == nil {
		return fmt.Errorf("sensorium: closed manifold")
	}
	if manifold.work.physicalTime != 0 && (p.Units != manifold.work.physics.Units || p.GravityG != manifold.work.physics.GravityG || p.Contacts != manifold.work.physics.Contacts) {
		return fmt.Errorf("physical units/gravity coupling cannot be changed during an evolved trajectory without an explicit parameter-work operator")
	}
	manifold.work.physics = p
	return nil
}

func (fluid *workspace) materialTotal() float64 {
	if fluid.particles == 0 {
		return 0
	}
	sum := 0.
	for _, v := range fluid.materialEnergy.Float32Slice()[:fluid.particles] {
		sum += float64(v)
	}
	return sum
}

// A physical source changes conservative material energy and, separately, its
// auxiliary reservoir. The float32 representation residual is always recorded.
func (fluid *workspace) materialWork(i int, work float64) error {
	data := fluid.materialEnergy.Float32Slice()
	old := float64(data[i])
	next := float32(old + work)
	if !finite(float64(next)) || next < 0 {
		return &CoupledStepError{"material energy source", i, true, fmt.Sprintf("E=%g work=%g", old, work)}
	}
	data[i] = next
	fluid.health.Sources.MaterialRoundoff += float64(next) - old - work
	return nil
}
