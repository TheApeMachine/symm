//go:build !darwin || !cgo

package sensorium

import (
	"unsafe"

	"github.com/theapemachine/errnie"
)

/* Engine cannot be initialized where the Metal CGo backend is unavailable. */
type Engine struct {
	GridSize [3]int
	Spacing  float32
}

/* Buffer has no device storage on unsupported platforms. */
type Buffer struct{}

func NewEngine(metallibPath string, gx, gy, gz int, spacing float32) (*Engine, error) {
	return nil, errnie.Error(errnie.Err(errnie.Internal, "sensorium: Metal engine requires Darwin with CGo", nil))
}

func (engine *Engine) Close() {}

func (engine *Engine) Synchronize() {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: Synchronize requires an initialized Darwin Metal engine", nil))
}

func (engine *Engine) NewBuffer(bytes uint64, initialData unsafe.Pointer) *Buffer {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: NewBuffer requires an initialized Darwin Metal engine", nil))
	return nil
}

func (buffer *Buffer) Close() {}

func (buffer *Buffer) Adopt() {}

func (buffer *Buffer) Zero() {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: Zero requires an initialized Darwin Metal engine", nil))
}

func (buffer *Buffer) Float32Slice() []float32 {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: Float32Slice requires an initialized Darwin Metal engine", nil))
	return nil
}

func (buffer *Buffer) Int32Slice() []int32 {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: Int32Slice requires an initialized Darwin Metal engine", nil))
	return nil
}

func (buffer *Buffer) UInt32Slice() []uint32 {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: UInt32Slice requires an initialized Darwin Metal engine", nil))
	return nil
}

func (engine *Engine) ClearField(field *Buffer) {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: ClearField requires an initialized Darwin Metal engine", nil))
}

func (engine *Engine) ReduceEnergyStats(values, outStats *Buffer) {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: ReduceEnergyStats requires an initialized Darwin Metal engine", nil))
}

func (engine *Engine) ScatterSorted(
	pos, vel, mass, heat, energy *Buffer,
	rhoField, momField, eField *Buffer,
	numParticles int,
) {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: ScatterSorted requires an initialized Darwin Metal engine", nil))
}

func (engine *Engine) PICGatherUpdate(
	posIn, mass, posOut, velOut, heatOut *Buffer,
	rhoField, momField, eField, gravityPot *Buffer,
	dbgHead, dbgWords *Buffer, dbgCapacity int,
	dt, gamma, rSpecific, cv, rhoMin, pMin, gravityEnabled float32,
) {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: PICGatherUpdate requires an initialized Darwin Metal engine", nil))
}

func (engine *Engine) ProjectModesToSpatial(
	modePsiReal, modePsiImag, modeAnchorIdx, modeAnchorWeight, particlePos *Buffer,
	psiReField, psiImField *Buffer,
	anchorsPerMode int,
) {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: ProjectModesToSpatial requires an initialized Darwin Metal engine", nil))
}

func (engine *Engine) PilotWaveGather(
	posIn, mass, posOut, velOut, psiRe, psiIm *Buffer,
	numParticles int, dt float32,
	hbarEff, epsDenom, massMin float32,
) {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: PilotWaveGather requires an initialized Darwin Metal engine", nil))
}

func (engine *Engine) GasRK2Stage1(
	rho0, mom0, e0, rho1, mom1, e1, k1Rho, k1Mom, k1E *Buffer,
	dbgHead, dbgWords *Buffer, dbgCapacity int,
	dt, gamma, cv, rhoMin, pMin, mu, kThermal float32,
) {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: GasRK2Stage1 requires an initialized Darwin Metal engine", nil))
}

func (engine *Engine) GasRK2Stage2(
	rho0, mom0, e0, rho1, mom1, e1, k1Rho, k1Mom, k1E *Buffer,
	rhoOut, momOut, eOut *Buffer,
	dbgHead, dbgWords *Buffer, dbgCapacity int,
	dt, gamma, cv, rhoMin, pMin, mu, kThermal float32,
) {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: GasRK2Stage2 requires an initialized Darwin Metal engine", nil))
}

func (engine *Engine) CoherenceGPEStep(
	oscPhase, oscOmega, oscAmp *Buffer,
	carrierReal, carrierImag, carrierOmega, carrierGateWidth *Buffer,
	kineticReal, kineticImag *Buffer,
	carrierAnchorIdx, carrierAnchorWeight, accums, numCarriersSnapshot, particlePos *Buffer,
	extraPotential *Buffer,
	numOsc, maxCarriers int,
	dt, hbarEff, massEff, gInteraction, energyDecay, chemPot, invDomega2 float32,
	rngSeed uint32, anchorEps, metricCoupling float32,
	metabolicRate, gateWidthMin, gateWidthMax, offenderWeightFloor, spatialSigma float32,
) {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: CoherenceGPEStep requires an initialized Darwin Metal engine", nil))
}

func (engine *Engine) ScatterComputeCellIdx(pos, cellIdx *Buffer) {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: ScatterComputeCellIdx requires an initialized Darwin Metal engine", nil))
}

func (engine *Engine) ScatterCountCells(cellIdx, cellCounts *Buffer) {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: ScatterCountCells requires an initialized Darwin Metal engine", nil))
}

func (engine *Engine) ScatterReorderParticles(
	posIn, velIn, massIn, heatIn, energyIn *Buffer,
	cellIdx, cellStarts, cellOffsets *Buffer,
	posOut, velOut, massOut, heatOut, energyOut, originalIdx *Buffer,
) {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: ScatterReorderParticles requires an initialized Darwin Metal engine", nil))
}

func (engine *Engine) CoherenceAccumulateForces(
	oscPhase, oscOmega, oscAmp, particlePos *Buffer,
	carrierOmega, carrierGateWidth, carrierAnchorIdx, carrierAnchorWeight *Buffer,
	accums, binStarts, carrierBinnedIdx, binParams *Buffer,
	numBins int,
	particleHeat *Buffer,
	numOsc int,
	numCarriersSnapshot *Buffer,
	maxCarriers int,
	dt, metabolicRate, gateWidthMin, gateWidthMax, offenderWeightFloor float32,
	spatialSigma float32,
) {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: CoherenceAccumulateForces requires an initialized Darwin Metal engine", nil))
}

func (engine *Engine) CoherenceUpdateOscillatorPhases(
	oscPhase, oscOmega, oscAmp *Buffer,
	carrierReal, carrierImag, carrierOmega, carrierGateWidth *Buffer,
	carrierAnchorIdx, carrierAnchorWeight, numCarriersSnapshot *Buffer,
	numOsc, maxCarriers int,
	dt, couplingScale, gateWidthMin, gateWidthMax float32,
	binStarts, carrierBinnedIdx, binParams *Buffer,
	numBins int,
	particlePos *Buffer,
	spatialSigma, metabolicRate, offenderWeightFloor float32,
) {
	errnie.Error(errnie.Err(errnie.Internal, "sensorium: CoherenceUpdateOscillatorPhases requires an initialized Darwin Metal engine", nil))
}
