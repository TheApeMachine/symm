package sensorium

import (
	"fmt"
	"math"
	"sort"
)

func (fluid *workspace) step() (Reading, error) {
	fluid.scatterParticles()

	if err := fluid.gasRK2(); err != nil {
		return Reading{}, err
	}

	if fluid.particles == 0 {
		fluid.psiRe.Zero()
		fluid.psiIm.Zero()
		return fluid.observe(), nil
	}

	if err := fluid.gatherParticles(); err != nil {
		return Reading{}, err
	}

	fluid.planckExchange()

	// 1. Seed particle anchors into the ω-frequency lattice
	fluid.seedModeAnchors()

	// 2. Step the Gross-Pitaevskii Equation across the spectral heads
	//    and pull particle oscillator phases towards resonant modes (Kuramoto sync)
	fluid.waveStep()

	// 3. Project the coherent mode amplitudes Ψ_k into the 3D spatial field Ψ(x)
	fluid.projectSpatialWave()

	return fluid.observe(), nil
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

func (fluid *workspace) scatterParticles() {
	fluid.engine.ClearField(fluid.rho)
	fluid.engine.ClearField(fluid.mom)
	fluid.engine.ClearField(fluid.energy)

	if fluid.particles == 0 {
		return
	}

	engine := fluid.engine
	engine.ScatterComputeCellIdx(fluid.pos, fluid.cellIdx)
	fluid.cellCounts.Zero()
	engine.ScatterCountCells(fluid.cellIdx, fluid.cellCounts)
	engine.Synchronize()
	exclusiveScanU32(fluid.cellCounts.UInt32Slice(), fluid.cellStarts.UInt32Slice())
	fluid.cellOffsets.Zero()

	engine.ScatterReorderParticles(
		fluid.pos,
		fluid.vel,
		fluid.mass,
		fluid.heat,
		fluid.oscEnergy,
		fluid.cellIdx,
		fluid.cellStarts,
		fluid.cellOffsets,
		fluid.sortedPos,
		fluid.sortedVel,
		fluid.sortedMass,
		fluid.sortedHeat,
		fluid.sortedEnergy,
		fluid.originalIdx,
	)
	engine.ScatterSorted(
		fluid.sortedPos,
		fluid.sortedVel,
		fluid.sortedMass,
		fluid.sortedHeat,
		fluid.sortedEnergy,
		fluid.rho,
		fluid.mom,
		fluid.energy,
		fluid.particles,
	)
	engine.Synchronize()
}

/*
gasRK2 advances the Eulerian grid one RK2 step with adaptive Δt halving.

This mirrors the Python reference (thermodynamics.py _run_gas_rk2): when the
Metal shader detects inadmissible cells (tag 0x13, 0x12, etc.) it poisons the
output with NaN and logs the event. The host detects the rejection by checking
the debug event counter, halves dt, and retries — standard RK2 step rejection,
not a clamp or fallback. The input buffers (rho, mom, energy) are read-only in
both stages, so retrying is safe.

Rejection attempts are silent: drainDebug is only called on the final failure
path so the error log contains the actual fatal event rather than a wall of
expected transient rejections.
*/
func (fluid *workspace) gasRK2() error {
	delta := float32(fluid.rates.deltaT)
	maxHalvings := 10

	for halvings := 0; ; halvings++ {
		// Clear the debug event buffer before each attempt so a retry does
		// not re-read stale rejection events from a prior attempt.
		fluid.dbgHead.UInt32Slice()[0] = 0

		fluid.dispatchGasStageOne(delta)
		fluid.dispatchGasStageTwo(delta)

		// Check the debug counter directly — if zero, no inadmissible cells
		// were logged and the step is accepted. This avoids calling drainDebug
		// (which logs via errnie.Error) on every transient retry.
		if fluid.dbgHead.UInt32Slice()[0] == 0 {
			break
		}

		if halvings >= maxHalvings {
			// Drain the final failure's debug buffer so the error message
			// contains the actual cell and tag that failed.
			_ = fluid.drainDebug()

			return fmt.Errorf(
				"gas RK2 failed after %d dt halvings (dt=%.6g)", maxHalvings, delta,
			)
		}

		// Reset the counter (already done at loop top) and retry with half dt.
		delta *= 0.5
	}

	fluid.acceptGasStep()

	return nil
}

func (fluid *workspace) dispatchGasStageOne(delta float32) {
	fluid.engine.GasRK2Stage1(
		fluid.rho,
		fluid.mom,
		fluid.energy,
		fluid.rho1, fluid.mom1, fluid.energy1,
		fluid.k1Rho, fluid.k1Mom, fluid.k1Energy,
		fluid.dbgHead, fluid.dbgWords, dbgCapacity,
		delta,
		float32(fluid.domain.Gamma),
		float32(fluid.domain.CV),
		float32(fluid.domain.RhoMin),
		float32(fluid.domain.PMin),
		float32(fluid.domain.Mu),
		float32(fluid.domain.KThermal),
	)
	fluid.engine.Synchronize()
}

func (fluid *workspace) dispatchGasStageTwo(delta float32) {
	fluid.engine.GasRK2Stage2(
		fluid.rho, fluid.mom, fluid.energy,
		fluid.rho1, fluid.mom1, fluid.energy1,
		fluid.k1Rho, fluid.k1Mom, fluid.k1Energy,
		fluid.rho2, fluid.mom2, fluid.energy2,
		fluid.dbgHead, fluid.dbgWords, dbgCapacity,
		delta,
		float32(fluid.domain.Gamma),
		float32(fluid.domain.CV),
		float32(fluid.domain.RhoMin),
		float32(fluid.domain.PMin),
		float32(fluid.domain.Mu),
		float32(fluid.domain.KThermal),
	)
	fluid.engine.Synchronize()
}

func (fluid *workspace) acceptGasStep() {
	copy(fluid.rho.Float32Slice(), fluid.rho2.Float32Slice())
	copy(fluid.mom.Float32Slice(), fluid.mom2.Float32Slice())
	copy(fluid.energy.Float32Slice(), fluid.energy2.Float32Slice())
}

func (fluid *workspace) gatherParticles() error {
	dt := float32(fluid.rates.deltaT)
	fluid.engine.PICGatherUpdate(
		fluid.pos, fluid.mass, fluid.posOut, fluid.velOut, fluid.heatOut,
		fluid.rho, fluid.mom, fluid.energy, fluid.gravity,
		fluid.dbgHead, fluid.dbgWords, dbgCapacity,
		dt,
		float32(fluid.domain.Gamma),
		float32(fluid.domain.RSpecific),
		float32(fluid.domain.CV),
		float32(fluid.domain.RhoMin),
		float32(fluid.domain.PMin),
		0,
	)
	fluid.engine.Synchronize()

	if err := fluid.drainDebug(); err != nil {
		return err
	}

	copy(fluid.pos.Float32Slice(), fluid.posOut.Float32Slice())
	copy(fluid.vel.Float32Slice(), fluid.velOut.Float32Slice())
	copy(fluid.heat.Float32Slice(), fluid.heatOut.Float32Slice())

	return nil
}

func (fluid *workspace) planckExchange() {
	dt := fluid.rates.deltaT
	cv := fluid.domain.CV
	kappa := fluid.domain.KThermal
	radius := 0.5 * fluid.domain.GridSpacing()
	heat := fluid.heat.Float32Slice()
	energy := fluid.oscEnergy.Float32Slice()
	omega := fluid.omega.Float32Slice()
	mass := fluid.mass.Float32Slice()
	amp := fluid.amp.Float32Slice()

	for index := 0; index < fluid.particles; index++ {
		particleMass := float64(mass[index])
		thermal := float64(heat[index])
		osc := float64(energy[index])
		freq := float64(omega[index])

		if math.IsNaN(thermal) || thermal < 0 {
			panic(fmt.Sprintf("sensorium: invalid thermal energy %v for particle %d", thermal, index))
		}

		if math.IsNaN(osc) || osc < 0 {
			panic(fmt.Sprintf("sensorium: invalid oscillator energy %v for particle %d", osc, index))
		}

		denom := particleMass * cv
		temperature := 0.0

		if denom > 0 {
			temperature = thermal / denom
		}

		eq := planckEnergy(math.Abs(freq), temperature)
		alpha := 0.0

		if kappa > 0 && radius > 0 && denom > 0 {
			tau := denom / (4 * math.Pi * kappa * radius)
			alpha = 1 - math.Exp(-dt/tau)
		}

		delta := alpha * (eq - osc)

		if delta > thermal {
			delta = thermal
		}
		if -delta > osc {
			delta = -osc
		}

		nextHeat := float32(thermal - delta)
		nextOsc := float32(osc + delta)
		if nextHeat < 0 || math.IsNaN(float64(nextHeat)) {
			nextHeat = 0
		}
		if nextOsc < 0 || math.IsNaN(float64(nextOsc)) {
			nextOsc = 0
		}

		heat[index] = nextHeat
		energy[index] = nextOsc
		amp[index] = float32(math.Sqrt(float64(nextOsc)))
	}
}

func planckEnergy(omega, temperature float64) float64 {
	if temperature <= 0 || omega == 0 {
		return 0
	}

	ratio := omega / temperature

	if ratio < 1e-4 {
		return temperature
	}

	if ratio > 50 {
		return omega * math.Exp(-ratio)
	}

	return omega / (math.Exp(ratio) - 1)
}

func isPositiveFinite(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func (fluid *workspace) waveStep() {
	dt := float32(fluid.rates.deltaT)
	modes := int(fluid.domain.MaxModes)
	sigma := fluid.spatialSigma()
	weightFloor := float32(math.Sqrt(1.0 / float64(uint32(1)<<23)))
	anchorEps := weightFloor
	gateMin := float32(fluid.domain.linewidthMin())
	gateMax := float32(fluid.domain.linewidthMax())
	fluid.writeCouplingAmp()
	heat := fluid.heat.Float32Slice()
	phase := fluid.phase.Float32Slice()
	headHeat := fluid.headHeat.Float32Slice()
	headPhase := fluid.headPhase.Float32Slice()
	budget := 1 / float32(spectralHeads)

	// Step each spectral head independently under the GPE
	for head := 0; head < spectralHeads; head++ {
		offset := float64(head) * (2 * math.Pi) / float64(spectralHeads)
		fluid.accums.Zero()

		for particle := 0; particle < fluid.particles; particle++ {
			headHeat[particle] = heat[particle] * budget
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

		fluid.engine.CoherenceGPEStep(
			fluid.headPhase, fluid.omega, fluid.couplingAmp,
			fluid.psiRealHeads[head], fluid.psiImagHeads[head],
			fluid.omegaLattice, fluid.gateWidth,
			fluid.kineticReal, fluid.kineticImag,
			fluid.anchorIdx, fluid.anchorWeight, fluid.accums,
			fluid.numCarriers, fluid.pos,
			nil,
			fluid.particles, modes,
			dt,
			float32(hbarEff), float32(massEff),
			float32(fluid.rates.gInteraction),
			float32(fluid.rates.energyDecay),
			0,
			float32(fluid.domain.invDomega2()),
			fluid.rngSeed+uint32(head),
			anchorEps,
			0,
			float32(fluid.rates.metabolicRate),
			gateMin, gateMax, weightFloor, float32(sigma),
		)

		fluid.engine.Synchronize()

		fluid.rngSeed++
	}

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
}

func (fluid *workspace) writeCouplingAmp() {
	omega := fluid.omega.Float32Slice()
	amp := fluid.amp.Float32Slice()
	out := fluid.couplingAmp.Float32Slice()
	omegaMin := omega[0]

	for _, value := range omega {
		if value < omegaMin {
			omegaMin = value
		}
	}

	floor := float32(fluid.domain.binWidth())

	if floor == 0 {
		floor = float32(fluid.domain.OmegaMin)
	}

	var mean float32

	for index, value := range omega {
		rel := value - omegaMin + floor

		if rel < floor {
			rel = floor
		}

		weight := float32(1 / math.Sqrt(float64(rel)))
		out[index] = weight
		mean += weight
	}

	mean /= float32(len(omega))

	for index := range out {
		out[index] = amp[index] * out[index] / mean
	}
}

func (fluid *workspace) spatialSigma() float64 {
	sigmaMax := 0.5 * min(fluid.domain.DomainX, fluid.domain.DomainY, fluid.domain.DomainZ)
	sigmaMin := fluid.domain.GridSpacing()
	cv := fluid.domain.CV

	if fluid.particles == 0 || !(cv > 0) {
		return sigmaMax
	}

	heat := fluid.heat.Float32Slice()
	mass := fluid.mass.Float32Slice()
	var tempSum, massSum float64
	var counted int

	for index := 0; index < fluid.particles; index++ {
		if mass[index] <= 0 {
			continue
		}

		tempSum += float64(heat[index]) / (float64(mass[index]) * cv)
		massSum += float64(mass[index])
		counted++
	}

	if counted == 0 {
		return sigmaMax
	}

	tempMean := tempSum / float64(counted)
	massMean := massSum / float64(counted)

	// A determined (non-zero, finite) mean temperature is what makes sigma a
	// length. Zero mean temperature means the field has no thermal length scale
	// yet, so it saturates the coupling range instead of dividing by zero.
	if !isPositiveFinite(tempMean) {
		return sigmaMax
	}

	denom := massMean * tempMean

	if !isPositiveFinite(denom) {
		return sigmaMax
	}

	sigma := math.Sqrt(2*math.Pi) / math.Sqrt(denom)
	return math.Min(sigmaMax, math.Max(sigmaMin, sigma))
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

func exclusiveScanU32(counts, starts []uint32) {
	var total uint32

	for index, count := range counts {
		starts[index] = total
		total += count
	}
}
