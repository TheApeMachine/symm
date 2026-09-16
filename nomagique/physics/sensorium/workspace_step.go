package sensorium

import (
	"fmt"
	"math"
	"sort"
)

// step is one transactional macro interval. Every active operator is called with
// the SAME accepted float32 substep; rejected substeps restore all reservoirs,
// phases, RNG state, fields, and source counters before retrying.
func (workspace *workspace) step() (Reading, error) {
	if workspace.particles == 0 {
		return Reading{}, fmt.Errorf("sensorium: empty material domain; no coupled trajectory advanced")
	}
	if err := workspace.validateInputs(); err != nil {
		return Reading{}, err
	}
	rollback := workspace.snapshotPhysics()
	request := workspace.rates.deltaT
	defer func() { workspace.rates.deltaT = request }()
	_, _, e, _, _ := workspace.particleTotals()
	initialMatter := workspace.materialTotal() + e
	injected := initialMatter
	if workspace.hasLastParticleEnergy {
		injected -= workspace.lastParticleEnergy
	}
	priorGravity := workspace.health.Sources.GravityFieldEnergy
	workspace.health = PhysicsHealth{UnresolvedCoupling: true}
	workspace.health.Sources.GravityFieldEnergy = priorGravity
	workspace.health.Sources.ExogenousParticleEnergy = injected
	h, err := advanceCoupled(request, workspace.physics, workspace.snapshotPhysics, workspace.stabilityLimit, func(dt float32) error {
		workspace.rates.deltaT = float64(dt)
		if workspace.physics.Contacts.Enabled {
			if err := workspace.contactKick(.5 * dt); err != nil {
				return err
			}
			if err := workspace.depositDual(); err != nil {
				return err
			}
		}
		if err := workspace.gasDual(dt); err != nil {
			return err
		}

		if err := workspace.gatherDual(dt); err != nil {
			return err
		}
		if err := workspace.contactKick(.5 * dt); err != nil {
			return err
		}
		if err := workspace.planckExchange(); err != nil {
			return err
		}
		workspace.seedModeAnchors()
		if err := workspace.waveStep(); err != nil {
			return err
		}
		workspace.projectSpatialWave()
		if err := workspace.gatherPilotWave(); err != nil {
			return err
		}
		return workspace.measureHealth()
	})
	if err != nil {
		rollback()
		return Reading{}, err
	}
	// Retain the stability limits measured on the last attempted state.
	limits := workspace.health.Integrator
	h.HyperbolicDT = limits.HyperbolicDT
	h.ViscousDT = limits.ViscousDT
	h.ThermalDT = limits.ThermalDT
	h.ParticleDT = limits.ParticleDT
	h.PhaseDT = limits.PhaseDT
	h.CombinedDT = limits.CombinedDT
	h.ContactDT = limits.ContactDT
	workspace.physicalTime += h.AcceptedDT
	h.Time = workspace.physicalTime
	workspace.health.Integrator = h
	_, _, e, _, _ = workspace.particleTotals()
	ledger := &workspace.health.Sources
	accounted := ledger.PICDepositEnergyResidual + ledger.GravityWork + ledger.GasEnergyResidual + ledger.PICRemapEnergy + ledger.PlanckRoundoff - ledger.CouplingHeatExport + ledger.PilotWork + ledger.CoherenceMechanicalWork + ledger.ContactMaterialWork + ledger.GravityRemapWork
	ledger.ParticleBalanceResidual = workspace.materialTotal() + e - initialMatter - accounted - ledger.MaterialRoundoff
	reading := workspace.observe()
	if !reading.IsFinite() {
		rollback()
		return Reading{}, fmt.Errorf("nonfinite committed physical diagnostic")
	}
	_, _, e, _, _ = workspace.particleTotals()
	workspace.lastParticleEnergy = workspace.materialTotal() + e
	workspace.hasLastParticleEnergy = true
	return reading, nil
}

// The grid momentum and State.Vel represent TOTAL material transport velocity.
// PilotVel stores the previously imposed, externally prescribed drift component.
// Applying only new-old avoids re-adding a constant guidance velocity each frame.
// Its exact kinetic work is ledgered; no backreaction on Psi is implied.
func (workspace *workspace) gatherPilotWave() error {
	n := workspace.particles
	p := workspace.hydroParams(float32(workspace.rates.deltaT))
	if err := workspace.engine.PilotCheckedTime(workspace.pos, workspace.mass, workspace.pilotPrevious, workspace.psiStartRe, workspace.psiStartIm, workspace.psiRe, workspace.psiIm, workspace.posOut, workspace.velOut, workspace.pilotReport, workspace.particleStatus, n, p, float32(workspace.physics.Units.Hbar), float32(workspace.physics.PilotTolerance), float32(workspace.physics.ParticleCells)); err != nil {
		return err
	}
	workspace.engine.Synchronize()
	x, v, guide, old, m, report := workspace.pos.Float32Slice(), workspace.vel.Float32Slice(), workspace.velOut.Float32Slice(), workspace.pilotPrevious.Float32Slice(), workspace.mass.Float32Slice(), workspace.pilotReport.Float32Slice()
	h := PilotHealth{MinDensity: math.MaxFloat64}
	density := make([]float64, n)
	for i := 0; i < n; i++ {
		var work, speed2 float64
		for a := 0; a < 3; a++ {
			j := 3*i + a
			before := float64(v[j])
			next := float32(before + float64(guide[j]) - float64(old[j]))
			if !finite(float64(next)) {
				return &CoupledStepError{"pilot velocity", i, true, "nonfinite candidate"}
			}
			work += .5 * float64(m[i]) * (float64(next) - before) * (float64(next) + before)
			v[j] = next
			old[j] = guide[j]
			speed2 += float64(guide[j]) * float64(guide[j])
		}
		if err := workspace.materialWork(i, work); err != nil {
			return err
		}
		workspace.health.Sources.PilotWork += work
		d, e, cells := float64(report[4*i]), float64(report[4*i+1]), float64(report[4*i+2])
		density[i] = d
		h.MinDensity = math.Min(h.MinDensity, d)
		h.IntegrationErrorMax = math.Max(h.IntegrationErrorMax, e)
		h.SpeedRMS += speed2
		h.SpeedMax = math.Max(h.SpeedMax, math.Sqrt(speed2))
		h.DisplacementRMS += cells * cells
		h.DisplacementMax = math.Max(h.DisplacementMax, cells)
	}
	copy(x, workspace.posOut.Float32Slice())
	if n > 0 {
		h.SpeedRMS = math.Sqrt(h.SpeedRMS / float64(n))
		h.DisplacementRMS = math.Sqrt(h.DisplacementRMS / float64(n))
		sort.Float64s(density)
		h.DensityP01 = sampleQuantile(density, .01)
		h.DensityP10 = sampleQuantile(density, .1)
		h.DensityMedian = sampleQuantile(density, .5)
	} else {
		h.MinDensity = 0
	}
	workspace.health.Pilot = h
	return nil
}

func sampleQuantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	x := q * float64(len(sorted)-1)
	i := int(math.Floor(x))
	if i+1 == len(sorted) {
		return sorted[i]
	}
	return sorted[i] + (x-float64(i))*(sorted[i+1]-sorted[i])
}

/*
projectSpatialWave projects the resonant mode coefficients Ψ_k into the 3D
spatial complex field Ψ(x) via their spatial anchors on the GPU.
*/
func (workspace *workspace) projectSpatialWave() {
	if workspace.particles == 0 {
		workspace.psiRe.Zero()
		workspace.psiIm.Zero()
		return
	}

	if workspace.waveProjectionReady {
		copy(workspace.psiStartRe.UInt32Slice(), workspace.psiRe.UInt32Slice())
		copy(workspace.psiStartIm.UInt32Slice(), workspace.psiIm.UInt32Slice())
	}
	workspace.psiRe.Zero()
	workspace.psiIm.Zero()

	workspace.engine.ProjectModesToSpatial(
		workspace.psiModeReal,
		workspace.psiModeImag,
		workspace.anchorIdx,
		workspace.anchorWeight,
		workspace.pos,
		workspace.psiRe,
		workspace.psiIm,
		modeAnchors,
	)
	workspace.engine.Synchronize()
	if !workspace.waveProjectionReady {
		// First projection is an initial condition, not an interpolation from
		// a fictitious zero wave (where guidance is undefined).
		copy(workspace.psiStartRe.UInt32Slice(), workspace.psiRe.UInt32Slice())
		copy(workspace.psiStartIm.UInt32Slice(), workspace.psiIm.UInt32Slice())
		workspace.waveProjectionReady = true
	}
}

func wrapIndex(index, extent int) int {
	if extent <= 0 {
		return 0
	}

	wrapped := index % extent

	if wrapped < 0 {
		wrapped += extent
	}

	return wrapped
}

func (workspace *workspace) meanHeads() {
	workspace.engine.Synchronize()
	modes := int(workspace.domain.MaxModes)
	real := workspace.psiModeReal.Float32Slice()
	imag := workspace.psiModeImag.Float32Slice()
	heads := make([][]float32, spectralHeads)
	headImag := make([][]float32, spectralHeads)

	for head := 0; head < spectralHeads; head++ {
		heads[head] = workspace.psiRealHeads[head].Float32Slice()
		headImag[head] = workspace.psiImagHeads[head].Float32Slice()
	}

	scale := float32(spectralHeads)

	for mode := 0; mode < modes; mode++ {
		var sumRe, sumIm float32

		for head := 0; head < spectralHeads; head++ {
			sumRe += heads[head][mode]
			sumIm += headImag[head][mode]
		}

		real[mode] = sumRe / scale
		imag[mode] = sumIm / scale
	}
}

func (workspace *workspace) seedModeAnchors() {
	modes := int(workspace.domain.MaxModes)
	domega := workspace.domain.binWidth()
	idx := workspace.anchorIdx.Int32Slice()
	weight := workspace.anchorWeight.Float32Slice()
	fillInt32(idx, -1)

	for index := range weight {
		weight[index] = 0
	}

	if workspace.particles == 0 || !(domega > 0) {
		return
	}

	omega := workspace.omega.Float32Slice()
	amp := workspace.amp.Float32Slice()
	omegaMin := workspace.domain.OmegaMin
	buckets := make([][]int, modes)

	for particle := 0; particle < workspace.particles; particle++ {
		bin := int(math.Round((float64(omega[particle]) - omegaMin) / domega))

		if bin < 0 {
			bin = 0
		}

		if bin >= modes {
			bin = modes - 1
		}

		buckets[bin] = append(buckets[bin], particle)
	}

	for mode, members := range buckets {
		if len(members) == 0 {
			continue
		}

		sort.Slice(members, func(left, right int) bool {
			return amp[members[left]] > amp[members[right]]
		})
		chosen := members

		if len(chosen) > modeAnchors {
			chosen = chosen[:modeAnchors]
		}

		base := mode * modeAnchors

		for slot, particle := range chosen {
			idx[base+slot] = int32(particle)
			weight[base+slot] = amp[particle]
		}
	}
}

func (workspace *workspace) planckExchange() error {
	q, e, w, m, a := workspace.heat.Float32Slice(), workspace.oscEnergy.Float32Slice(), workspace.omega.Float32Slice(), workspace.mass.Float32Slice(), workspace.amp.Float32Slice()
	for i := 0; i < workspace.particles; i++ {
		q1, e1, residual, err := PlanckTransfer(float64(q[i]), float64(e[i]), float64(m[i]), float64(w[i]), workspace.domain.CV,
			workspace.domain.KThermal, .5*workspace.domain.GridSpacing(), workspace.rates.deltaT, workspace.physics.Units)
		if err != nil {
			return &CoupledStepError{"Planck exchange", i, false, err.Error()}
		}
		workspace.health.Sources.PlanckToOscillator += float64(e1) - float64(e[i])
		workspace.health.Sources.PlanckRoundoff += residual
		if err := workspace.materialWork(i, float64(q1)-float64(q[i])); err != nil {
			return err
		}
		q[i] = q1
		e[i] = e1
		a[i] = float32(math.Sqrt(float64(e1)))
	}
	return nil
}

func isPositiveFinite(value float64) bool { return value > 0 && finite(value) }

func (workspace *workspace) waveStep() error {
	dt := float32(workspace.rates.deltaT)
	modes := int(workspace.domain.MaxModes)
	sigma, err := workspace.spatialSigma()
	if err != nil {
		return err
	}
	// No unbounded truncation of weak couplings or normalized drive.
	weightFloor := float32(0)
	anchorEps := float32(math.Sqrt(1.0 / float64(uint32(1)<<23)))
	gateMin := float32(workspace.domain.linewidthMin())
	gateMax := float32(workspace.domain.linewidthMax())
	workspace.writeCouplingAmp()
	heat := workspace.heat.Float32Slice()
	phase := workspace.phase.Float32Slice()
	headHeat := workspace.headHeat.Float32Slice()
	headPhase := workspace.headPhase.Float32Slice()
	budget := 1 / float32(spectralHeads)
	originalHeat := append([]float32(nil), heat[:workspace.particles]...)
	spent := make([]float64, workspace.particles)
	workspace.health.Wave = WaveHealth{}
	workspace.reciprocalForce.Zero()
	effective := workspace.reciprocalAmplitude.Float32Slice()
	amplitudes := workspace.couplingAmp.Float32Slice()
	for i := 0; i < workspace.particles; i++ {
		// Same float32 operation order as mc_coupling_budget. This does not
		// debit heat a second time; it freezes the interaction's coefficient.
		required := ((float32(workspace.rates.metabolicRate) * dt) * amplitudes[i]) * amplitudes[i]
		available := originalHeat[i] * budget
		if !finite(float64(required)) || required < 0 {
			return &CoupledStepError{"reciprocal coefficient", i, false, "invalid budget"}
		}
		fraction := float32(1)
		if required > 0 {
			fraction = min(required, available) / required
		}
		effective[i] = amplitudes[i] * float32(math.Sqrt(float64(fraction)))
	}

	// Step each spectral head independently under the GPE
	for head := 0; head < spectralHeads; head++ {
		offset := float64(head) * (2 * math.Pi) / float64(spectralHeads)
		workspace.accums.Zero()

		for particle := 0; particle < workspace.particles; particle++ {
			headHeat[particle] = originalHeat[particle] * budget
			headPhase[particle] = float32(wrapPhase(float64(phase[particle]) + offset))
		}

		workspace.engine.CoherenceAccumulateForces(
			workspace.headPhase, workspace.omega, workspace.couplingAmp, workspace.pos,
			workspace.omegaLattice, workspace.gateWidth, workspace.anchorIdx, workspace.anchorWeight,
			workspace.accums, workspace.binStarts, workspace.binnedIdx, workspace.binParams,
			modes,
			workspace.headHeat,
			workspace.particles,
			workspace.numCarriers,
			modes,
			dt,
			float32(workspace.rates.metabolicRate),
			gateMin, gateMax, weightFloor,
			float32(sigma),
		)

		workspace.engine.Synchronize()
		oldReal := append([]float32(nil), workspace.psiRealHeads[head].Float32Slice()[:modes]...)
		oldImag := append([]float32(nil), workspace.psiImagHeads[head].Float32Slice()[:modes]...)
		copy(workspace.reciprocalOldRe.Float32Slice(), oldReal)
		copy(workspace.reciprocalOldIm.Float32Slice(), oldImag)
		workspace.engine.CoherenceGPEStep(
			workspace.headPhase, workspace.omega, workspace.couplingAmp,
			workspace.psiRealHeads[head], workspace.psiImagHeads[head],
			workspace.omegaLattice, workspace.gateWidth,
			workspace.kineticReal, workspace.kineticImag,
			workspace.anchorIdx, workspace.anchorWeight, workspace.accums,
			workspace.numCarriers, workspace.pos,
			workspace.spectralPotential,
			workspace.particles, modes,
			dt,
			float32(workspace.physics.Units.Hbar), float32(massEff),
			float32(workspace.rates.gInteraction),
			float32(workspace.rates.energyDecay),
			0,
			float32(workspace.domain.invDomega2()),
			workspace.rngSeed+uint32(head),
			anchorEps,
			0,
			float32(workspace.rates.metabolicRate),
			gateMin, gateMax, weightFloor, float32(sigma),
			workspace.spectralMetric,
		)

		workspace.engine.Synchronize()

		if err := workspace.engine.ReadWaveLedger(workspace.waveLedger, modes); err != nil {
			return err
		}
		if err := workspace.engine.ReciprocalHead(workspace.pos, workspace.coherencePosition, workspace.reciprocalAmplitude, workspace.omega, workspace.omegaLattice, workspace.gateWidth, workspace.anchorIdx, workspace.anchorWeight, workspace.reciprocalOldRe, workspace.reciprocalOldIm, workspace.waveLedger, workspace.reciprocalForce, workspace.reciprocalPotential, workspace.reciprocalStatus, workspace.particles, modes, modeAnchors, float32(sigma), float32(workspace.domain.binWidth()), workspace.hydroParams(dt)); err != nil {
			return err
		}
		if err := workspace.accountWaveHead(head, oldReal, oldImag); err != nil {
			return err
		}
		for i := 0; i < workspace.particles; i++ {
			supplied := float64(originalHeat[i] * budget)
			remaining := float64(headHeat[i])
			if !finite(remaining) || remaining < 0 || remaining > supplied {
				return &CoupledStepError{"coherence heat budget", i, false, fmt.Sprintf("budget=%g remaining=%g", supplied, remaining)}
			}
			spent[i] += supplied - remaining
		}
		workspace.rngSeed++
	}

	for i := 0; i < workspace.particles; i++ {
		remaining, err := debitHeadBudget(originalHeat[i], spent[i])
		if err != nil {
			return &CoupledStepError{"head reconciliation", i, false, err.Error()}
		}
		workspace.health.Sources.CouplingHeatExport += float64(originalHeat[i]) - float64(remaining)
		if err := workspace.materialWork(i, float64(remaining)-float64(heat[i])); err != nil {
			return err
		}
		heat[i] = remaining
	}
	if err := workspace.applyCoherenceImpulse(dt); err != nil {
		return err
	}
	copy(workspace.coherencePosition.Float32Slice(), workspace.pos.Float32Slice())
	// 1. Average across all spectral heads into psiModeReal & psiModeImag
	workspace.meanHeads()

	// 2. Apply resonance torque to pull particle phases towards resonant modes
	workspace.engine.CoherenceUpdateOscillatorPhases(
		workspace.phase, workspace.omega, workspace.amp,
		workspace.psiModeReal, workspace.psiModeImag, workspace.omegaLattice, workspace.gateWidth,
		workspace.anchorIdx, workspace.anchorWeight, workspace.numCarriers,
		workspace.particles, modes,
		dt,
		1.0, // coupling scale
		gateMin, gateMax,
		workspace.binStarts, workspace.binnedIdx, workspace.binParams,
		modes,
		workspace.pos,
		float32(sigma),
		float32(workspace.rates.metabolicRate),
		weightFloor,
	)
	workspace.engine.Synchronize()
	for i, value := range phase[:workspace.particles] {
		if !finite(float64(value)) {
			return &CoupledStepError{"phase synchronization", i, true, "nonfinite phase"}
		}
	}
	if err := workspace.engine.ReadPhaseLedger(workspace.phaseLedger, workspace.particles); err != nil {
		return err
	}
	return workspace.accountPhase()
}

// Canonical oscillator coordinate: A=sqrt(Eosc). There is no unexplained
// inverse-square-root frequency reweighting of an already normalized amplitude.
func (workspace *workspace) writeCouplingAmp() {
	copy(workspace.couplingAmp.Float32Slice()[:workspace.particles], workspace.amp.Float32Slice()[:workspace.particles])
}

// The normalized free-particle thermal density matrix is exp(-r^2/(4*sigma^2)),
// sigma^2=hbar^2/(2*m*kB*T). The native overlap periodizes that heat kernel;
// there is NO grid-spacing or half-box clamp. Mean m,T define an explicit
// homogeneous effective bath, not a per-particle microscopic derivation.
func (workspace *workspace) spatialSigma() (float64, error) {
	if workspace.particles == 0 {
		return 0, nil
	}
	q, m := workspace.heat.Float32Slice(), workspace.mass.Float32Slice()
	ts, ms := 0.0, 0.0
	for i := 0; i < workspace.particles; i++ {
		if !isPositiveFinite(float64(m[i])) || !finite(float64(q[i])) || q[i] < 0 {
			return 0, fmt.Errorf("invalid thermal coherence sample %d", i)
		}
		ts += float64(q[i]) / (float64(m[i]) * workspace.domain.CV)
		ms += float64(m[i])
	}
	tm, mm := ts/float64(workspace.particles), ms/float64(workspace.particles)
	if !finite(tm) || !isPositiveFinite(mm) {
		return 0, fmt.Errorf("nonfinite effective thermal bath")
	}
	raw, used := 0.0, 0.0
	uniform := tm == 0
	if tm > 0 {
		raw = workspace.physics.Units.Hbar / math.Sqrt(2*mm*workspace.physics.Units.Boltzmann*tm)
		if !isPositiveFinite(raw) {
			return 0, fmt.Errorf("invalid thermal coherence scale")
		}
		// For sigma>=every box extent the first nonconstant Fourier coefficient
		// is < exp(-4*pi^2), far below float32 resolution. 0 explicitly denotes
		// this uniform limit in the kernel; it never denotes a density floor.
		uniform = raw >= max(workspace.domain.DomainX, workspace.domain.DomainY, workspace.domain.DomainZ)
		if !uniform {
			used = float64(float32(raw))
			if !isPositiveFinite(used) {
				return 0, fmt.Errorf("thermal coherence scale is not representable in float32")
			}
		}
	}
	workspace.health.SpatialSigmaRaw = raw
	workspace.health.SpatialSigmaUsed = used
	workspace.health.SigmaUnderResolved = tm > 0 && raw < workspace.domain.GridSpacing()
	workspace.health.SigmaUniformLimit = uniform
	return used, nil
}

func (workspace *workspace) observe() Reading {
	workspace.engine.Synchronize()
	rho := workspace.rho.Float32Slice()
	mom := workspace.mom.Float32Slice()
	energy := workspace.energy.Float32Slice()
	gx := int(workspace.domain.GridX)
	gy := int(workspace.domain.GridY)
	gz := int(workspace.domain.GridZ)
	dx := workspace.domain.GridSpacing()
	gamma := workspace.domain.Gamma
	var speed2, divAbs, press2, strain2 float64
	cells := gx * gy * gz

	for z := range gz {
		for y := range gy {
			for x := range gx {
				cell := x + gx*(y+gy*z)
				density := float64(rho[cell])
				ux, uy, uz := 0.0, 0.0, 0.0

				if density != 0 {
					ux = float64(mom[cell*3+0]) / density
					uy = float64(mom[cell*3+1]) / density
					uz = float64(mom[cell*3+2]) / density
				}

				speed2 += ux*ux + uy*uy + uz*uz
				xp := (x + 1) % gx
				xm := (x - 1 + gx) % gx
				yp := (y + 1) % gy
				ym := (y - 1 + gy) % gy
				zp := (z + 1) % gz
				zm := (z - 1 + gz) % gz

				divAbs += math.Abs((cellVelocity(
					rho, mom, xp, y, z, gx, gy, 0,
				) - cellVelocity(
					rho, mom, xm, y, z, gx, gy, 0,
				) + cellVelocity(
					rho, mom, x, yp, z, gx, gy, 1,
				) - cellVelocity(
					rho, mom, x, ym, z, gx, gy, 1,
				) + cellVelocity(
					rho, mom, x, y, zp, gx, gy, 2,
				) - cellVelocity(
					rho, mom, x, y, zm, gx, gy, 2,
				)) / (2 * dx))

				px := cellPressure(
					energy, xp, y, z, gx, gy, gamma,
				) - cellPressure(
					energy, xm, y, z, gx, gy, gamma,
				)

				py := cellPressure(
					energy, x, yp, z, gx, gy, gamma,
				) - cellPressure(
					energy, x, ym, z, gx, gy, gamma,
				)

				pz := cellPressure(
					energy, x, y, zp, gx, gy, gamma,
				) - cellPressure(
					energy, x, y, zm, gx, gy, gamma,
				)

				press2 += (px*px + py*py + pz*pz) / (4 * dx * dx)

				dudx := (cellVelocity(
					rho, mom, xp, y, z, gx, gy, 0,
				) - cellVelocity(
					rho, mom, xm, y, z, gx, gy, 0,
				)) / (2 * dx)

				dvdy := (cellVelocity(
					rho, mom, x, yp, z, gx, gy, 1,
				) - cellVelocity(
					rho, mom, x, ym, z, gx, gy, 1,
				)) / (2 * dx)

				dwdz := (cellVelocity(
					rho, mom, x, y, zp, gx, gy, 2,
				) - cellVelocity(
					rho, mom, x, y, zm, gx, gy, 2,
				)) / (2 * dx)

				strain2 += dudx*dudx + dvdy*dvdy + dwdz*dwdz
			}
		}
	}

	count := float64(cells)
	var coherence float64

	for head := 0; head < spectralHeads; head++ {
		real := workspace.psiRealHeads[head].Float32Slice()
		imag := workspace.psiImagHeads[head].Float32Slice()

		for mode, value := range real {
			coherence += float64(value)*float64(value) + float64(imag[mode])*float64(imag[mode])
		}
	}

	modes := float64(workspace.domain.MaxModes * spectralHeads)
	return Reading{
		Health:           workspace.health,
		GuidanceSpeed:    math.Sqrt(speed2 / count),
		Divergence:       divAbs / count,
		PressureGradNorm: math.Sqrt(press2 / count),
		ViscosityProxy:   workspace.domain.Mu * math.Sqrt(strain2/count),
		CoherenceMag2:    coherence / modes,
		KuramotoR:        kuramotoFromPhase(workspace.phase, workspace.particles),
	}
}

func cellVelocity(rho, mom []float32, x, y, z, gx, gy, axis int) float64 {
	cell := x + gx*(y+gy*z)
	density := float64(rho[cell])

	if density == 0 {
		return 0
	}

	return float64(mom[cell*3+axis]) / density
}

func cellPressure(energy []float32, x, y, z, gx, gy int, gamma float64) float64 {
	cell := x + gx*(y+gy*z)
	return (gamma - 1) * float64(energy[cell])
}

func kuramotoFromPhase(phase *Buffer, particles int) float64 {
	if phase == nil || particles == 0 {
		return 0
	}

	values := phase.Float32Slice()[:particles]

	var sumCos, sumSin float64

	for _, value := range values {
		sumCos += math.Cos(float64(value))
		sumSin += math.Sin(float64(value))
	}

	count := float64(len(values))
	meanCos := sumCos / count
	meanSin := sumSin / count
	return math.Sqrt(meanCos*meanCos + meanSin*meanSin)
}

func wrapPhase(phase float64) float64 {
	twoPi := 2 * math.Pi
	wrapped := math.Mod(phase, twoPi)

	if wrapped < 0 {
		wrapped += twoPi
	}

	return wrapped
}
