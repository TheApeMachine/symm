package sensorium

import (
	"math"
	"sort"
)

func (fluid *workspace) step() Reading {
	fluid.scatterParticles()
	fluid.gasRK2()

	if fluid.particles == 0 {
		fluid.psiRe.Zero()
		fluid.psiIm.Zero()
		return fluid.observe()
	}

	fluid.projectSpatialWave()
	fluid.gatherParticles()
	fluid.planckExchange()
	fluid.waveStep()
	return fluid.observe()
}

/*
projectSpatialWave rebuilds Ψ(x) as a CIC of each oscillator's phasor
m e^{iθ}. GPE stays on the 1D ω-lattice; its heads are not a spatial field.
*/
func (fluid *workspace) projectSpatialWave() {
	fluid.psiRe.Zero()
	fluid.psiIm.Zero()
	fluid.splatParticleWave()
}

func (fluid *workspace) splatParticleWave() {
	if fluid.particles == 0 {
		return
	}

	gx := fluid.domain.GridX
	gy := fluid.domain.GridY
	gz := fluid.domain.GridZ
	inv := 1 / fluid.domain.GridSpacing()
	pos := fluid.pos.Float32Slice()
	mass := fluid.mass.Float32Slice()
	phase := fluid.phase.Float32Slice()
	psiRe := fluid.psiRe.Float32Slice()
	psiIm := fluid.psiIm.Float32Slice()

	for particle := 0; particle < fluid.particles; particle++ {
		deposit := float64(mass[particle])
		angle := float64(phase[particle])
		real := float32(deposit * math.Cos(angle))
		imag := float32(deposit * math.Sin(angle))
		gridX := float64(pos[particle*3+0]) * inv
		gridY := float64(pos[particle*3+1]) * inv
		gridZ := float64(pos[particle*3+2]) * inv
		ix0 := int(math.Floor(gridX))
		iy0 := int(math.Floor(gridY))
		iz0 := int(math.Floor(gridZ))
		fx := float32(gridX - math.Floor(gridX))
		fy := float32(gridY - math.Floor(gridY))
		fz := float32(gridZ - math.Floor(gridZ))
		wx0 := 1 - fx
		wy0 := 1 - fy
		wz0 := 1 - fz
		ix1 := wrapIndex(ix0+1, gx)
		iy1 := wrapIndex(iy0+1, gy)
		iz1 := wrapIndex(iz0+1, gz)
		ix0 = wrapIndex(ix0, gx)
		iy0 = wrapIndex(iy0, gy)
		iz0 = wrapIndex(iz0, gz)
		corners := [8]struct {
			x, y, z int
			weight  float32
		}{
			{ix0, iy0, iz0, wx0 * wy0 * wz0},
			{ix1, iy0, iz0, fx * wy0 * wz0},
			{ix0, iy1, iz0, wx0 * fy * wz0},
			{ix1, iy1, iz0, fx * fy * wz0},
			{ix0, iy0, iz1, wx0 * wy0 * fz},
			{ix1, iy0, iz1, fx * wy0 * fz},
			{ix0, iy1, iz1, wx0 * fy * fz},
			{ix1, iy1, iz1, fx * fy * fz},
		}

		for _, corner := range corners {
			if !(corner.weight > 0) {
				continue
			}

			cell := corner.x + gx*(corner.y+gy*corner.z)
			psiRe[cell] += real * corner.weight
			psiIm[cell] += imag * corner.weight
		}
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
	fluid.admitConserved()
}

func (fluid *workspace) admitConserved() {
	rho := fluid.rho.Float32Slice()
	mom := fluid.mom.Float32Slice()
	energy := fluid.energy.Float32Slice()
	rhoMin := float32(fluid.domain.RhoMin)

	for cell, density := range rho {
		if density > rhoMin || density < -rhoMin {
			continue
		}

		mom[cell*3+0] = 0
		mom[cell*3+1] = 0
		mom[cell*3+2] = 0
		energy[cell] = 0
	}
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
	budget := 1 / float32(spectralHeads)

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

func exclusiveScanU32(counts, starts []uint32) {
	var total uint32

	for index, count := range counts {
		starts[index] = total
		total += count
	}
}
