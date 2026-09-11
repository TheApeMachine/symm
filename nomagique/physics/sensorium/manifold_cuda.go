//go:build linux && cgo && cuda

package sensorium

/*
#cgo LDFLAGS: -L${SRCDIR}/cuda/build -lsensorium_cuda -Wl,-rpath,${SRCDIR}/cuda/build
#include "bridge.h"
#include <stdlib.h>
#include <string.h>
*/
import "C"
import (
	"fmt"
	"math"
	"runtime"
	"unsafe"
)

type Engine struct {
	ctx      *C.ManifoldContext
	GridSize [3]int
	Spacing  float32
}

// NewEngine retains the shared platform API. CUDA kernels are compiled into
// libsensorium_cuda; the Metal-library path is unused on this backend.
func NewEngine(_ string, gx, gy, gz int, spacing float32) (*Engine, error) {
	return NewCUDAEngine(0, gx, gy, gz, spacing)
}

// NewCUDAEngine owns one selected device and one stream, not a sharded domain.
func NewCUDAEngine(device, gx, gy, gz int, spacing float32) (*Engine, error) {
	if gx <= 0 || gy <= 0 || gz <= 0 || !(spacing > 0) || math.IsInf(float64(spacing), 0) {
		return nil, fmt.Errorf("sensorium: positive grid dimensions and finite spacing required")
	}
	if uint64(gx) > uint64(math.MaxUint32)/3/uint64(gy)/uint64(gz) {
		return nil, fmt.Errorf("sensorium: grid exceeds kernel uint32 indexing")
	}
	if device < 0 || int64(device) > math.MaxInt32 {
		return nil, fmt.Errorf("sensorium: invalid CUDA device ordinal %d", device)
	}
	var message [512]C.char
	ctx := C.manifold_create_cuda_context(C.int(device), &message[0], C.uint64_t(len(message)))
	if ctx == nil {
		return nil, fmt.Errorf("sensorium: CUDA initialization: %s", C.GoString(&message[0]))
	}
	e := &Engine{ctx: ctx, GridSize: [3]int{gx, gy, gz}, Spacing: spacing}
	runtime.SetFinalizer(e, (*Engine).Close)
	return e, nil
}

// Err reports runtime/launch failures, not the gas solver's recoverable debug tags.
func (e *Engine) Err() error {
	if e == nil || e.ctx == nil {
		return fmt.Errorf("sensorium: CUDA engine is closed")
	}
	message := C.GoString(C.manifold_last_error(e.ctx))
	runtime.KeepAlive(e)
	if message != "" {
		return fmt.Errorf("sensorium: CUDA: %s", message)
	}
	return nil
}

func (e *Engine) check() {
	if err := e.Err(); err != nil {
		panic(err)
	}
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
	if e == nil || e.ctx == nil {
		panic("sensorium: synchronize on closed CUDA engine")
	}
	if !bool(C.manifold_synchronize_checked(e.ctx)) {
		e.check()
		panic("sensorium: CUDA synchronization failed")
	}
	runtime.KeepAlive(e)
}

// ----------------------------------------------------------------------------
// Managed-memory views: host/device ownership changes require synchronization
// ----------------------------------------------------------------------------
type Buffer struct {
	cBuf  *C.ManifoldBuffer
	size  uint64
	owner *Engine
}

func (e *Engine) NewBuffer(bytes uint64, initialData unsafe.Pointer) *Buffer {
	if bytes == 0 || bytes > uint64(^uint(0)>>1) {
		panic("sensorium: invalid CUDA buffer size")
	}
	e.check()
	cBuf := C.manifold_create_buffer(e.ctx, C.uint64_t(bytes), initialData)
	runtime.KeepAlive(e)
	if cBuf == nil {
		e.check()
		panic("sensorium: CUDA buffer allocation failed")
	}
	b := &Buffer{cBuf: cBuf, size: bytes, owner: e}
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
	b.owner = nil
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
		b.owner.check()
		panic("sensorium: CUDA buffer is unavailable")
	}

	C.memset(pointer, 0, C.size_t(b.size))
	runtime.KeepAlive(b)
}

func (b *Buffer) Float32Slice() []float32 {
	ptr := C.manifold_get_buffer_pointer(b.cBuf)
	if ptr == nil {
		b.owner.check()
		panic("sensorium: CUDA buffer is unavailable")
	}
	view := unsafe.Slice((*float32)(ptr), b.size/4)
	runtime.KeepAlive(b)
	return view
}

func (b *Buffer) Int32Slice() []int32 {
	ptr := C.manifold_get_buffer_pointer(b.cBuf)
	if ptr == nil {
		b.owner.check()
		panic("sensorium: CUDA buffer is unavailable")
	}
	view := unsafe.Slice((*int32)(ptr), b.size/4)
	runtime.KeepAlive(b)
	return view
}

func (b *Buffer) UInt32Slice() []uint32 {
	ptr := C.manifold_get_buffer_pointer(b.cBuf)
	if ptr == nil {
		b.owner.check()
		panic("sensorium: CUDA buffer is unavailable")
	}
	view := unsafe.Slice((*uint32)(ptr), b.size/4)
	runtime.KeepAlive(b)
	return view
}

// ----------------------------------------------------------------------------
// 1. Diagnostics & Field Operations
// ----------------------------------------------------------------------------
func (e *Engine) ClearField(field *Buffer) {
	e.check()
	C.manifold_clear_field(e.ctx, field.cBuf)

	e.check()
	runtime.KeepAlive(field)
	runtime.KeepAlive(e)
}

func (e *Engine) ReduceEnergyStats(x, outStats *Buffer) {
	e.check()
	C.manifold_thermo_reduce_energy_stats(e.ctx, x.cBuf, outStats.cBuf)

	e.check()
	runtime.KeepAlive(x)
	runtime.KeepAlive(outStats)
	runtime.KeepAlive(e)
}

// ----------------------------------------------------------------------------
// 2. PIC & Sort-Based Scatter
// ----------------------------------------------------------------------------
func (e *Engine) ScatterSorted(
	pos, vel, mass, heat, energy *Buffer,
	rhoField, momField, eField *Buffer,
	numParticles int,
) {
	e.check()
	C.manifold_scatter_sorted(
		e.ctx,
		pos.cBuf, vel.cBuf, mass.cBuf, heat.cBuf, energy.cBuf,
		rhoField.cBuf, momField.cBuf, eField.cBuf,
		C.int64_t(e.GridSize[0]), C.int64_t(e.GridSize[1]), C.int64_t(e.GridSize[2]),
		C.float(e.Spacing),
	)

	e.check()
	runtime.KeepAlive(pos)
	runtime.KeepAlive(vel)
	runtime.KeepAlive(mass)
	runtime.KeepAlive(heat)
	runtime.KeepAlive(energy)
	runtime.KeepAlive(rhoField)
	runtime.KeepAlive(momField)
	runtime.KeepAlive(eField)
	runtime.KeepAlive(e)
}

func (e *Engine) PICGatherUpdate(
	posIn, mass, posOut, velOut, heatOut *Buffer,
	rhoField, momField, eField, gravityPot *Buffer,
	dbgHead, dbgWords *Buffer, dbgCapacity int,
	dt, gamma, rSpecific, cv, rhoMin, pMin, gravityEnabled float32,
) {
	e.check()
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

	e.check()
	runtime.KeepAlive(posIn)
	runtime.KeepAlive(mass)
	runtime.KeepAlive(posOut)
	runtime.KeepAlive(velOut)
	runtime.KeepAlive(heatOut)
	runtime.KeepAlive(rhoField)
	runtime.KeepAlive(momField)
	runtime.KeepAlive(eField)
	runtime.KeepAlive(gravityPot)
	runtime.KeepAlive(dbgHead)
	runtime.KeepAlive(dbgWords)
	runtime.KeepAlive(e)
}

// ----------------------------------------------------------------------------
// 3. Quantum Flow (Pilot-Wave)
// ----------------------------------------------------------------------------
func (e *Engine) ProjectModesToSpatial(
	modePsiReal, modePsiImag, modeAnchorIdx, modeAnchorWeight, particlePos *Buffer,
	psiReField, psiImField *Buffer,
	anchorsPerMode int,
) {
	e.check()
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

	e.check()
	runtime.KeepAlive(modePsiReal)
	runtime.KeepAlive(modePsiImag)
	runtime.KeepAlive(modeAnchorIdx)
	runtime.KeepAlive(modeAnchorWeight)
	runtime.KeepAlive(particlePos)
	runtime.KeepAlive(psiReField)
	runtime.KeepAlive(psiImField)
	runtime.KeepAlive(e)
}

func (e *Engine) PilotWaveGather(
	posIn, mass, posOut, velOut, psiRe, psiIm *Buffer,
	numParticles int, dt float32,
	hbarEff, epsDenom, massMin float32,
) {
	e.check()
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

	e.check()
	runtime.KeepAlive(posIn)
	runtime.KeepAlive(mass)
	runtime.KeepAlive(posOut)
	runtime.KeepAlive(velOut)
	runtime.KeepAlive(psiRe)
	runtime.KeepAlive(psiIm)
	runtime.KeepAlive(e)
}

func (e *Engine) GasRK2Stage1(
	rho0, mom0, e0, rho1, mom1, e1, k1Rho, k1Mom, k1E *Buffer,
	dbgHead, dbgWords *Buffer, dbgCapacity int,
	dt, gamma, cv, rhoMin, pMin, mu, kThermal float32,
) {
	e.check()
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

	e.check()
	runtime.KeepAlive(rho0)
	runtime.KeepAlive(mom0)
	runtime.KeepAlive(e0)
	runtime.KeepAlive(rho1)
	runtime.KeepAlive(mom1)
	runtime.KeepAlive(e1)
	runtime.KeepAlive(k1Rho)
	runtime.KeepAlive(k1Mom)
	runtime.KeepAlive(k1E)
	runtime.KeepAlive(dbgHead)
	runtime.KeepAlive(dbgWords)
	runtime.KeepAlive(e)
}

func (e *Engine) GasRK2Stage2(
	rho0, mom0, e0, rho1, mom1, e1, k1Rho, k1Mom, k1E *Buffer,
	rhoOut, momOut, eOut *Buffer,
	dbgHead, dbgWords *Buffer, dbgCapacity int,
	dt, gamma, cv, rhoMin, pMin, mu, kThermal float32,
) {
	e.check()
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

	e.check()
	runtime.KeepAlive(rho0)
	runtime.KeepAlive(mom0)
	runtime.KeepAlive(e0)
	runtime.KeepAlive(rho1)
	runtime.KeepAlive(mom1)
	runtime.KeepAlive(e1)
	runtime.KeepAlive(k1Rho)
	runtime.KeepAlive(k1Mom)
	runtime.KeepAlive(k1E)
	runtime.KeepAlive(rhoOut)
	runtime.KeepAlive(momOut)
	runtime.KeepAlive(eOut)
	runtime.KeepAlive(dbgHead)
	runtime.KeepAlive(dbgWords)
	runtime.KeepAlive(e)
}

// ----------------------------------------------------------------------------
// 5. Coherence Lattice & GPE Step
// ----------------------------------------------------------------------------
func (e *Engine) CoherenceGPEStep(
	oscPhase, oscOmega, oscAmp *Buffer,
	carrierReal, carrierImag, carrierOmega, carrierGateWidth *Buffer,
	kineticReal, kineticImag *Buffer,
	carrierAnchorIdx, carrierAnchorWeight, accums, numCarriersSnapshot, particlePos *Buffer,
	extraPotential *Buffer,
	numOsc, maxCarriers int,
	dt, hbarEff, massEff, gInteraction, energyDecay, chemPot, invDomega2 float32,
	rngSeed uint32, anchorEps, metricCoupling float32,
	metabolicRate, gateWidthMin, gateWidthMax, offenderWeightFloor, spatialSigma float32,
	geometry ...*Buffer,
) {
	e.check()
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

	var metric *C.ManifoldBuffer
	if len(geometry) > 1 {
		panic("one metric-volume buffer expected")
	}
	if len(geometry) == 1 && geometry[0] != nil {
		metric = geometry[0].cBuf
	}
	C.manifold_coherence_gpe_step_geometry(
		e.ctx,
		oscPhase.cBuf, oscOmega.cBuf, oscAmp.cBuf,
		carrierReal.cBuf, carrierImag.cBuf, carrierOmega.cBuf, carrierGateWidth.cBuf,
		kineticReal.cBuf, kineticImag.cBuf,
		carrierAnchorIdx.cBuf, carrierAnchorWeight.cBuf,
		accums.cBuf, numCarriersSnapshot.cBuf, particlePos.cBuf,
		prm, gp, extra, metric,
	)

	e.check()
	runtime.KeepAlive(oscPhase)
	runtime.KeepAlive(oscOmega)
	runtime.KeepAlive(oscAmp)
	runtime.KeepAlive(carrierReal)
	runtime.KeepAlive(carrierImag)
	runtime.KeepAlive(carrierOmega)
	runtime.KeepAlive(carrierGateWidth)
	runtime.KeepAlive(kineticReal)
	runtime.KeepAlive(kineticImag)
	runtime.KeepAlive(carrierAnchorIdx)
	runtime.KeepAlive(carrierAnchorWeight)
	runtime.KeepAlive(accums)
	runtime.KeepAlive(numCarriersSnapshot)
	runtime.KeepAlive(particlePos)
	runtime.KeepAlive(extraPotential)
	runtime.KeepAlive(e)
}

func (e *Engine) ScatterComputeCellIdx(pos, cellIdx *Buffer) {
	e.check()
	C.manifold_scatter_compute_cell_idx(
		e.ctx,
		pos.cBuf,
		cellIdx.cBuf,
		C.int64_t(e.GridSize[0]),
		C.int64_t(e.GridSize[1]),
		C.int64_t(e.GridSize[2]),
		C.float(e.Spacing),
	)

	e.check()
	runtime.KeepAlive(pos)
	runtime.KeepAlive(cellIdx)
	runtime.KeepAlive(e)
}

func (e *Engine) ScatterCountCells(cellIdx, cellCounts *Buffer) {
	e.check()
	C.manifold_scatter_count_cells(
		e.ctx,
		cellIdx.cBuf,
		cellCounts.cBuf,
		C.int64_t(e.GridSize[0]),
		C.int64_t(e.GridSize[1]),
		C.int64_t(e.GridSize[2]),
		C.float(e.Spacing),
	)

	e.check()
	runtime.KeepAlive(cellIdx)
	runtime.KeepAlive(cellCounts)
	runtime.KeepAlive(e)
}

func (e *Engine) ScatterReorderParticles(
	posIn, velIn, massIn, heatIn, energyIn *Buffer,
	cellIdx, cellStarts, cellOffsets *Buffer,
	posOut, velOut, massOut, heatOut, energyOut, originalIdx *Buffer,
) {
	e.check()
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

	e.check()
	runtime.KeepAlive(posIn)
	runtime.KeepAlive(velIn)
	runtime.KeepAlive(massIn)
	runtime.KeepAlive(heatIn)
	runtime.KeepAlive(energyIn)
	runtime.KeepAlive(cellIdx)
	runtime.KeepAlive(cellStarts)
	runtime.KeepAlive(cellOffsets)
	runtime.KeepAlive(posOut)
	runtime.KeepAlive(velOut)
	runtime.KeepAlive(massOut)
	runtime.KeepAlive(heatOut)
	runtime.KeepAlive(energyOut)
	runtime.KeepAlive(originalIdx)
	runtime.KeepAlive(e)
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
	e.check()
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

	e.check()
	runtime.KeepAlive(oscPhase)
	runtime.KeepAlive(oscOmega)
	runtime.KeepAlive(oscAmp)
	runtime.KeepAlive(particlePos)
	runtime.KeepAlive(carrierOmega)
	runtime.KeepAlive(carrierGateWidth)
	runtime.KeepAlive(carrierAnchorIdx)
	runtime.KeepAlive(carrierAnchorWeight)
	runtime.KeepAlive(accums)
	runtime.KeepAlive(binStarts)
	runtime.KeepAlive(carrierBinnedIdx)
	runtime.KeepAlive(binParams)
	runtime.KeepAlive(particleHeat)
	runtime.KeepAlive(numCarriersSnapshot)
	runtime.KeepAlive(e)
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
	e.check()
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

	e.check()
	runtime.KeepAlive(oscPhase)
	runtime.KeepAlive(oscOmega)
	runtime.KeepAlive(oscAmp)
	runtime.KeepAlive(carrierReal)
	runtime.KeepAlive(carrierImag)
	runtime.KeepAlive(carrierOmega)
	runtime.KeepAlive(carrierGateWidth)
	runtime.KeepAlive(carrierAnchorIdx)
	runtime.KeepAlive(carrierAnchorWeight)
	runtime.KeepAlive(numCarriersSnapshot)
	runtime.KeepAlive(binStarts)
	runtime.KeepAlive(carrierBinnedIdx)
	runtime.KeepAlive(binParams)
	runtime.KeepAlive(particlePos)
	runtime.KeepAlive(e)
}

// ExclusiveScanU32 dispatches a complete hierarchical GPU scan. out has n+1 words.
func (e *Engine) ExclusiveScanU32(in, out *Buffer, n int) error {
	if e == nil || e.ctx == nil || in == nil || out == nil || n < 0 {
		return fmt.Errorf("sensorium: invalid exclusive scan arguments")
	}
	ok := C.manifold_exclusive_scan_u32(e.ctx, in.cBuf, out.cBuf, C.int64_t(n))
	runtime.KeepAlive(e)
	runtime.KeepAlive(in)
	runtime.KeepAlive(out)
	if !bool(ok) {
		return fmt.Errorf("sensorium scan: %s", C.GoString(C.manifold_last_error(e.ctx)))
	}
	return nil
}
