package sensorium

/*
#cgo CXXFLAGS: -std=c++20 -x objective-c++ -fobjc-arc
#cgo LDFLAGS: -framework Metal -framework Foundation -lc++
#include "bridge.h"
#include <stdlib.h>
#include <string.h>
*/
import "C"
import (
	"fmt"
	"runtime"
	"unsafe"
)

type Engine struct {
	ctx      *C.ManifoldContext
	GridSize [3]int
	Spacing  float32
}

func NewEngine(metallibPath string, gx, gy, gz int, spacing float32) (*Engine, error) {
	cPath := C.CString(metallibPath)
	defer C.free(unsafe.Pointer(cPath))

	ctx := C.manifold_create_context(cPath)
	if ctx == nil {
		return nil, fmt.Errorf("manifold: failed to initialize Metal context with metallib: %s", metallibPath)
	}

	e := &Engine{
		ctx:      ctx,
		GridSize: [3]int{gx, gy, gz},
		Spacing:  spacing,
	}

	runtime.SetFinalizer(e, func(obj *Engine) {
		obj.Close()
	})

	return e, nil
}

func (e *Engine) Close() {
	if e == nil || e.ctx == nil {
		return
	}

	runtime.SetFinalizer(e, nil)
	C.manifold_destroy_context(e.ctx)
	e.ctx = nil
}

func (e *Engine) Synchronize() {
	C.manifold_synchronize(e.ctx)
}

// ----------------------------------------------------------------------------
// Unified Memory Buffer Wrappers (Zero-Copy)
// ----------------------------------------------------------------------------
type Buffer struct {
	cBuf *C.ManifoldBuffer
	size uint64
}

func (e *Engine) NewBuffer(bytes uint64, initialData unsafe.Pointer) *Buffer {
	cBuf := C.manifold_create_buffer(e.ctx, C.uint64_t(bytes), initialData)
	b := &Buffer{cBuf: cBuf, size: bytes}
	runtime.SetFinalizer(b, func(obj *Buffer) {
		obj.Close()
	})
	return b
}

func (b *Buffer) Close() {
	if b == nil || b.cBuf == nil {
		return
	}

	runtime.SetFinalizer(b, nil)
	C.manifold_destroy_buffer(b.cBuf)
	b.cBuf = nil
	b.size = 0
}

func (b *Buffer) Adopt() {
	if b == nil {
		return
	}

	runtime.SetFinalizer(b, nil)
}

func (b *Buffer) Zero() {
	if b == nil || b.cBuf == nil {
		return
	}

	pointer := C.manifold_get_buffer_pointer(b.cBuf)

	if pointer == nil {
		return
	}

	C.memset(pointer, 0, C.size_t(b.size))
}

func (b *Buffer) Float32Slice() []float32 {
	ptr := C.manifold_get_buffer_pointer(b.cBuf)
	if ptr == nil {
		return nil
	}
	return unsafe.Slice((*float32)(ptr), b.size/4)
}

func (b *Buffer) Int32Slice() []int32 {
	ptr := C.manifold_get_buffer_pointer(b.cBuf)
	if ptr == nil {
		return nil
	}
	return unsafe.Slice((*int32)(ptr), b.size/4)
}

func (b *Buffer) UInt32Slice() []uint32 {
	ptr := C.manifold_get_buffer_pointer(b.cBuf)
	if ptr == nil {
		return nil
	}
	return unsafe.Slice((*uint32)(ptr), b.size/4)
}

// ----------------------------------------------------------------------------
// 1. Diagnostics & Field Operations
// ----------------------------------------------------------------------------
func (e *Engine) ClearField(field *Buffer) {
	C.manifold_clear_field(e.ctx, field.cBuf)
}

func (e *Engine) ReduceEnergyStats(x, outStats *Buffer) {
	C.manifold_thermo_reduce_energy_stats(e.ctx, x.cBuf, outStats.cBuf)
}

// ----------------------------------------------------------------------------
// 2. PIC & Sort-Based Scatter
// ----------------------------------------------------------------------------
func (e *Engine) ScatterSorted(
	pos, vel, mass, heat, energy *Buffer,
	rhoField, momField, eField *Buffer,
	numParticles int,
) {
	C.manifold_scatter_sorted(
		e.ctx,
		pos.cBuf, vel.cBuf, mass.cBuf, heat.cBuf, energy.cBuf,
		rhoField.cBuf, momField.cBuf, eField.cBuf,
		C.int64_t(e.GridSize[0]), C.int64_t(e.GridSize[1]), C.int64_t(e.GridSize[2]),
		C.float(e.Spacing),
	)
}

func (e *Engine) PICGatherUpdate(
	posIn, mass, posOut, velOut, heatOut *Buffer,
	rhoField, momField, eField, gravityPot *Buffer,
	dbgHead, dbgWords *Buffer, dbgCapacity int,
	dt, gamma, rSpecific, cv, rhoMin, pMin, gravityEnabled float32,
) {
	domainX := float32(e.GridSize[0]) * e.Spacing
	domainY := float32(e.GridSize[1]) * e.Spacing
	domainZ := float32(e.GridSize[2]) * e.Spacing

	C.manifold_pic_gather_update_particles(
		e.ctx,
		posIn.cBuf, mass.cBuf, posOut.cBuf, velOut.cBuf, heatOut.cBuf,
		rhoField.cBuf, momField.cBuf, eField.cBuf, gravityPot.cBuf,
		dbgHead.cBuf, dbgWords.cBuf, C.int64_t(dbgCapacity),
		C.int64_t(e.GridSize[0]), C.int64_t(e.GridSize[1]), C.int64_t(e.GridSize[2]),
		C.float(e.Spacing), C.float(dt),
		C.float(domainX), C.float(domainY), C.float(domainZ),
		C.float(gamma), C.float(rSpecific), C.float(cv),
		C.float(rhoMin), C.float(pMin), C.float(gravityEnabled),
	)
}

// ----------------------------------------------------------------------------
// 3. Quantum Flow (Pilot-Wave)
// ----------------------------------------------------------------------------
func (e *Engine) ProjectModesToSpatial(
	modePsiReal, modePsiImag, modeAnchorIdx, modeAnchorWeight, particlePos *Buffer,
	psiReField, psiImField *Buffer,
	anchorsPerMode int,
) {
	C.manifold_project_modes_to_spatial_psi(
		e.ctx,
		modePsiReal.cBuf, modePsiImag.cBuf,
		modeAnchorIdx.cBuf, modeAnchorWeight.cBuf,
		particlePos.cBuf,
		psiReField.cBuf, psiImField.cBuf,
		C.int64_t(anchorsPerMode),
		C.int64_t(e.GridSize[0]), C.int64_t(e.GridSize[1]), C.int64_t(e.GridSize[2]),
		C.float(e.Spacing),
	)
}

func (e *Engine) PilotWaveGather(
	posIn, mass, posOut, velOut, psiRe, psiIm *Buffer,
	numParticles int, dt float32,
	hbarEff, epsDenom, massMin float32,
) {
	domainX := float32(e.GridSize[0]) * e.Spacing
	domainY := float32(e.GridSize[1]) * e.Spacing
	domainZ := float32(e.GridSize[2]) * e.Spacing

	C.manifold_pic_gather_pilot_wave(
		e.ctx,
		posIn.cBuf, mass.cBuf, posOut.cBuf, velOut.cBuf,
		psiRe.cBuf, psiIm.cBuf,
		C.int64_t(numParticles),
		C.int64_t(e.GridSize[0]), C.int64_t(e.GridSize[1]), C.int64_t(e.GridSize[2]),
		C.float(e.Spacing), C.float(dt),
		C.float(domainX), C.float(domainY), C.float(domainZ),
		C.float(hbarEff), C.float(epsDenom), C.float(massMin),
	)
}

// ----------------------------------------------------------------------------
// 4. Gas Dynamics (Eulerian RK2)
// ----------------------------------------------------------------------------
func (e *Engine) GasRK2Stage1(
	rho0, mom0, e0, rho1, mom1, e1, k1Rho, k1Mom, k1E *Buffer,
	dbgHead, dbgWords *Buffer, dbgCapacity int,
	dt, gamma, cv, rhoMin, pMin, mu, kThermal float32,
) {
	C.manifold_gas_rk2_stage1(
		e.ctx,
		rho0.cBuf, mom0.cBuf, e0.cBuf,
		rho1.cBuf, mom1.cBuf, e1.cBuf,
		k1Rho.cBuf, k1Mom.cBuf, k1E.cBuf,
		dbgHead.cBuf, dbgWords.cBuf, C.int64_t(dbgCapacity),
		C.int64_t(e.GridSize[0]), C.int64_t(e.GridSize[1]), C.int64_t(e.GridSize[2]),
		C.float(e.Spacing), C.float(dt), C.float(gamma), C.float(cv),
		C.float(rhoMin), C.float(pMin), C.float(mu), C.float(kThermal),
	)
}

func (e *Engine) GasRK2Stage2(
	rho0, mom0, e0, rho1, mom1, e1, k1Rho, k1Mom, k1E *Buffer,
	rhoOut, momOut, eOut *Buffer,
	dbgHead, dbgWords *Buffer, dbgCapacity int,
	dt, gamma, cv, rhoMin, pMin, mu, kThermal float32,
) {
	C.manifold_gas_rk2_stage2(
		e.ctx,
		rho0.cBuf, mom0.cBuf, e0.cBuf,
		rho1.cBuf, mom1.cBuf, e1.cBuf,
		k1Rho.cBuf, k1Mom.cBuf, k1E.cBuf,
		rhoOut.cBuf, momOut.cBuf, eOut.cBuf,
		dbgHead.cBuf, dbgWords.cBuf, C.int64_t(dbgCapacity),
		C.int64_t(e.GridSize[0]), C.int64_t(e.GridSize[1]), C.int64_t(e.GridSize[2]),
		C.float(e.Spacing), C.float(dt), C.float(gamma), C.float(cv),
		C.float(rhoMin), C.float(pMin), C.float(mu), C.float(kThermal),
	)
}

// ----------------------------------------------------------------------------
// 5. Coherence Lattice & GPE Step
// ----------------------------------------------------------------------------
func (e *Engine) CoherenceGPEStep(
	oscPhase, oscOmega, oscAmp *Buffer,
	carrierReal, carrierImag, carrierOmega, carrierGateWidth *Buffer,
	carrierAnchorIdx, carrierAnchorWeight, accums, numCarriersSnapshot, particlePos *Buffer,
	extraPotential *Buffer,
	numOsc, maxCarriers int,
	dt, hbarEff, massEff, gInteraction, energyDecay, chemPot, invDomega2 float32,
	rngSeed uint32, anchorEps, metricCoupling float32,
	metabolicRate, gateWidthMin, gateWidthMax, offenderWeightFloor, spatialSigma float32,
) {
	domainX := float32(e.GridSize[0]) * e.Spacing
	domainY := float32(e.GridSize[1]) * e.Spacing
	domainZ := float32(e.GridSize[2]) * e.Spacing

	prm := C.SpectralModeParams{
		num_osc:               C.uint32_t(numOsc),
		max_carriers:          C.uint32_t(maxCarriers),
		num_carriers:          C.uint32_t(maxCarriers),
		dt:                    C.float(dt),
		gate_width_min:        C.float(gateWidthMin),
		gate_width_max:        C.float(gateWidthMax),
		offender_weight_floor: C.float(offenderWeightFloor),
		domain_x:              C.float(domainX),
		domain_y:              C.float(domainY),
		domain_z:              C.float(domainZ),
		spatial_sigma:         C.float(spatialSigma),
		metabolic_rate:        C.float(metabolicRate),
	}

	gp := C.GPEParams{
		dt:                 C.float(dt),
		hbar_eff:           C.float(hbarEff),
		mass_eff:           C.float(massEff),
		g_interaction:      C.float(gInteraction),
		energy_decay:       C.float(energyDecay),
		chemical_potential: C.float(chemPot),
		inv_domega2:        C.float(invDomega2),
		anchors:            8,
		rng_seed:           C.uint32_t(rngSeed),
		anchor_eps:         C.float(anchorEps),
		metric_coupling:    C.float(metricCoupling),
	}

	var extra *C.ManifoldBuffer
	if extraPotential != nil {
		extra = extraPotential.cBuf
	}

	C.manifold_coherence_gpe_step(
		e.ctx,
		oscPhase.cBuf, oscOmega.cBuf, oscAmp.cBuf,
		carrierReal.cBuf, carrierImag.cBuf, carrierOmega.cBuf, carrierGateWidth.cBuf,
		carrierAnchorIdx.cBuf, carrierAnchorWeight.cBuf,
		accums.cBuf, numCarriersSnapshot.cBuf, particlePos.cBuf,
		prm, gp, extra,
	)
}

func (e *Engine) ScatterComputeCellIdx(pos, cellIdx *Buffer) {
	C.manifold_scatter_compute_cell_idx(
		e.ctx,
		pos.cBuf,
		cellIdx.cBuf,
		C.int64_t(e.GridSize[0]),
		C.int64_t(e.GridSize[1]),
		C.int64_t(e.GridSize[2]),
		C.float(e.Spacing),
	)
}

func (e *Engine) ScatterCountCells(cellIdx, cellCounts *Buffer) {
	C.manifold_scatter_count_cells(
		e.ctx,
		cellIdx.cBuf,
		cellCounts.cBuf,
		C.int64_t(e.GridSize[0]),
		C.int64_t(e.GridSize[1]),
		C.int64_t(e.GridSize[2]),
		C.float(e.Spacing),
	)
}

func (e *Engine) ScatterReorderParticles(
	posIn, velIn, massIn, heatIn, energyIn *Buffer,
	cellIdx, cellStarts, cellOffsets *Buffer,
	posOut, velOut, massOut, heatOut, energyOut, originalIdx *Buffer,
) {
	C.manifold_scatter_reorder_particles(
		e.ctx,
		posIn.cBuf, velIn.cBuf, massIn.cBuf, heatIn.cBuf, energyIn.cBuf,
		cellIdx.cBuf, cellStarts.cBuf, cellOffsets.cBuf,
		posOut.cBuf, velOut.cBuf, massOut.cBuf, heatOut.cBuf, energyOut.cBuf,
		originalIdx.cBuf,
		C.int64_t(e.GridSize[0]),
		C.int64_t(e.GridSize[1]),
		C.int64_t(e.GridSize[2]),
		C.float(e.Spacing),
	)
}

func (e *Engine) CoherenceAccumulateForces(
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
	domainX := float32(e.GridSize[0]) * e.Spacing
	domainY := float32(e.GridSize[1]) * e.Spacing
	domainZ := float32(e.GridSize[2]) * e.Spacing

	C.manifold_coherence_accumulate_forces(
		e.ctx,
		oscPhase.cBuf, oscOmega.cBuf, oscAmp.cBuf, particlePos.cBuf,
		carrierOmega.cBuf, carrierGateWidth.cBuf,
		carrierAnchorIdx.cBuf, carrierAnchorWeight.cBuf,
		accums.cBuf, binStarts.cBuf, carrierBinnedIdx.cBuf, binParams.cBuf,
		C.int64_t(numBins),
		particleHeat.cBuf,
		C.int64_t(numOsc),
		numCarriersSnapshot.cBuf,
		C.int64_t(maxCarriers),
		C.float(dt),
		C.float(metabolicRate),
		C.float(gateWidthMin),
		C.float(gateWidthMax),
		C.float(offenderWeightFloor),
		C.float(domainX), C.float(domainY), C.float(domainZ),
		C.float(spatialSigma),
	)
}

func (e *Engine) CoherenceUpdateOscillatorPhases(
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
	domainX := float32(e.GridSize[0]) * e.Spacing
	domainY := float32(e.GridSize[1]) * e.Spacing
	domainZ := float32(e.GridSize[2]) * e.Spacing

	prm := C.SpectralModeParams{
		num_osc:               C.uint32_t(numOsc),
		max_carriers:          C.uint32_t(maxCarriers),
		num_carriers:          C.uint32_t(maxCarriers),
		dt:                    C.float(dt),
		coupling_scale:        C.float(couplingScale),
		gate_width_min:        C.float(gateWidthMin),
		gate_width_max:        C.float(gateWidthMax),
		offender_weight_floor: C.float(offenderWeightFloor),
		domain_x:              C.float(domainX),
		domain_y:              C.float(domainY),
		domain_z:              C.float(domainZ),
		spatial_sigma:         C.float(spatialSigma),
		metabolic_rate:        C.float(metabolicRate),
	}

	C.manifold_coherence_update_oscillator_phases(
		e.ctx,
		oscPhase.cBuf, oscOmega.cBuf, oscAmp.cBuf,
		carrierReal.cBuf, carrierImag.cBuf, carrierOmega.cBuf, carrierGateWidth.cBuf,
		carrierAnchorIdx.cBuf, carrierAnchorWeight.cBuf,
		numCarriersSnapshot.cBuf,
		prm,
		binStarts.cBuf, carrierBinnedIdx.cBuf, binParams.cBuf,
		C.int64_t(numBins),
		particlePos.cBuf,
	)
}
