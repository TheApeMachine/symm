package sensorium

import (
	"math"
)

const (
	spectralHeads = 8
	modeAnchors   = 8
	gInteraction  = -1.0
	metabolicRate = 0.5
	hbarEff       = 1.0
	massEff       = 1.0
	airGamma      = 1.4
	gridViscosity = 1e-4
	airPrandtl    = 0.71
	floorDensity  = 1e-3
	floorPressure = 1e-3
	dtMax         = 0.015
)

type workspaceGrid struct {
	GridX, GridY, GridZ        int
	DomainX, DomainY, DomainZ  float64
	DeltaT                     float64
	MaxModes                   int
	Gamma, CV, RSpecific       float64
	RhoMin, PMin, Mu, KThermal float64
	OmegaMin, OmegaMax         float64
}

func derivedModes(maximumAxis int) int {
	bits := 0

	for shifted := maximumAxis - 1; shifted > 0; shifted >>= 1 {
		bits++
	}

	if bits == 0 {
		return 1
	}

	return 1 << bits
}

func newWorkspaceGrid(gx, gy, gz int) workspaceGrid {
	maximumAxis := max(gx, gy, gz)
	spacing := 1 / float64(maximumAxis)
	gamma := airGamma
	rSpecific := gamma - 1
	cP := gamma * rSpecific / (gamma - 1)
	modes := derivedModes(maximumAxis)
	deltaT := spacing

	if deltaT > dtMax {
		deltaT = dtMax
	}

	return workspaceGrid{
		GridX:     gx,
		GridY:     gy,
		GridZ:     gz,
		DomainX:   float64(gx) * spacing,
		DomainY:   float64(gy) * spacing,
		DomainZ:   float64(gz) * spacing,
		DeltaT:    deltaT,
		MaxModes:  modes,
		Gamma:     gamma,
		CV:        1,
		RSpecific: rSpecific,
		RhoMin:    floorDensity,
		PMin:      floorPressure,
		Mu:        gridViscosity,
		KThermal:  (gridViscosity * cP) / airPrandtl,
		OmegaMin:  -4,
		OmegaMax:  4,
	}
}

func (grid workspaceGrid) GridSpacing() float64 {
	return 1 / float64(max(grid.GridX, grid.GridY, grid.GridZ))
}

func (grid workspaceGrid) CellCount() int {
	return grid.GridX * grid.GridY * grid.GridZ
}

func (grid workspaceGrid) binWidth() float64 {
	if grid.MaxModes < 2 {
		return 0
	}

	return (grid.OmegaMax - grid.OmegaMin) / float64(grid.MaxModes-1)
}

func (grid workspaceGrid) linewidthMin() float64 {
	domega := grid.binWidth()

	if domega > 0 {
		return 0.25 * domega
	}

	return grid.OmegaMin
}

func (grid workspaceGrid) linewidthMax() float64 {
	domega := grid.binWidth()

	if domega > 0 {
		return 4 * domega
	}

	return grid.OmegaMax
}

func (grid workspaceGrid) invDomega2() float64 {
	domega := grid.binWidth()

	if domega == 0 {
		return 0
	}

	return 1 / (domega * domega)
}

/*
workspace is the Metal buffer owner: scatter → gas RK2 → gather → GPE.
*/
type workspace struct {
	engine    *Engine
	domain    workspaceGrid
	particles int
	rngSeed   uint32
	rates     stepRates

	rho, mom, energy                    *Buffer
	rho1, mom1, energy1                 *Buffer
	rho2, mom2, energy2                 *Buffer
	k1Rho, k1Mom, k1Energy              *Buffer
	gravity                             *Buffer
	cellCounts, cellStarts, cellOffsets *Buffer
	psiRe, psiIm                        *Buffer
	dbgHead, dbgWords                   *Buffer

	omegaLattice, gateWidth  *Buffer
	accums                   *Buffer
	numCarriers              *Buffer
	anchorIdx, anchorWeight  *Buffer
	binStarts, binnedIdx     *Buffer
	binParams                *Buffer
	psiRealHeads             []*Buffer
	psiImagHeads             []*Buffer
	psiModeReal, psiModeImag *Buffer
	headPhase, headHeat      *Buffer
	couplingAmp              *Buffer

	pos, vel, mass, heat, oscEnergy      *Buffer
	phase, omega, amp                    *Buffer
	posOut, velOut, heatOut              *Buffer
	cellIdx, originalIdx                 *Buffer
	sortedPos, sortedVel                 *Buffer
	sortedMass, sortedHeat, sortedEnergy *Buffer
}

type stepRates struct {
	deltaT, energyDecay, metabolicRate, gInteraction float64
}

type fieldScale struct {
	density, momentum, energy, wave float32
}

func newWorkspace(gx, gy, gz int) (*workspace, error) {
	grid := newWorkspaceGrid(gx, gy, gz)
	engine, err := NewEmbeddedEngine(gx, gy, gz, float32(grid.GridSpacing()))

	if err != nil {
		return nil, err
	}

	fluid := &workspace{
		engine:  engine,
		domain:  grid,
		rngSeed: 1,
		rates: stepRates{
			deltaT:        grid.DeltaT,
			energyDecay:   1 / float64(grid.MaxModes),
			metabolicRate: metabolicRate,
			gInteraction:  gInteraction,
		},
	}
	fluid.allocateGrid()
	fluid.seedLattice()

	return fluid, nil
}

func (fluid *workspace) Close() {
	if fluid == nil {
		return
	}

	for _, buffer := range fluid.allBuffers() {
		if buffer != nil {
			buffer.Close()
		}
	}

	if fluid.engine != nil {
		fluid.engine.Close()
		fluid.engine = nil
	}
}

func (fluid *workspace) gpu(bytes uint64) *Buffer {
	buffer := fluid.engine.NewBuffer(bytes, nil)
	buffer.Adopt()
	return buffer
}

func (fluid *workspace) allocateGrid() {
	cells := uint64(fluid.domain.CellCount())
	modes := uint64(fluid.domain.MaxModes)
	fluid.rho = fluid.gpu(cells * 4)
	fluid.mom = fluid.gpu(cells * 3 * 4)
	fluid.energy = fluid.gpu(cells * 4)
	fluid.rho1 = fluid.gpu(cells * 4)
	fluid.mom1 = fluid.gpu(cells * 3 * 4)
	fluid.energy1 = fluid.gpu(cells * 4)
	fluid.rho2 = fluid.gpu(cells * 4)
	fluid.mom2 = fluid.gpu(cells * 3 * 4)
	fluid.energy2 = fluid.gpu(cells * 4)
	fluid.k1Rho = fluid.gpu(cells * 4)
	fluid.k1Mom = fluid.gpu(cells * 3 * 4)
	fluid.k1Energy = fluid.gpu(cells * 4)
	fluid.gravity = fluid.gpu(cells * 4)
	fluid.cellCounts = fluid.gpu(cells * 4)
	fluid.cellStarts = fluid.gpu(cells * 4)
	fluid.cellOffsets = fluid.gpu(cells * 4)
	fluid.psiRe = fluid.gpu(cells * 4)
	fluid.psiIm = fluid.gpu(cells * 4)
	fluid.dbgHead = fluid.gpu(4)
	fluid.dbgWords = fluid.gpu(6 * 4)
	fluid.omegaLattice = fluid.gpu(modes * 4)
	fluid.gateWidth = fluid.gpu(modes * 4)
	fluid.accums = fluid.gpu(modes * 8 * 4)
	fluid.numCarriers = fluid.gpu(4)
	fluid.anchorIdx = fluid.gpu(modes * uint64(modeAnchors) * 4)
	fluid.anchorWeight = fluid.gpu(modes * uint64(modeAnchors) * 4)
	fluid.binStarts = fluid.gpu((modes + 1) * 4)
	fluid.binnedIdx = fluid.gpu(modes * 4)
	fluid.binParams = fluid.gpu(2 * 4)
	fluid.psiRealHeads = make([]*Buffer, spectralHeads)
	fluid.psiImagHeads = make([]*Buffer, spectralHeads)

	for head := 0; head < spectralHeads; head++ {
		fluid.psiRealHeads[head] = fluid.gpu(modes * 4)
		fluid.psiImagHeads[head] = fluid.gpu(modes * 4)
	}

	fluid.psiModeReal = fluid.gpu(modes * 4)
	fluid.psiModeImag = fluid.gpu(modes * 4)

	fluid.gravity.Zero()
	fluid.dbgHead.Zero()
	fluid.dbgWords.Zero()
	fluid.anchorIdx.Zero()
	fillInt32(fluid.anchorIdx.Int32Slice(), -1)
	fluid.anchorWeight.Zero()
}

func (fluid *workspace) seedLattice() {
	modes := int(fluid.domain.MaxModes)
	omega := fluid.omegaLattice.Float32Slice()
	width := fluid.gateWidth.Float32Slice()
	starts := fluid.binStarts.Int32Slice()
	index := fluid.binnedIdx.Int32Slice()
	params := fluid.binParams.Float32Slice()
	domega := float32(fluid.domain.binWidth())

	for mode := 0; mode < modes; mode++ {
		omega[mode] = float32(fluid.domain.OmegaMin) + float32(mode)*domega
		width[mode] = domega
		index[mode] = int32(mode)
		starts[mode] = int32(mode)
	}

	starts[modes] = int32(modes)
	params[0] = float32(fluid.domain.OmegaMin)

	if domega != 0 {
		params[1] = 1 / domega
	}

	fluid.numCarriers.Int32Slice()[0] = int32(modes)
}

func (fluid *workspace) loadState(state *State) {
	if state == nil || state.N == 0 {
		fluid.allocateParticles(0)
		fluid.particles = 0
		return
	}

	fluid.allocateParticles(state.N)
	fluid.particles = state.N
	copy(fluid.pos.Float32Slice(), state.Pos)
	copy(fluid.vel.Float32Slice(), state.Vel)
	copy(fluid.mass.Float32Slice(), state.Mass)
	copy(fluid.heat.Float32Slice(), state.Heat)
	copy(fluid.oscEnergy.Float32Slice(), state.Energy)
	copy(fluid.phase.Float32Slice(), state.Phase)
	copy(fluid.omega.Float32Slice(), state.Omega)
	amp := fluid.amp.Float32Slice()

	for index := 0; index < state.N; index++ {
		energy := state.Energy[index]

		if energy < 0 {
			energy = 0
		}

		amp[index] = float32(math.Sqrt(float64(energy)))
	}
}

func (fluid *workspace) storeState(state *State) {
	if state == nil || fluid.particles == 0 {
		return
	}

	if state.N != fluid.particles {
		resized := newState(fluid.particles)
		copy(resized.Bytes, state.Bytes)
		copy(resized.Seqs, state.Seqs)
		copy(resized.TokenIDs, state.TokenIDs)
		copy(resized.ContentIDs, state.ContentIDs)
		copy(resized.Clamped, state.Clamped)
		copy(resized.Dark, state.Dark)
		*state = *resized
	}

	fluid.engine.Synchronize()
	copy(state.Pos, fluid.pos.Float32Slice())
	copy(state.Vel, fluid.vel.Float32Slice())
	copy(state.Mass, fluid.mass.Float32Slice())
	copy(state.Heat, fluid.heat.Float32Slice())
	copy(state.Energy, fluid.oscEnergy.Float32Slice())
	copy(state.Phase, fluid.phase.Float32Slice())
	copy(state.Omega, fluid.omega.Float32Slice())
}

func (fluid *workspace) packFields(momRho, energy, waveReal, waveImag []float32) fieldScale {
	fluid.engine.Synchronize()
	cells := fluid.domain.CellCount()
	rho := fluid.rho.Float32Slice()
	mom := fluid.mom.Float32Slice()
	internal := fluid.energy.Float32Slice()
	psiRe := fluid.psiRe.Float32Slice()
	psiIm := fluid.psiIm.Float32Slice()
	var scale fieldScale

	for cell := 0; cell < cells; cell++ {
		mx := mom[cell*3+0]
		my := mom[cell*3+1]
		mz := mom[cell*3+2]
		density := rho[cell]
		momRho[cell*4+0] = mx
		momRho[cell*4+1] = my
		momRho[cell*4+2] = mz
		momRho[cell*4+3] = density
		energy[cell] = internal[cell]
		waveReal[cell] = psiRe[cell]
		waveImag[cell] = psiIm[cell]
		scale.momentum = maxAbs32(scale.momentum, mx, my, mz)
		scale.density = maxAbs32(scale.density, density)
		scale.energy = maxAbs32(scale.energy, internal[cell])
		scale.wave = maxAbs32(scale.wave, psiRe[cell], psiIm[cell])
	}

	return scale
}

func (fluid *workspace) allocateParticles(count int) {
	if count == fluid.particles && fluid.pos != nil {
		return
	}

	fluid.closeParticles()

	if count == 0 {
		return
	}

	n := uint64(count)
	fluid.pos = fluid.gpu(n * 3 * 4)
	fluid.vel = fluid.gpu(n * 3 * 4)
	fluid.mass = fluid.gpu(n * 4)
	fluid.heat = fluid.gpu(n * 4)
	fluid.oscEnergy = fluid.gpu(n * 4)
	fluid.phase = fluid.gpu(n * 4)
	fluid.omega = fluid.gpu(n * 4)
	fluid.amp = fluid.gpu(n * 4)
	fluid.posOut = fluid.gpu(n * 3 * 4)
	fluid.velOut = fluid.gpu(n * 3 * 4)
	fluid.heatOut = fluid.gpu(n * 4)
	fluid.cellIdx = fluid.gpu(n * 4)
	fluid.originalIdx = fluid.gpu(n * 4)
	fluid.sortedPos = fluid.gpu(n * 3 * 4)
	fluid.sortedVel = fluid.gpu(n * 3 * 4)
	fluid.sortedMass = fluid.gpu(n * 4)
	fluid.sortedHeat = fluid.gpu(n * 4)
	fluid.sortedEnergy = fluid.gpu(n * 4)
	fluid.headPhase = fluid.gpu(n * 4)
	fluid.headHeat = fluid.gpu(n * 4)
	fluid.couplingAmp = fluid.gpu(n * 4)
}

func (fluid *workspace) closeParticles() {
	for _, buffer := range []*Buffer{
		fluid.headPhase, fluid.headHeat, fluid.couplingAmp,
		fluid.pos, fluid.vel, fluid.mass, fluid.heat, fluid.oscEnergy,
		fluid.phase, fluid.omega, fluid.amp,
		fluid.posOut, fluid.velOut, fluid.heatOut,
		fluid.cellIdx, fluid.originalIdx,
		fluid.sortedPos, fluid.sortedVel,
		fluid.sortedMass, fluid.sortedHeat, fluid.sortedEnergy,
	} {
		if buffer != nil {
			buffer.Close()
		}
	}

	fluid.headPhase = nil
	fluid.headHeat = nil
	fluid.couplingAmp = nil
	fluid.pos = nil
	fluid.vel = nil
	fluid.mass = nil
	fluid.heat = nil
	fluid.oscEnergy = nil
	fluid.phase = nil
	fluid.omega = nil
	fluid.amp = nil
	fluid.posOut = nil
	fluid.velOut = nil
	fluid.heatOut = nil
	fluid.cellIdx = nil
	fluid.originalIdx = nil
	fluid.sortedPos = nil
	fluid.sortedVel = nil
	fluid.sortedMass = nil
	fluid.sortedHeat = nil
	fluid.sortedEnergy = nil
}

func (fluid *workspace) allBuffers() []*Buffer {
	buffers := []*Buffer{
		fluid.rho, fluid.mom, fluid.energy,
		fluid.rho1, fluid.mom1, fluid.energy1,
		fluid.rho2, fluid.mom2, fluid.energy2,
		fluid.k1Rho, fluid.k1Mom, fluid.k1Energy,
		fluid.gravity,
		fluid.cellCounts, fluid.cellStarts, fluid.cellOffsets,
		fluid.psiRe, fluid.psiIm,
		fluid.dbgHead, fluid.dbgWords,
		fluid.omegaLattice, fluid.gateWidth,
		fluid.accums, fluid.numCarriers,
		fluid.anchorIdx, fluid.anchorWeight,
		fluid.psiModeReal, fluid.psiModeImag,
		fluid.binStarts, fluid.binnedIdx, fluid.binParams,
		fluid.headPhase, fluid.headHeat, fluid.couplingAmp,
		fluid.pos, fluid.vel, fluid.mass, fluid.heat, fluid.oscEnergy,
		fluid.phase, fluid.omega, fluid.amp,
		fluid.posOut, fluid.velOut, fluid.heatOut,
		fluid.cellIdx, fluid.originalIdx,
		fluid.sortedPos, fluid.sortedVel,
		fluid.sortedMass, fluid.sortedHeat, fluid.sortedEnergy,
	}
	buffers = append(buffers, fluid.psiRealHeads...)
	buffers = append(buffers, fluid.psiImagHeads...)
	return buffers
}

func fillInt32(values []int32, fill int32) {
	for index := range values {
		values[index] = fill
	}
}

func maxAbs32(peak float32, values ...float32) float32 {
	for _, value := range values {
		abs := float32(math.Abs(float64(value)))

		if abs > peak {
			peak = abs
		}
	}

	return peak
}
