package sensorium

import (
	"math"
)

func (fluid *workspace) step() Reading {
	fluid.scatterParticles()
	fluid.gasRK2()

	if fluid.particles > 0 {
		fluid.gatherParticles()
		fluid.planckExchange()
		fluid.waveStep()
	}

	fluid.psiRe.Zero()
	fluid.psiIm.Zero()
	return fluid.observe()
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
}

func (fluid *workspace) gasRK2() {
	dt := float32(fluid.rates.deltaT)
	gamma := float32(fluid.domain.Gamma)
	cv := float32(fluid.domain.CV)
	rhoMin := float32(fluid.domain.RhoMin)
	pMin := float32(fluid.domain.PMin)
	mu := float32(fluid.domain.Mu)
	kThermal := float32(fluid.domain.KThermal)
	engine := fluid.engine
	engine.GasRK2Stage1(
		fluid.rho, fluid.mom, fluid.energy,
		fluid.rho1, fluid.mom1, fluid.energy1,
		fluid.k1Rho, fluid.k1Mom, fluid.k1Energy,
		fluid.dbgHead, fluid.dbgWords, 0,
		dt, gamma, cv, rhoMin, pMin, mu, kThermal,
	)
	engine.GasRK2Stage2(
		fluid.rho, fluid.mom, fluid.energy,
		fluid.rho1, fluid.mom1, fluid.energy1,
		fluid.k1Rho, fluid.k1Mom, fluid.k1Energy,
		fluid.rho2, fluid.mom2, fluid.energy2,
		fluid.dbgHead, fluid.dbgWords, 0,
		dt, gamma, cv, rhoMin, pMin, mu, kThermal,
	)
	engine.Synchronize()
	copy(fluid.rho.Float32Slice(), fluid.rho2.Float32Slice())
	copy(fluid.mom.Float32Slice(), fluid.mom2.Float32Slice())
	copy(fluid.energy.Float32Slice(), fluid.energy2.Float32Slice())
}

func (fluid *workspace) gatherParticles() {
	dt := float32(fluid.rates.deltaT)
	fluid.engine.PICGatherUpdate(
		fluid.pos, fluid.mass, fluid.posOut, fluid.velOut, fluid.heatOut,
		fluid.rho, fluid.mom, fluid.energy, fluid.gravity,
		fluid.dbgHead, fluid.dbgWords, 0,
		dt,
		float32(fluid.domain.Gamma),
		float32(fluid.domain.RSpecific),
		float32(fluid.domain.CV),
		float32(fluid.domain.RhoMin),
		float32(fluid.domain.PMin),
		0,
	)
	fluid.engine.Synchronize()
	copy(fluid.pos.Float32Slice(), fluid.posOut.Float32Slice())
	copy(fluid.vel.Float32Slice(), fluid.velOut.Float32Slice())
	copy(fluid.heat.Float32Slice(), fluid.heatOut.Float32Slice())
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
		denom := particleMass * cv
		temperature := 0.0

		if denom > 0 {
			temperature = thermal / denom
		}

		eq := planckEnergy(freq, temperature)
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

		heat[index] = float32(thermal - delta)
		energy[index] = float32(osc + delta)
		amp[index] = float32(math.Sqrt(math.Max(0, osc+delta)))
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
	incoming := make([]float32, fluid.particles)
	delta := make([]float32, fluid.particles)
	budget := 1 / float32(spectralHeads)
	domainMin := min(fluid.domain.DomainX, fluid.domain.DomainY, fluid.domain.DomainZ)
	kappaPhysical := 1.0

	if sigma > 0 && domainMin > 0 {
		kappaPhysical = (domainMin / sigma) * (domainMin / sigma)
	}

	couplingScale := float32(kappaPhysical / math.Sqrt(float64(modes*spectralHeads)))

	for head := 0; head < spectralHeads; head++ {
		offset := float64(head) * (2 * math.Pi) / float64(spectralHeads)
		fluid.accums.Zero()

		for particle := 0; particle < fluid.particles; particle++ {
			headHeat[particle] = heat[particle] * budget
			wrapped := wrapPhase(float64(phase[particle]) + offset)
			headPhase[particle] = float32(wrapped)
			incoming[particle] = float32(wrapped)
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

		fluid.engine.CoherenceUpdateOscillatorPhases(
			fluid.headPhase, fluid.omega, fluid.couplingAmp,
			fluid.psiRealHeads[head], fluid.psiImagHeads[head],
			fluid.omegaLattice, fluid.gateWidth,
			fluid.anchorIdx, fluid.anchorWeight, fluid.numCarriers,
			fluid.particles, modes,
			dt, couplingScale, gateMin, gateMax,
			fluid.binStarts, fluid.binnedIdx, fluid.binParams,
			modes,
			fluid.pos,
			float32(sigma),
			float32(fluid.rates.metabolicRate),
			weightFloor,
		)

		fluid.engine.Synchronize()

		for particle := 0; particle < fluid.particles; particle++ {
			delta[particle] += float32(wrapDelta(float64(headPhase[particle]), float64(incoming[particle])))
		}

		fluid.rngSeed++
	}

	heads := float64(spectralHeads)

	for particle := 0; particle < fluid.particles; particle++ {
		phase[particle] = float32(wrapPhase(float64(phase[particle]) + float64(delta[particle])/heads))
	}
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
	denom := massMean * tempMean

	if !(denom > 0) {
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
		KuramotoR:        kuramotoFromPhase(fluid.phase),
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

func kuramotoFromPhase(phase *Buffer) float64 {
	if phase == nil {
		return 0
	}

	values := phase.Float32Slice()

	if len(values) == 0 {
		return 0
	}

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

func wrapDelta(now, previous float64) float64 {
	return wrapPhase(now-previous+math.Pi) - math.Pi
}

func exclusiveScanU32(counts, starts []uint32) {
	var total uint32

	for index, count := range counts {
		starts[index] = total
		total += count
	}
}
