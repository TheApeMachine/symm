package sensorium

import (
	"fmt"
	"math"
	"sort"
)

// step is one transactional macro interval. Every active operator is called with
// the SAME accepted float32 substep; rejected substeps restore all reservoirs,
// phases, RNG state, fields, and source counters before retrying.
func (fluid *workspace) step() (Reading, error) {
	if fluid.particles == 0 {
		return Reading{}, fmt.Errorf("sensorium: empty material domain; no coupled trajectory advanced")
	}
	if err := fluid.validateInputs(); err != nil {
		return Reading{}, err
	}
	rollback := fluid.snapshotPhysics()
	request := fluid.rates.deltaT
	defer func() { fluid.rates.deltaT = request }()
	_, _, e, _, _ := fluid.particleTotals()
	initialMatter := fluid.materialTotal() + e
	injected := initialMatter
	if fluid.hasLastParticleEnergy {
		injected -= fluid.lastParticleEnergy
	}
	priorGravity := fluid.health.Sources.GravityFieldEnergy
	fluid.health = PhysicsHealth{UnresolvedCoupling: true}
	fluid.health.Sources.GravityFieldEnergy = priorGravity
	fluid.health.Sources.ExogenousParticleEnergy = injected
	h, err := advanceCoupled(request, fluid.physics, fluid.snapshotPhysics, fluid.stabilityLimit, func(dt float32) error {
		fluid.rates.deltaT = float64(dt)
		if fluid.physics.Contacts.Enabled {
			if err := fluid.contactKick(.5 * dt); err != nil {
				return err
			}
			if err := fluid.depositDual(); err != nil {
				return err
			}
		}
		if err := fluid.gasDual(dt); err != nil {
			return err
		}

		if err := fluid.gatherDual(dt); err != nil {
			return err
		}
		if err := fluid.contactKick(.5 * dt); err != nil {
			return err
		}
		if err := fluid.planckExchange(); err != nil {
			return err
		}
		fluid.seedModeAnchors()
		if err := fluid.waveStep(); err != nil {
			return err
		}
		fluid.projectSpatialWave()
		if err := fluid.gatherPilotWave(); err != nil {
			return err
		}
		return fluid.measureHealth()
	})
	if err != nil {
		rollback()
		return Reading{}, err
	}
	// Retain the stability limits measured on the last attempted state.
	limits := fluid.health.Integrator
	h.HyperbolicDT = limits.HyperbolicDT
	h.ViscousDT = limits.ViscousDT
	h.ThermalDT = limits.ThermalDT
	h.ParticleDT = limits.ParticleDT
	h.PhaseDT = limits.PhaseDT
	h.CombinedDT = limits.CombinedDT
	h.ContactDT = limits.ContactDT
	fluid.physicalTime += h.AcceptedDT
	h.Time = fluid.physicalTime
	fluid.health.Integrator = h
	_, _, e, _, _ = fluid.particleTotals()
	ledger := &fluid.health.Sources
	accounted := ledger.PICDepositEnergyResidual + ledger.GravityWork + ledger.GasEnergyResidual + ledger.PICRemapEnergy + ledger.PlanckRoundoff - ledger.CouplingHeatExport + ledger.PilotWork + ledger.CoherenceMechanicalWork + ledger.ContactMaterialWork + ledger.GravityRemapWork
	ledger.ParticleBalanceResidual = fluid.materialTotal() + e - initialMatter - accounted - ledger.MaterialRoundoff
	reading := fluid.observe()
	if !reading.IsFinite() {
		rollback()
		return Reading{}, fmt.Errorf("nonfinite committed physical diagnostic")
	}
	_, _, e, _, _ = fluid.particleTotals()
	fluid.lastParticleEnergy = fluid.materialTotal() + e
	fluid.hasLastParticleEnergy = true
	return reading, nil
}

// The grid momentum and State.Vel represent TOTAL material transport velocity.
// PilotVel stores the previously imposed, externally prescribed drift component.
// Applying only new-old avoids re-adding a constant guidance velocity each frame.
// Its exact kinetic work is ledgered; no backreaction on Psi is implied.
func (fluid *workspace) gatherPilotWave() error {
	n := fluid.particles
	p := fluid.hydroParams(float32(fluid.rates.deltaT))
	if err := fluid.engine.PilotCheckedTime(fluid.pos, fluid.mass, fluid.pilotPrevious, fluid.psiStartRe, fluid.psiStartIm, fluid.psiRe, fluid.psiIm, fluid.posOut, fluid.velOut, fluid.pilotReport, fluid.particleStatus, n, p, float32(fluid.physics.Units.Hbar), float32(fluid.physics.PilotTolerance), float32(fluid.physics.ParticleCells)); err != nil {
		return err
	}
	fluid.engine.Synchronize()
	x, v, guide, old, m, report := fluid.pos.Float32Slice(), fluid.vel.Float32Slice(), fluid.velOut.Float32Slice(), fluid.pilotPrevious.Float32Slice(), fluid.mass.Float32Slice(), fluid.pilotReport.Float32Slice()
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
		if err := fluid.materialWork(i, work); err != nil {
			return err
		}
		fluid.health.Sources.PilotWork += work
		d, e, cells := float64(report[4*i]), float64(report[4*i+1]), float64(report[4*i+2])
		density[i] = d
		h.MinDensity = math.Min(h.MinDensity, d)
		h.IntegrationErrorMax = math.Max(h.IntegrationErrorMax, e)
		h.SpeedRMS += speed2
		h.SpeedMax = math.Max(h.SpeedMax, math.Sqrt(speed2))
		h.DisplacementRMS += cells * cells
		h.DisplacementMax = math.Max(h.DisplacementMax, cells)
	}
	copy(x, fluid.posOut.Float32Slice())
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
	fluid.health.Pilot = h
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
func (fluid *workspace) projectSpatialWave() {
	if fluid.particles == 0 {
		fluid.psiRe.Zero()
		fluid.psiIm.Zero()
		return
	}

	if fluid.waveProjectionReady {
		copy(fluid.psiStartRe.UInt32Slice(), fluid.psiRe.UInt32Slice())
		copy(fluid.psiStartIm.UInt32Slice(), fluid.psiIm.UInt32Slice())
	}
	fluid.psiRe.Zero()
	fluid.psiIm.Zero()

	fluid.engine.ProjectModesToSpatial(
		fluid.psiModeReal,
		fluid.psiModeImag,
		fluid.anchorIdx,
		fluid.anchorWeight,
		fluid.pos,
		fluid.psiRe,
		fluid.psiIm,
		modeAnchors,
	)
	fluid.engine.Synchronize()
	if !fluid.waveProjectionReady {
		// First projection is an initial condition, not an interpolation from
		// a fictitious zero wave (where guidance is undefined).
		copy(fluid.psiStartRe.UInt32Slice(), fluid.psiRe.UInt32Slice())
		copy(fluid.psiStartIm.UInt32Slice(), fluid.psiIm.UInt32Slice())
		fluid.waveProjectionReady = true
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

func (fluid *workspace) meanHeads() {
	fluid.engine.Synchronize()
	modes := int(fluid.domain.MaxModes)
	real := fluid.psiModeReal.Float32Slice()
	imag := fluid.psiModeImag.Float32Slice()
	heads := make([][]float32, spectralHeads)
	headImag := make([][]float32, spectralHeads)

	for head := 0; head < spectralHeads; head++ {
		heads[head] = fluid.psiRealHeads[head].Float32Slice()
		headImag[head] = fluid.psiImagHeads[head].Float32Slice()
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

func (fluid *workspace) seedModeAnchors() {
	modes := int(fluid.domain.MaxModes)
	domega := fluid.domain.binWidth()
	idx := fluid.anchorIdx.Int32Slice()
	weight := fluid.anchorWeight.Float32Slice()
	fillInt32(idx, -1)

	for index := range weight {
		weight[index] = 0
	}

	if fluid.particles == 0 || !(domega > 0) {
		return
	}

	omega := fluid.omega.Float32Slice()
	amp := fluid.amp.Float32Slice()
	omegaMin := fluid.domain.OmegaMin
	buckets := make([][]int, modes)

	for particle := 0; particle < fluid.particles; particle++ {
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

func (fluid *workspace) planckExchange() error {
	q, e, w, m, a := fluid.heat.Float32Slice(), fluid.oscEnergy.Float32Slice(), fluid.omega.Float32Slice(), fluid.mass.Float32Slice(), fluid.amp.Float32Slice()
	for i := 0; i < fluid.particles; i++ {
		q1, e1, residual, err := PlanckTransfer(float64(q[i]), float64(e[i]), float64(m[i]), float64(w[i]), fluid.domain.CV,
			fluid.domain.KThermal, .5*fluid.domain.GridSpacing(), fluid.rates.deltaT, fluid.physics.Units)
		if err != nil {
			return &CoupledStepError{"Planck exchange", i, false, err.Error()}
		}
		fluid.health.Sources.PlanckToOscillator += float64(e1) - float64(e[i])
		fluid.health.Sources.PlanckRoundoff += residual
		if err := fluid.materialWork(i, float64(q1)-float64(q[i])); err != nil {
			return err
		}
		q[i] = q1
		e[i] = e1
		a[i] = float32(math.Sqrt(float64(e1)))
	}
	return nil
}

func isPositiveFinite(value float64) bool { return value > 0 && finite(value) }

func (fluid *workspace) waveStep() error {
	dt := float32(fluid.rates.deltaT)
	modes := int(fluid.domain.MaxModes)
	sigma, err := fluid.spatialSigma()
	if err != nil {
		return err
	}
	// No unbounded truncation of weak couplings or normalized drive.
	weightFloor := float32(0)
	anchorEps := float32(math.Sqrt(1.0 / float64(uint32(1)<<23)))
	gateMin := float32(fluid.domain.linewidthMin())
	gateMax := float32(fluid.domain.linewidthMax())
	fluid.writeCouplingAmp()
	heat := fluid.heat.Float32Slice()
	phase := fluid.phase.Float32Slice()
	headHeat := fluid.headHeat.Float32Slice()
	headPhase := fluid.headPhase.Float32Slice()
	budget := 1 / float32(spectralHeads)
	originalHeat := append([]float32(nil), heat[:fluid.particles]...)
	spent := make([]float64, fluid.particles)
	fluid.health.Wave = WaveHealth{}
	fluid.reciprocalForce.Zero()
	effective := fluid.reciprocalAmplitude.Float32Slice()
	amplitudes := fluid.couplingAmp.Float32Slice()
	for i := 0; i < fluid.particles; i++ {
		// Same float32 operation order as mc_coupling_budget. This does not
		// debit heat a second time; it freezes the interaction's coefficient.
		required := ((float32(fluid.rates.metabolicRate) * dt) * amplitudes[i]) * amplitudes[i]
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
		fluid.accums.Zero()

		for particle := 0; particle < fluid.particles; particle++ {
			headHeat[particle] = originalHeat[particle] * budget
			headPhase[particle] = float32(wrapPhase(float64(phase[particle]) + offset))
		}

		fluid.engine.CoherenceAccumulateForces(
			fluid.headPhase, fluid.omega, fluid.couplingAmp, fluid.pos,
			fluid.omegaLattice, fluid.gateWidth, fluid.anchorIdx, fluid.anchorWeight,
			fluid.accums, fluid.binStarts, fluid.binnedIdx, fluid.binParams,
			modes,
			fluid.headHeat,
			fluid.particles,
			fluid.numCarriers,
			modes,
			dt,
			float32(fluid.rates.metabolicRate),
			gateMin, gateMax, weightFloor,
			float32(sigma),
		)

		fluid.engine.Synchronize()
		oldReal := append([]float32(nil), fluid.psiRealHeads[head].Float32Slice()[:modes]...)
		oldImag := append([]float32(nil), fluid.psiImagHeads[head].Float32Slice()[:modes]...)
		copy(fluid.reciprocalOldRe.Float32Slice(), oldReal)
		copy(fluid.reciprocalOldIm.Float32Slice(), oldImag)
		fluid.engine.CoherenceGPEStep(
			fluid.headPhase, fluid.omega, fluid.couplingAmp,
			fluid.psiRealHeads[head], fluid.psiImagHeads[head],
			fluid.omegaLattice, fluid.gateWidth,
			fluid.kineticReal, fluid.kineticImag,
			fluid.anchorIdx, fluid.anchorWeight, fluid.accums,
			fluid.numCarriers, fluid.pos,
			fluid.spectralPotential,
			fluid.particles, modes,
			dt,
			float32(fluid.physics.Units.Hbar), float32(massEff),
			float32(fluid.rates.gInteraction),
			float32(fluid.rates.energyDecay),
			0,
			float32(fluid.domain.invDomega2()),
			fluid.rngSeed+uint32(head),
			anchorEps,
			0,
			float32(fluid.rates.metabolicRate),
			gateMin, gateMax, weightFloor, float32(sigma),
			fluid.spectralMetric,
		)

		fluid.engine.Synchronize()

		if err := fluid.engine.ReadWaveLedger(fluid.waveLedger, modes); err != nil {
			return err
		}
		if err := fluid.engine.ReciprocalHead(fluid.pos, fluid.coherencePosition, fluid.reciprocalAmplitude, fluid.omega, fluid.omegaLattice, fluid.gateWidth, fluid.anchorIdx, fluid.anchorWeight, fluid.reciprocalOldRe, fluid.reciprocalOldIm, fluid.waveLedger, fluid.reciprocalForce, fluid.reciprocalPotential, fluid.reciprocalStatus, fluid.particles, modes, modeAnchors, float32(sigma), float32(fluid.domain.binWidth()), fluid.hydroParams(dt)); err != nil {
			return err
		}
		if err := fluid.accountWaveHead(head, oldReal, oldImag); err != nil {
			return err
		}
		for i := 0; i < fluid.particles; i++ {
			supplied := float64(originalHeat[i] * budget)
			remaining := float64(headHeat[i])
			if !finite(remaining) || remaining < 0 || remaining > supplied {
				return &CoupledStepError{"coherence heat budget", i, false, fmt.Sprintf("budget=%g remaining=%g", supplied, remaining)}
			}
			spent[i] += supplied - remaining
		}
		fluid.rngSeed++
	}

	for i := 0; i < fluid.particles; i++ {
		remaining, err := debitHeadBudget(originalHeat[i], spent[i])
		if err != nil {
			return &CoupledStepError{"head reconciliation", i, false, err.Error()}
		}
		fluid.health.Sources.CouplingHeatExport += float64(originalHeat[i]) - float64(remaining)
		if err := fluid.materialWork(i, float64(remaining)-float64(heat[i])); err != nil {
			return err
		}
		heat[i] = remaining
	}
	if err := fluid.applyCoherenceImpulse(dt); err != nil {
		return err
	}
	copy(fluid.coherencePosition.Float32Slice(), fluid.pos.Float32Slice())
	// 1. Average across all spectral heads into psiModeReal & psiModeImag
	fluid.meanHeads()

	// 2. Apply resonance torque to pull particle phases towards resonant modes
	fluid.engine.CoherenceUpdateOscillatorPhases(
		fluid.phase, fluid.omega, fluid.amp,
		fluid.psiModeReal, fluid.psiModeImag, fluid.omegaLattice, fluid.gateWidth,
		fluid.anchorIdx, fluid.anchorWeight, fluid.numCarriers,
		fluid.particles, modes,
		dt,
		1.0, // coupling scale
		gateMin, gateMax,
		fluid.binStarts, fluid.binnedIdx, fluid.binParams,
		modes,
		fluid.pos,
		float32(sigma),
		float32(fluid.rates.metabolicRate),
		weightFloor,
	)
	fluid.engine.Synchronize()
	for i, value := range phase[:fluid.particles] {
		if !finite(float64(value)) {
			return &CoupledStepError{"phase synchronization", i, true, "nonfinite phase"}
		}
	}
	if err := fluid.engine.ReadPhaseLedger(fluid.phaseLedger, fluid.particles); err != nil {
		return err
	}
	return fluid.accountPhase()
}

// Canonical oscillator coordinate: A=sqrt(Eosc). There is no unexplained
// inverse-square-root frequency reweighting of an already normalized amplitude.
func (fluid *workspace) writeCouplingAmp() {
	copy(fluid.couplingAmp.Float32Slice()[:fluid.particles], fluid.amp.Float32Slice()[:fluid.particles])
}

// The normalized free-particle thermal density matrix is exp(-r^2/(4*sigma^2)),
// sigma^2=hbar^2/(2*m*kB*T). The native overlap periodizes that heat kernel;
// there is NO grid-spacing or half-box clamp. Mean m,T define an explicit
// homogeneous effective bath, not a per-particle microscopic derivation.
func (fluid *workspace) spatialSigma() (float64, error) {
	if fluid.particles == 0 {
		return 0, nil
	}
	q, m := fluid.heat.Float32Slice(), fluid.mass.Float32Slice()
	ts, ms := 0.0, 0.0
	for i := 0; i < fluid.particles; i++ {
		if !isPositiveFinite(float64(m[i])) || !finite(float64(q[i])) || q[i] < 0 {
			return 0, fmt.Errorf("invalid thermal coherence sample %d", i)
		}
		ts += float64(q[i]) / (float64(m[i]) * fluid.domain.CV)
		ms += float64(m[i])
	}
	tm, mm := ts/float64(fluid.particles), ms/float64(fluid.particles)
	if !finite(tm) || !isPositiveFinite(mm) {
		return 0, fmt.Errorf("nonfinite effective thermal bath")
	}
	raw, used := 0.0, 0.0
	uniform := tm == 0
	if tm > 0 {
		raw = fluid.physics.Units.Hbar / math.Sqrt(2*mm*fluid.physics.Units.Boltzmann*tm)
		if !isPositiveFinite(raw) {
			return 0, fmt.Errorf("invalid thermal coherence scale")
		}
		// For sigma>=every box extent the first nonconstant Fourier coefficient
		// is < exp(-4*pi^2), far below float32 resolution. 0 explicitly denotes
		// this uniform limit in the kernel; it never denotes a density floor.
		uniform = raw >= max(fluid.domain.DomainX, fluid.domain.DomainY, fluid.domain.DomainZ)
		if !uniform {
			used = float64(float32(raw))
			if !isPositiveFinite(used) {
				return 0, fmt.Errorf("thermal coherence scale is not representable in float32")
			}
		}
	}
	fluid.health.SpatialSigmaRaw = raw
	fluid.health.SpatialSigmaUsed = used
	fluid.health.SigmaUnderResolved = tm > 0 && raw < fluid.domain.GridSpacing()
	fluid.health.SigmaUniformLimit = uniform
	return used, nil
}

func (fluid *workspace) observe() Reading {
	fluid.engine.Synchronize()
	rho := fluid.rho.Float32Slice()
	mom := fluid.mom.Float32Slice()
	energy := fluid.energy.Float32Slice()
	gx := int(fluid.domain.GridX)
	gy := int(fluid.domain.GridY)
	gz := int(fluid.domain.GridZ)
	dx := fluid.domain.GridSpacing()
	gamma := fluid.domain.Gamma
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
		real := fluid.psiRealHeads[head].Float32Slice()
		imag := fluid.psiImagHeads[head].Float32Slice()

		for mode, value := range real {
			coherence += float64(value)*float64(value) + float64(imag[mode])*float64(imag[mode])
		}
	}

	modes := float64(fluid.domain.MaxModes * spectralHeads)
	return Reading{
		Health:           fluid.health,
		GuidanceSpeed:    math.Sqrt(speed2 / count),
		Divergence:       divAbs / count,
		PressureGradNorm: math.Sqrt(press2 / count),
		ViscosityProxy:   fluid.domain.Mu * math.Sqrt(strain2/count),
		CoherenceMag2:    coherence / modes,
		KuramotoR:        kuramotoFromPhase(fluid.phase, fluid.particles),
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
