package sensorium

import (
	"math"
	"slices"
)

const (
	spectralHeads = 8
	modeAnchors   = 8
	gInteraction  = 0.0
	metabolicRate = 0.5
	hbarEff       = 1.0
	massEff       = 1.0
	airGamma      = 1.4
	gridViscosity = 1e-4
	airPrandtl    = 0.71
	floorDensity  = 1e-3
	floorPressure = 1e-3
	dtMax         = 0.015

	// dbgWordsPerEvent mirrors DBG_WORDS_PER_EVENT in manifold.metal: each
	// event is {tag, gid, a, b, c, d}. The two must agree or the host decodes
	// garbage.
	dbgWordsPerEvent = 6

	// dbgCapacity is how many kernel events one step may record. A step that
	// goes bad tends to go bad in many cells at once, so the buffer holds a
	// batch rather than a single event; the kernel drops the overflow and the
	// host reports how many were lost.
	dbgCapacity = 256
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
	modes := derivedModes(maximumAxis)

	// Initial macro request only; state-dependent stability is owned by the coupled controller.
	deltaT := 0.18 * spacing
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
		KThermal:  gridViscosity * gamma / airPrandtl,
		OmegaMin:  -4,
		OmegaMax:  4,
	}
}

func (workspaceGrid workspaceGrid) GridSpacing() float64 {
	return 1 / float64(max(workspaceGrid.GridX, workspaceGrid.GridY, workspaceGrid.GridZ))
}

func (workspaceGrid workspaceGrid) CellCount() int {
	return workspaceGrid.GridX * workspaceGrid.GridY * workspaceGrid.GridZ
}

func (workspaceGrid workspaceGrid) binWidth() float64 {
	if workspaceGrid.MaxModes < 2 {
		return workspaceGrid.OmegaMax - workspaceGrid.OmegaMin // one quadrature cell spans the declared interval
	}

	return (workspaceGrid.OmegaMax - workspaceGrid.OmegaMin) / float64(workspaceGrid.MaxModes-1)
}

func (workspaceGrid workspaceGrid) linewidthMin() float64 {
	domega := workspaceGrid.binWidth()

	if domega > 0 {
		return 0.25 * domega
	}

	return workspaceGrid.OmegaMin
}

func (workspaceGrid workspaceGrid) linewidthMax() float64 {
	domega := workspaceGrid.binWidth()

	if domega > 0 {
		return 4 * domega
	}

	return workspaceGrid.OmegaMax
}

func (workspaceGrid workspaceGrid) invDomega2() float64 {
	domega := workspaceGrid.binWidth()

	if domega == 0 {
		return 0
	}

	return 1 / (domega * domega)
}

/*
workspace is the Metal buffer owner: scatter → gas RK2 → gather → GPE.
*/
type workspace struct {
	spectralPotential, spectralMetric *Buffer
	waveProjectionReady               bool
	psiStartRe, psiStartIm            *Buffer
	contentIDs                        []int64

	coherencePosition, reciprocalForce, reciprocalAmplitude, reciprocalStatus *Buffer
	reciprocalOldRe, reciprocalOldIm, reciprocalPotential                     *Buffer

	engine                                                                                         *Engine
	domain                                                                                         workspaceGrid
	particles                                                                                      int
	particleCapacity                                                                               int
	rngSeed                                                                                        uint32
	rates                                                                                          stepRates
	physics                                                                                        PhysicsControls
	health                                                                                         PhysicsHealth
	physicalTime                                                                                   float64
	lastParticleEnergy                                                                             float64
	hasLastParticleEnergy                                                                          bool
	previousPotential                                                                              []float32
	lastPhaseRate                                                                                  float64
	phasePrior, phaseLedger                                                                        *Buffer
	gravityFluxBase, gravityPrior, gravityKickWork, materialEnergy, materialEnergyOut, remapReport *Buffer
	hydro, hydroOut, hydroWork1, hydroWork2, hydroStatus, hydroDiagnostics                         *Buffer
	acceleration, poissonState, poissonScratch                                                     *Buffer
	pilotPrevious, pilotReport, contactReport, particleStatus, waveLedger                          *Buffer

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
	kineticReal, kineticImag *Buffer
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
		physics: defaultPhysicsControls(),
		rates: stepRates{
			deltaT:        grid.DeltaT,
			energyDecay:   0.8,
			metabolicRate: metabolicRate,
			gInteraction:  0.0,
		},
	}
	fluid.allocateGrid()
	fluid.seedLattice()

	return fluid, nil
}

func (workspace *workspace) Close() {
	if workspace == nil {
		return
	}

	for _, buffer := range workspace.allBuffers() {
		if buffer != nil {
			buffer.Close()
		}
	}

	if workspace.engine != nil {
		workspace.engine.Close()
		workspace.engine = nil
	}
}

func (workspace *workspace) gpu(bytes uint64) *Buffer {
	buffer := workspace.engine.NewBuffer(bytes, nil)
	buffer.Adopt()
	return buffer
}

func (workspace *workspace) allocateGrid() {
	cells := uint64(workspace.domain.CellCount())
	modes := uint64(workspace.domain.MaxModes)
	workspace.rho = workspace.gpu(cells * 4)
	workspace.mom = workspace.gpu(cells * 3 * 4)
	workspace.energy = workspace.gpu(cells * 4)
	workspace.rho1 = workspace.gpu(cells * 4)
	workspace.mom1 = workspace.gpu(cells * 3 * 4)
	workspace.energy1 = workspace.gpu(cells * 4)
	workspace.rho2 = workspace.gpu(cells * 4)
	workspace.mom2 = workspace.gpu(cells * 3 * 4)
	workspace.energy2 = workspace.gpu(cells * 4)
	workspace.k1Rho = workspace.gpu(cells * 4)
	workspace.k1Mom = workspace.gpu(cells * 3 * 4)
	workspace.k1Energy = workspace.gpu(cells * 4)
	workspace.gravity = workspace.gpu(cells * 4)
	workspace.hydro = workspace.gpu(cells * 24)
	workspace.hydroOut = workspace.gpu(cells * 24)
	workspace.hydroWork1 = workspace.gpu(cells * 24)
	workspace.hydroWork2 = workspace.gpu(cells * 24)
	workspace.hydroStatus = workspace.gpu(cells * 4)
	workspace.hydroDiagnostics = workspace.gpu(cells * 32)
	workspace.acceleration = workspace.gpu(cells * 12)
	workspace.poissonState = workspace.gpu(cells * 8)
	// Per-line Bluestein scratch is provisioned by the native Poisson host.
	workspace.waveLedger = workspace.gpu(modes * 6 * 4)
	workspace.reciprocalOldRe = workspace.gpu(modes * 4)
	workspace.reciprocalOldIm = workspace.gpu(modes * 4)
	workspace.reciprocalPotential = workspace.gpu(modes * 4)
	workspace.remapReport = workspace.gpu(48)
	workspace.gravityFluxBase = workspace.gpu(cells * 24)
	workspace.gravityPrior = workspace.gpu(cells * 4)
	workspace.gravityKickWork = workspace.gpu(cells * 4)
	workspace.previousPotential = make([]float32, int(modes)*spectralHeads)
	workspace.cellCounts = workspace.gpu(cells * 4)
	workspace.cellStarts = workspace.gpu((cells + 1) * 4)
	workspace.cellOffsets = workspace.gpu(cells * 4)
	workspace.psiStartRe = workspace.gpu(cells * 4)
	workspace.psiStartIm = workspace.gpu(cells * 4)
	workspace.psiRe = workspace.gpu(cells * 4)
	workspace.psiIm = workspace.gpu(cells * 4)
	workspace.dbgHead = workspace.gpu(4)
	workspace.dbgWords = workspace.gpu(dbgCapacity * dbgWordsPerEvent * 4)
	workspace.omegaLattice = workspace.gpu(modes * 4)
	workspace.gateWidth = workspace.gpu(modes * 4)
	workspace.accums = workspace.gpu(modes * 8 * 4)
	workspace.numCarriers = workspace.gpu(4)
	workspace.anchorIdx = workspace.gpu(modes * uint64(modeAnchors) * 4)
	workspace.anchorWeight = workspace.gpu(modes * uint64(modeAnchors) * 4)
	workspace.binStarts = workspace.gpu((modes + 1) * 4)
	workspace.binnedIdx = workspace.gpu(modes * 4)
	workspace.binParams = workspace.gpu(2 * 4)
	workspace.psiRealHeads = make([]*Buffer, spectralHeads)
	workspace.psiImagHeads = make([]*Buffer, spectralHeads)

	for head := 0; head < spectralHeads; head++ {
		workspace.psiRealHeads[head] = workspace.gpu(modes * 4)
		workspace.psiImagHeads[head] = workspace.gpu(modes * 4)
	}

	workspace.psiModeReal = workspace.gpu(modes * 4)
	workspace.psiModeImag = workspace.gpu(modes * 4)
	workspace.kineticReal = workspace.gpu(modes * 4)
	workspace.kineticImag = workspace.gpu(modes * 4)

	for _, buffer := range workspace.allBuffers() {
		if buffer != nil {
			buffer.Zero()
		}
	}

	fillInt32(workspace.anchorIdx.Int32Slice(), -1)
}

func (workspace *workspace) seedLattice() {
	modes := int(workspace.domain.MaxModes)
	omega := workspace.omegaLattice.Float32Slice()
	width := workspace.gateWidth.Float32Slice()
	starts := workspace.binStarts.Int32Slice()
	index := workspace.binnedIdx.Int32Slice()
	params := workspace.binParams.Float32Slice()
	domega := float32(workspace.domain.binWidth())

	for mode := 0; mode < modes; mode++ {
		omega[mode] = float32(workspace.domain.OmegaMin) + float32(mode)*domega
		width[mode] = domega
		index[mode] = int32(mode)
		starts[mode] = int32(mode)
	}

	starts[modes] = int32(modes)
	params[0] = float32(workspace.domain.OmegaMin)

	if domega != 0 {
		params[1] = 1 / domega
	}

	workspace.numCarriers.Int32Slice()[0] = int32(modes)
}

func (workspace *workspace) loadState(state *State) {
	if state == nil || state.N == 0 {
		workspace.allocateParticles(0)
		workspace.particles = 0
		workspace.contentIDs = nil
		workspace.waveProjectionReady = false
		return
	}

	if workspace.particles != state.N || !slices.Equal(workspace.contentIDs, state.ContentIDs) {
		workspace.waveProjectionReady = false
	}

	workspace.contentIDs = append(workspace.contentIDs[:0], state.ContentIDs...)
	workspace.allocateParticles(state.N)
	workspace.particles = state.N
	copy(workspace.pos.Float32Slice(), state.Pos)
	copy(workspace.vel.Float32Slice(), state.Vel)
	state.ensureCoherencePosition()
	copy(workspace.coherencePosition.Float32Slice(), state.CoherencePosition)
	workspace.pilotPrevious.Zero()
	copy(workspace.pilotPrevious.Float32Slice(), state.PilotVel)
	workspace.phasePrior.Zero()
	copy(workspace.phasePrior.Float32Slice(), state.PhasePotential)
	copy(workspace.mass.Float32Slice(), state.Mass)
	copy(workspace.heat.Float32Slice(), state.Heat)
	state.ensureMaterialEnergy()
	copy(workspace.materialEnergy.Float32Slice(), state.MaterialEnergy)
	copy(workspace.oscEnergy.Float32Slice(), state.Energy)
	copy(workspace.phase.Float32Slice(), state.Phase)
	copy(workspace.omega.Float32Slice(), state.Omega)
	amp := workspace.amp.Float32Slice()

	for index := 0; index < state.N; index++ {
		energy := state.Energy[index]

		amp[index] = float32(math.Sqrt(float64(energy)))
		state.Amp[index] = amp[index]
	}
}

func (workspace *workspace) storeState(state *State) {
	if state == nil || workspace.particles == 0 {
		return
	}

	if state.N != workspace.particles {
		resized := newState(workspace.particles)
		copy(resized.Bytes, state.Bytes)
		copy(resized.Seqs, state.Seqs)
		copy(resized.TokenIDs, state.TokenIDs)
		copy(resized.ContentIDs, state.ContentIDs)
		copy(resized.Clamped, state.Clamped)
		copy(resized.Dark, state.Dark)
		*state = *resized
	}

	workspace.engine.Synchronize()
	copy(state.Pos, workspace.pos.Float32Slice())
	copy(state.Vel, workspace.vel.Float32Slice())
	state.ensureCoherencePosition()
	copy(state.CoherencePosition, workspace.coherencePosition.Float32Slice())
	if len(state.PilotVel) != state.N*3 {
		state.PilotVel = make([]float32, state.N*3)
	}
	copy(state.PilotVel, workspace.pilotPrevious.Float32Slice())
	if len(state.PhasePotential) != state.N {
		state.PhasePotential = make([]float32, state.N)
	}
	copy(state.PhasePotential, workspace.phasePrior.Float32Slice())
	copy(state.Mass, workspace.mass.Float32Slice())
	copy(state.Heat, workspace.heat.Float32Slice())
	if len(state.MaterialEnergy) != state.N {
		state.MaterialEnergy = make([]float32, state.N)
	}
	copy(state.MaterialEnergy, workspace.materialEnergy.Float32Slice())
	copy(state.Energy, workspace.oscEnergy.Float32Slice())
	copy(state.Phase, workspace.phase.Float32Slice())
	copy(state.Omega, workspace.omega.Float32Slice())
	copy(state.Amp, workspace.amp.Float32Slice())
}

func (workspace *workspace) packFields(momRho, energy, waveReal, waveImag []float32) fieldScale {
	workspace.engine.Synchronize()
	cells := workspace.domain.CellCount()
	rho := workspace.rho.Float32Slice()
	mom := workspace.mom.Float32Slice()
	internal := workspace.energy.Float32Slice()
	psiRe := workspace.psiRe.Float32Slice()
	psiIm := workspace.psiIm.Float32Slice()
	var scale fieldScale

	for cell := range cells {
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

func (workspace *workspace) allocateParticles(count int) {
	if count <= workspace.particleCapacity && workspace.pos != nil {
		workspace.particles = count
		return
	}

	workspace.closeParticles()

	if count == 0 {
		workspace.particles = 0
		workspace.particleCapacity = 0
		return
	}

	capacity := count

	if capacity < 1024 {
		capacity = 1024
	}

	workspace.particles = count
	workspace.particleCapacity = capacity

	particleCount := uint64(capacity)
	workspace.coherencePosition = workspace.gpu(particleCount * 12)
	workspace.reciprocalForce = workspace.gpu(particleCount * 12)
	workspace.reciprocalAmplitude = workspace.gpu(particleCount * 4)
	workspace.reciprocalStatus = workspace.gpu(uint64(max(capacity, workspace.domain.MaxModes)) * 4)
	workspace.pos = workspace.gpu(particleCount * 3 * 4)
	workspace.vel = workspace.gpu(particleCount * 3 * 4)
	workspace.mass = workspace.gpu(particleCount * 4)
	workspace.heat = workspace.gpu(particleCount * 4)
	workspace.materialEnergy = workspace.gpu(particleCount * 4)
	workspace.materialEnergyOut = workspace.gpu(particleCount * 4)
	workspace.oscEnergy = workspace.gpu(particleCount * 4)
	workspace.phase = workspace.gpu(particleCount * 4)
	workspace.omega = workspace.gpu(particleCount * 4)
	workspace.amp = workspace.gpu(particleCount * 4)
	workspace.posOut = workspace.gpu(particleCount * 3 * 4)
	workspace.velOut = workspace.gpu(particleCount * 3 * 4)
	workspace.heatOut = workspace.gpu(particleCount * 4)
	workspace.cellIdx = workspace.gpu(particleCount * 4)
	workspace.originalIdx = workspace.gpu(particleCount * 4)
	workspace.sortedPos = workspace.gpu(particleCount * 3 * 4)
	workspace.sortedVel = workspace.gpu(particleCount * 3 * 4)
	workspace.sortedMass = workspace.gpu(particleCount * 4)
	workspace.sortedHeat = workspace.gpu(particleCount * 4)
	workspace.sortedEnergy = workspace.gpu(particleCount * 4)
	workspace.headPhase = workspace.gpu(particleCount * 4)
	workspace.headHeat = workspace.gpu(particleCount * 4)
	workspace.couplingAmp = workspace.gpu(particleCount * 4)
	workspace.pilotPrevious = workspace.gpu(particleCount * 3 * 4)
	workspace.pilotReport = workspace.gpu(particleCount * 4 * 4)
	workspace.contactReport = workspace.gpu(particleCount * 7 * 4)
	workspace.phasePrior = workspace.gpu(particleCount * 4)
	workspace.phaseLedger = workspace.gpu(particleCount * 6 * 4)
	workspace.particleStatus = workspace.gpu(particleCount * 4)
}

func (workspace *workspace) closeParticles() {
	workspace.particleCapacity = 0

	for _, buffer := range []*Buffer{
		workspace.coherencePosition, workspace.reciprocalForce, workspace.reciprocalAmplitude, workspace.reciprocalStatus,
		workspace.headPhase, workspace.headHeat, workspace.couplingAmp, workspace.pilotPrevious, workspace.pilotReport, workspace.contactReport, workspace.particleStatus, workspace.phasePrior, workspace.phaseLedger,
		workspace.pos, workspace.vel, workspace.mass, workspace.heat, workspace.oscEnergy, workspace.materialEnergy, workspace.materialEnergyOut,
		workspace.phase, workspace.omega, workspace.amp,
		workspace.posOut, workspace.velOut, workspace.heatOut,
		workspace.cellIdx, workspace.originalIdx,
		workspace.sortedPos, workspace.sortedVel,
		workspace.sortedMass, workspace.sortedHeat, workspace.sortedEnergy,
	} {
		if buffer != nil {
			buffer.Close()
		}
	}

	workspace.coherencePosition = nil
	workspace.reciprocalForce = nil
	workspace.reciprocalAmplitude = nil
	workspace.reciprocalStatus = nil
	workspace.headPhase = nil
	workspace.headHeat = nil
	workspace.couplingAmp = nil
	workspace.pilotPrevious = nil
	workspace.pilotReport = nil
	workspace.contactReport = nil
	workspace.phasePrior = nil
	workspace.phaseLedger = nil
	workspace.particleStatus = nil
	workspace.pos = nil
	workspace.vel = nil
	workspace.mass = nil
	workspace.heat = nil
	workspace.materialEnergy = nil
	workspace.materialEnergyOut = nil
	workspace.oscEnergy = nil
	workspace.phase = nil
	workspace.omega = nil
	workspace.amp = nil
	workspace.posOut = nil
	workspace.velOut = nil
	workspace.heatOut = nil
	workspace.cellIdx = nil
	workspace.originalIdx = nil
	workspace.sortedPos = nil
	workspace.sortedVel = nil
	workspace.sortedMass = nil
	workspace.sortedHeat = nil
	workspace.sortedEnergy = nil
}

func (workspace *workspace) allBuffers() []*Buffer {
	buffers := []*Buffer{
		workspace.spectralPotential, workspace.spectralMetric,
		workspace.reciprocalOldRe, workspace.reciprocalOldIm, workspace.reciprocalPotential,
		workspace.rho, workspace.mom, workspace.energy,
		workspace.rho1, workspace.mom1, workspace.energy1,
		workspace.rho2, workspace.mom2, workspace.energy2,
		workspace.k1Rho, workspace.k1Mom, workspace.k1Energy,
		workspace.gravity, workspace.hydro, workspace.hydroOut, workspace.hydroWork1, workspace.hydroWork2, workspace.hydroStatus, workspace.hydroDiagnostics, workspace.acceleration, workspace.poissonState, workspace.poissonScratch, workspace.waveLedger, workspace.remapReport, workspace.gravityFluxBase, workspace.gravityPrior, workspace.gravityKickWork,
		workspace.cellCounts, workspace.cellStarts, workspace.cellOffsets,
		workspace.psiRe, workspace.psiIm, workspace.psiStartRe, workspace.psiStartIm,
		workspace.dbgHead, workspace.dbgWords,
		workspace.omegaLattice, workspace.gateWidth,
		workspace.accums, workspace.numCarriers,
		workspace.anchorIdx, workspace.anchorWeight,
		workspace.psiModeReal, workspace.psiModeImag,
		workspace.kineticReal, workspace.kineticImag,
		workspace.binStarts, workspace.binnedIdx, workspace.binParams,
		workspace.coherencePosition, workspace.reciprocalForce, workspace.reciprocalAmplitude, workspace.reciprocalStatus,
		workspace.headPhase, workspace.headHeat, workspace.couplingAmp, workspace.pilotPrevious, workspace.pilotReport, workspace.contactReport, workspace.particleStatus, workspace.phasePrior, workspace.phaseLedger,
		workspace.pos, workspace.vel, workspace.mass, workspace.heat, workspace.oscEnergy, workspace.materialEnergy, workspace.materialEnergyOut,
		workspace.phase, workspace.omega, workspace.amp,
		workspace.posOut, workspace.velOut, workspace.heatOut,
		workspace.cellIdx, workspace.originalIdx,
		workspace.sortedPos, workspace.sortedVel,
		workspace.sortedMass, workspace.sortedHeat, workspace.sortedEnergy,
	}
	buffers = append(buffers, workspace.psiRealHeads...)
	buffers = append(buffers, workspace.psiImagHeads...)
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

/* spectralPeaks scans synchronized modes without copying the lattice arrays. */
func (workspace *workspace) spectralPeaks() []SpectralPeak {
	omega := workspace.omegaLattice.Float32Slice()[:workspace.domain.MaxModes]
	real := workspace.psiModeReal.Float32Slice()
	imag := workspace.psiModeImag.Float32Slice()
	var peaks []SpectralPeak

	for index := 1; index+1 < len(omega); index++ {
		power := real[index]*real[index] + imag[index]*imag[index]
		left := real[index-1]*real[index-1] + imag[index-1]*imag[index-1]
		right := real[index+1]*real[index+1] + imag[index+1]*imag[index+1]

		if power > left && power >= right {
			peaks = append(peaks, SpectralPeak{Index: index, Frequency: omega[index], Power: power})
		}
	}

	return peaks
}
