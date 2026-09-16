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

func (workspace *workspace) hydroParams(dt float32) hydroParameters {
	d := workspace.domain
	p := workspace.physics
	return hydroParameters{uint32(d.CellCount()), uint32(d.GridX), uint32(d.GridY), uint32(d.GridZ),
		float32(d.GridSpacing()), dt, float32(d.Gamma), float32(d.CV), float32(d.Mu), 0, float32(d.KThermal),
		float32(p.EtaPressure), float32(p.EtaSync), float32(p.CFL), 1, 0}
}

func (workspace *workspace) snapshotPhysics() func() {
	workspace.engine.Synchronize()
	buffers := workspace.allBuffers()
	saved := make([][]uint32, len(buffers))
	for i, b := range buffers {
		if b != nil {
			saved[i] = append([]uint32(nil), b.UInt32Slice()...)
		}
	}
	waveReady := workspace.waveProjectionReady
	rng := workspace.rngSeed
	health := workspace.health
	clock := workspace.physicalTime
	phaseRate := workspace.lastPhaseRate
	potential := append([]float32(nil), workspace.previousPotential...)
	return func() {
		workspace.engine.Synchronize()
		for i, b := range buffers {
			if b != nil {
				copy(b.UInt32Slice(), saved[i])
			}
		}
		workspace.waveProjectionReady = waveReady
		workspace.rngSeed = rng
		workspace.health = health
		workspace.physicalTime = clock
		workspace.lastPhaseRate = phaseRate
		copy(workspace.previousPotential, potential)
	}
}
func (workspace *workspace) validateInputs() error {
	if err := workspace.physics.validate(); err != nil {
		return err
	}
	d := workspace.domain
	if !isPositiveFinite(d.CV) || !isPositiveFinite(d.Gamma-1) || !finite(d.Mu) || d.Mu < 0 || !finite(d.KThermal) || d.KThermal < 0 ||
		math.Abs(d.RSpecific-(d.Gamma-1)*d.CV) > 16*math.Ldexp(1, -23)*math.Max(1, math.Abs(d.RSpecific)) {
		return fmt.Errorf("inconsistent ideal-gas/material model")
	}
	if workspace.particles == 0 {
		return nil
	}
	m, q, e := workspace.mass.Float32Slice(), workspace.heat.Float32Slice(), workspace.oscEnergy.Float32Slice()
	pos, vel := workspace.pos.Float32Slice(), workspace.vel.Float32Slice()
	phase, omega := workspace.phase.Float32Slice(), workspace.omega.Float32Slice()
	total, priorPosition := workspace.materialEnergy.Float32Slice(), workspace.coherencePosition.Float32Slice()
	pilot, prior := workspace.pilotPrevious.Float32Slice(), workspace.phasePrior.Float32Slice()
	for i := 0; i < workspace.particles; i++ {
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
func (workspace *workspace) depositDual() error {
	workspace.engine.Synchronize()
	workspace.hydro.Zero()
	if workspace.particles > 0 {
		if err := workspace.engine.DepositMaterial(workspace.pos, workspace.vel, workspace.mass, workspace.heat, workspace.materialEnergy, workspace.hydro, workspace.particleStatus, workspace.particles, workspace.hydroParams(1)); err != nil {
			return err
		}
		if err := checkFlags("PIC deposit", workspace.particleStatus, workspace.particles); err != nil {
			return err
		}
	}
	workspace.health.Sources.PICDepositEnergyResidual += sumHydroEnergy(workspace.hydro.Float32Slice(), workspace.domain.GridSpacing()) - workspace.materialTotal()
	return workspace.exportDual()
}
func (workspace *workspace) exportDual() error {
	if err := workspace.engine.ExportDual(workspace.hydro, workspace.rho, workspace.mom, workspace.energy, workspace.hydroStatus, workspace.hydroParams(1)); err != nil {
		return err
	}
	return checkFlags("hydro primitive export", workspace.hydroStatus, workspace.domain.CellCount())
}
func diagnosticBound(rate, factor float64) float64 {
	if rate == 0 {
		return 0
	}
	return factor / rate
}
func (workspace *workspace) stabilityLimit() (float64, error) {
	if err := workspace.validateInputs(); err != nil {
		return 0, err
	}
	if err := workspace.depositDual(); err != nil {
		return 0, err
	}
	if err := workspace.engine.HydroRates(workspace.hydro, workspace.acceleration, workspace.hydroDiagnostics, workspace.hydroStatus, workspace.hydroParams(1)); err != nil {
		return 0, err
	}
	if err := checkFlags("hydro stability", workspace.hydroStatus, workspace.domain.CellCount()); err != nil {
		return 0, err
	}
	d := workspace.domain
	dx := d.GridSpacing()
	rho, mom, e := workspace.rho.Float32Slice(), workspace.mom.Float32Slice(), workspace.energy.Float32Slice()
	diag := workspace.hydroDiagnostics.Float32Slice()
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
	if workspace.particles > 0 {
		v := workspace.vel.Float32Slice()
		for i := 0; i < workspace.particles; i++ {
			s := 0.0
			for a := 0; a < 3; a++ {
				s += float64(v[3*i+a]) * float64(v[3*i+a])
			}
			speed = math.Max(speed, math.Sqrt(s))
		}
	}
	phaseRate := workspace.lastPhaseRate
	if workspace.particles > 0 {
		for _, w := range workspace.omega.Float32Slice()[:workspace.particles] {
			phaseRate = math.Max(phaseRate, math.Abs(float64(w)))
		}
	}
	p := workspace.physics
	h := &workspace.health.Integrator
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
	if workspace.physics.Contacts.Enabled {
		limit, err := workspace.contactRates()
		if err != nil {
			return 0, err
		}
		bound = math.Min(bound, limit)
		workspace.health.Integrator.ContactDT = limit
	}
	return bound, nil
}
func (workspace *workspace) gasDual(dt float32) error {
	gravityBefore := workspace.health.Sources.GravityWork
	fieldBefore := 0.0
	before := sumHydroEnergy(workspace.hydro.Float32Slice(), workspace.domain.GridSpacing())
	if workspace.physics.GravityG > 0 {
		workspace.gravityKickWork.Zero()
		priorField := workspace.health.Sources.GravityFieldEnergy
		if err := workspace.gravityKick(.5 * dt); err != nil {
			return err
		}
		fieldBefore = workspace.health.Sources.GravityFieldEnergy
		copy(workspace.gravityPrior.UInt32Slice(), workspace.gravity.UInt32Slice())
		copy(workspace.gravityFluxBase.UInt32Slice(), workspace.hydro.UInt32Slice())
		// Rebuilding after particle transport/remapping or exogenous changes is
		// recorded separately from the self-gravity hydro bracket residual.
		workspace.health.Sources.GravityRebuildChange += fieldBefore - priorField
	}
	if err := workspace.engine.HydroAdvance(workspace.hydro, workspace.hydroOut, workspace.hydroWork1, workspace.hydroWork2, workspace.hydroStatus, workspace.acceleration, workspace.hydroParams(dt)); err != nil {
		return err
	}
	if err := checkFlags("gas RK2", workspace.hydroStatus, workspace.domain.CellCount()); err != nil {
		return err
	}
	if err := workspace.engine.HydroBudget(workspace.hydro, workspace.hydroWork1, workspace.hydroDiagnostics, workspace.hydroStatus, workspace.hydroParams(dt)); err != nil {
		return err
	}
	budget := workspace.hydroDiagnostics.Float32Slice()
	vol := math.Pow(workspace.domain.GridSpacing(), 3)
	for i := 0; i < workspace.domain.CellCount(); i++ {
		workspace.health.Sources.ViscousToHeat += float64(budget[2*i]) * vol
		workspace.health.Sources.ThermalConductionNet += float64(budget[2*i+1]) * vol
	}
	copy(workspace.hydro.UInt32Slice(), workspace.hydroOut.UInt32Slice())
	if workspace.physics.GravityG > 0 {
		if err := workspace.gravityKick(.5 * dt); err != nil {
			return err
		}
		beforeCompatible := sumHydroEnergy(workspace.hydro.Float32Slice(), workspace.domain.GridSpacing())
		if err := workspace.engine.GravityCompatible(workspace.gravityFluxBase, workspace.hydroWork1, workspace.hydro, workspace.gravityPrior, workspace.gravity, workspace.gravityKickWork, workspace.hydroOut, workspace.hydroStatus, workspace.hydroParams(dt)); err != nil {
			return err
		}
		copy(workspace.hydro.UInt32Slice(), workspace.hydroOut.UInt32Slice())
		workspace.health.Sources.GravityWork += sumHydroEnergy(workspace.hydro.Float32Slice(), workspace.domain.GridSpacing()) - beforeCompatible
	}
	after := sumHydroEnergy(workspace.hydro.Float32Slice(), workspace.domain.GridSpacing())
	workspace.health.Sources.GasEnergyResidual += after - before - (workspace.health.Sources.GravityWork - gravityBefore)
	if workspace.physics.GravityG > 0 {
		workspace.health.Sources.GravityBalanceResidual += workspace.health.Sources.GravityWork - gravityBefore + workspace.health.Sources.GravityFieldEnergy - fieldBefore
	}
	return workspace.exportDual()
}
func sumHydroEnergy(u []float32, dx float64) float64 {
	s := 0.0
	for i := 4; i < len(u); i += 6 {
		s += float64(u[i])
	}
	return s * dx * dx * dx
}
func (workspace *workspace) particleTotals() (mass, thermal, osc, kinetic float64, momentum [3]float64) {
	if workspace.particles == 0 {
		return
	}
	m, q, e, v := workspace.mass.Float32Slice(), workspace.heat.Float32Slice(), workspace.oscEnergy.Float32Slice(), workspace.vel.Float32Slice()
	for i := 0; i < workspace.particles; i++ {
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
func (workspace *workspace) gatherDual(dt float32) error {
	if workspace.particles == 0 {
		return nil
	}
	if err := workspace.engine.GatherDual(workspace.pos, workspace.mass, workspace.posOut, workspace.velOut, workspace.heatOut, workspace.hydro, workspace.particleStatus, workspace.particles, workspace.hydroParams(dt)); err != nil {
		return err
	}
	if err := checkFlags("PIC gather", workspace.particleStatus, workspace.particles); err != nil {
		return err
	}
	if err := workspace.engine.RemapConservative(workspace.hydro, workspace.posOut, workspace.mass, workspace.velOut, workspace.heatOut, workspace.materialEnergyOut, workspace.remapReport, workspace.particleStatus, workspace.particles, workspace.hydroParams(dt), float32(workspace.physics.RemapWidthCells), float32(workspace.physics.RemapTolerance), workspace.physics.RemapIterations, float32(workspace.physics.GravityG)); err != nil {
		return err
	}
	report := workspace.remapReport.Float32Slice()
	workspace.health.Remap = RemapHealth{MaxMarginalResidual: float64(report[0]), Iterations: int(report[1]), MassRoundoffScale: float64(report[2]), MixingToAuxiliary: float64(report[3]), EnergyResidual: float64(report[4]), MomentumResidual: [3]float64{float64(report[5]), float64(report[6]), float64(report[7])}, WidthCells: workspace.physics.RemapWidthCells}
	workspace.health.Sources.RemapMixingToAuxiliary += float64(report[3])
	if workspace.physics.GravityG > 0 {
		workspace.health.Sources.GravityRemapWork += float64(report[8])
		workspace.health.Sources.GravityFieldEnergy = float64(report[10])
		workspace.health.Sources.GravityRemapResidual += float64(report[11])
	}
	n := workspace.particles
	// Material grid state is an intermediate representation. Its remap error is
	// exposed as NUMERICAL discrepancy, never booked as an external source.
	gridMass, gridEnergy := 0.0, 0.0
	var gridP [3]float64
	u := workspace.hydro.Float32Slice()
	vol := math.Pow(workspace.domain.GridSpacing(), 3)
	for i := 0; i < len(u); i += 6 {
		gridMass += float64(u[i]) * vol
		gridEnergy += float64(u[i+4]) * vol
		for a := 0; a < 3; a++ {
			gridP[a] += float64(u[i+1+a]) * vol
		}
	}
	copy(workspace.pos.Float32Slice()[:3*n], workspace.posOut.Float32Slice()[:3*n])
	copy(workspace.vel.Float32Slice()[:3*n], workspace.velOut.Float32Slice()[:3*n])
	copy(workspace.heat.Float32Slice()[:n], workspace.heatOut.Float32Slice()[:n])
	copy(workspace.materialEnergy.Float32Slice()[:n], workspace.materialEnergyOut.Float32Slice()[:n])
	mass, _, _, _, mom := workspace.particleTotals()
	s := &workspace.health.Sources
	s.PICRemapMass += mass - gridMass
	s.PICRemapEnergy += workspace.materialTotal() - gridEnergy - float64(report[8])
	for a := 0; a < 3; a++ {
		s.PICRemapMomentum[a] += mom[a] - gridP[a]
	}
	return nil
}

// Gravity uses a solved periodic mean-subtracted Poisson field. Kicks update
// total energy by the EXACT kinetic-energy increment and leave auxiliary heat
// unchanged. Two kicks bracket hydro; no hidden forcing in PIC gather.
func (workspace *workspace) gravityKick(dt float32) error {
	p := workspace.hydroParams(dt)
	if err := workspace.engine.Poisson(workspace.hydro, workspace.poissonState, workspace.gravity, workspace.acceleration, workspace.hydroStatus, p, float32(workspace.physics.GravityG)); err != nil {
		return err
	}
	if err := checkFlags("Poisson", workspace.hydroStatus, workspace.domain.CellCount()); err != nil {
		return err
	}
	u, g := workspace.hydro.Float32Slice(), workspace.acceleration.Float32Slice()
	phi := workspace.gravity.Float32Slice()
	fieldEnergy := 0.0
	d := workspace.domain
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
	workspace.health.Sources.GravityFieldEnergy = fieldEnergy
	rate := 0.0
	for i := 0; i < workspace.domain.CellCount(); i++ {
		a2 := 0.0
		for a := 0; a < 3; a++ {
			a2 += float64(g[3*i+a]) * float64(g[3*i+a])
		}
		rate = math.Max(rate, math.Sqrt(a2))
	}
	if .5*rate*float64(2*dt)*float64(2*dt) > workspace.physics.ParticleCells*workspace.domain.GridSpacing() {
		return &CoupledStepError{"gravity displacement", -1, true, "gravity kick exceeds cell displacement criterion"}
	}
	before := sumHydroEnergy(u, workspace.domain.GridSpacing())
	old := make([]float32, workspace.domain.CellCount())
	for i := range old {
		old[i] = u[6*i+4]
	}
	if err := workspace.engine.GravityKick(workspace.hydro, workspace.acceleration, workspace.hydroStatus, p); err != nil {
		return err
	}
	if err := checkFlags("gravity kick", workspace.hydroStatus, workspace.domain.CellCount()); err != nil {
		return err
	}
	workspace.health.Sources.GravityWork += sumHydroEnergy(u, workspace.domain.GridSpacing()) - before
	work := workspace.gravityKickWork.Float32Slice()
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

func (workspace *workspace) materialTotal() float64 {
	if workspace.particles == 0 {
		return 0
	}
	sum := 0.
	for _, v := range workspace.materialEnergy.Float32Slice()[:workspace.particles] {
		sum += float64(v)
	}
	return sum
}

// A physical source changes conservative material energy and, separately, its
// auxiliary reservoir. The float32 representation residual is always recorded.
func (workspace *workspace) materialWork(i int, work float64) error {
	data := workspace.materialEnergy.Float32Slice()
	old := float64(data[i])
	next := float32(old + work)
	if !finite(float64(next)) || next < 0 {
		return &CoupledStepError{"material energy source", i, true, fmt.Sprintf("E=%g work=%g", old, work)}
	}
	data[i] = next
	workspace.health.Sources.MaterialRoundoff += float64(next) - old - work
	return nil
}
