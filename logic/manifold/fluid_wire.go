package manifold

import (
	"encoding/binary"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/physics/sensorium"
)

/*
The fluid wire contract the /fluid viewer decodes. The three channels carry
self-describing 64-byte-header slabs: SFF1 (Eulerian gas + wave fields),
SPF1 (the resident oscillator gas), and SPH1 (phase reading + spectral modes +
downsampled oscillators). The layouts are mirrored exactly on the frontend in
frontend/src/components/fluid-3d/wire.ts — change either side only with the
other.
*/
const (
	fluidSlabHeaderSize = 64
	fluidSlabVersion    = 1
	fluidFieldsMagic    = "SFF1"
	fluidParticlesMagic = "SPF1"
	fluidPhaseMagic     = "SPH1"

	// fluidParticleStride is one oscillator row: Pos3 + Vel3 + Mass + Heat +
	// Energy + Phase + Omega + Amp.
	fluidParticleStride = 12

	// fluidOscillatorCap bounds the phase slab's oscillator sample so the
	// Kuramoto ring stays legible regardless of resident gas size.
	fluidOscillatorCap = 256
)

func putString(target []byte, offset int, value string) {
	copy(target[offset:], value)
}

func putU16(target []byte, offset int, value uint16) {
	binary.LittleEndian.PutUint16(target[offset:], value)
}

func putU32(target []byte, offset int, value uint32) {
	binary.LittleEndian.PutUint32(target[offset:], value)
}

func putU64(target []byte, offset int, value uint64) {
	binary.LittleEndian.PutUint64(target[offset:], value)
}

func putF32(target []byte, offset int, value float32) {
	binary.LittleEndian.PutUint32(target[offset:], math.Float32bits(value))
}

func float32Bytes(values []float32) []byte {
	if len(values) == 0 {
		return nil
	}

	return unsafe.Slice((*byte)(unsafe.Pointer(&values[0])), len(values)*4)
}

func float32At(slab []byte, offset int) []float32 {
	return unsafe.Slice((*float32)(unsafe.Pointer(&slab[offset])), (len(slab)-offset)/4)
}

/*
encodeFieldsSlab packs the resident Eulerian fields into one SFF1 slab. momRho
carries four floats per cell — momentum xyz then density — matching the
sensorium packFields layout exactly.
*/
func encodeFieldsSlab(
	sequence uint64,
	gridX, gridY, gridZ int,
	spacing float32,
	densityScale, momentumScale, energyScale, waveScale float32,
	momRho, energy, waveReal, waveImag []float32,
) []byte {
	cells := gridX * gridY * gridZ
	energyOffset := fluidSlabHeaderSize + cells*4*4
	waveRealOffset := energyOffset + cells*4
	waveImaginaryOffset := waveRealOffset + cells*4
	byteLength := waveImaginaryOffset + cells*4

	slab := make([]byte, byteLength)
	putString(slab, 0, fluidFieldsMagic)
	putU16(slab, 4, fluidSlabVersion)
	putU16(slab, 6, fluidSlabHeaderSize)
	putU64(slab, 8, sequence)
	putU32(slab, 16, uint32(gridX))
	putU32(slab, 20, uint32(gridY))
	putU32(slab, 24, uint32(gridZ))
	putF32(slab, 28, spacing)
	putF32(slab, 32, densityScale)
	putF32(slab, 36, momentumScale)
	putF32(slab, 40, energyScale)
	putF32(slab, 44, waveScale)
	putU32(slab, 48, uint32(energyOffset))
	putU32(slab, 52, uint32(waveRealOffset))
	putU32(slab, 56, uint32(waveImaginaryOffset))
	putU32(slab, 60, uint32(byteLength))

	copy(slab[fluidSlabHeaderSize:], float32Bytes(momRho))
	copy(slab[energyOffset:], float32Bytes(energy))
	copy(slab[waveRealOffset:], float32Bytes(waveReal))
	copy(slab[waveImaginaryOffset:], float32Bytes(waveImag))

	return slab
}

/*
encodeParticlesSlab packs the resident oscillator gas into one SPF1 slab, one
interleaved fluidParticleStride row per oscillator.
*/
func encodeParticlesSlab(
	sequence uint64,
	state *sensorium.State,
	heatScale, energyScale, massScale float32,
) []byte {
	count := state.N
	byteLength := fluidSlabHeaderSize + count*fluidParticleStride*4

	slab := make([]byte, byteLength)
	putString(slab, 0, fluidParticlesMagic)
	putU16(slab, 4, fluidSlabVersion)
	putU16(slab, 6, fluidSlabHeaderSize)
	putU64(slab, 8, sequence)
	putU32(slab, 16, uint32(count))
	putU32(slab, 20, fluidParticleStride)
	putF32(slab, 24, heatScale)
	putF32(slab, 28, energyScale)
	putF32(slab, 32, massScale)
	putU32(slab, 36, uint32(byteLength))

	values := float32At(slab, fluidSlabHeaderSize)

	for index := 0; index < count; index++ {
		offset := index * fluidParticleStride
		values[offset+0] = state.Pos[index*3+0]
		values[offset+1] = state.Pos[index*3+1]
		values[offset+2] = state.Pos[index*3+2]
		values[offset+3] = state.Vel[index*3+0]
		values[offset+4] = state.Vel[index*3+1]
		values[offset+5] = state.Vel[index*3+2]
		values[offset+6] = state.Mass[index]
		values[offset+7] = state.Heat[index]
		values[offset+8] = state.Energy[index]
		values[offset+9] = state.Phase[index]
		values[offset+10] = state.Omega[index]
		values[offset+11] = state.Amp[index]
	}

	return slab
}

/*
encodePhaseSlab packs the phase reading, the spectral mode lattice, and a
downsampled oscillator phase sample into one SPH1 slab.
*/
func encodePhaseSlab(
	sequence uint64,
	reading sensorium.Reading,
	state *sensorium.State,
	modeOmega, modeReal, modeImag, modeLinewidth []float32,
) []byte {
	oscillatorCount := len(state.Phase)

	if oscillatorCount > fluidOscillatorCap {
		oscillatorCount = fluidOscillatorCap
	}

	modeCount := len(modeOmega)
	phasesOffset := fluidSlabHeaderSize
	omegasOffset := phasesOffset + oscillatorCount*4
	modesOffset := omegasOffset + oscillatorCount*4
	modeRealOffset := modesOffset + modeCount*4
	modeImagOffset := modeRealOffset + modeCount*4
	modeLinewidthOffset := modeImagOffset + modeCount*4
	byteLength := modeLinewidthOffset + modeCount*4

	slab := make([]byte, byteLength)
	putString(slab, 0, fluidPhaseMagic)
	putU16(slab, 4, fluidSlabVersion)
	putU16(slab, 6, fluidSlabHeaderSize)
	putU64(slab, 8, sequence)
	putF32(slab, 16, float32(reading.Divergence))
	putF32(slab, 20, float32(reading.GuidanceSpeed))
	putF32(slab, 24, float32(reading.CoherenceMag2))
	putF32(slab, 28, float32(reading.PressureGradNorm))
	putF32(slab, 32, float32(reading.ViscosityProxy))
	putF32(slab, 36, float32(reading.KuramotoR))
	putU32(slab, 40, uint32(oscillatorCount))
	putU32(slab, 44, uint32(modeCount))
	putU32(slab, 48, uint32(byteLength))

	phases := float32At(slab, phasesOffset)
	omegas := float32At(slab, omegasOffset)

	// Uniform stride sampling: source = index*N/cap stays strictly below N for
	// every index below the cap, so a crowded gas cannot index past its rows.
	particleCount := len(state.Phase)

	for index := 0; index < oscillatorCount; index++ {
		source := index * particleCount / fluidOscillatorCap
		phases[index] = state.Phase[source]
		omegas[index] = state.Omega[source]
	}

	copy(slab[modesOffset:], float32Bytes(modeOmega))
	copy(slab[modeRealOffset:], float32Bytes(modeReal))
	copy(slab[modeImagOffset:], float32Bytes(modeImag))
	copy(slab[modeLinewidthOffset:], float32Bytes(modeLinewidth))

	return slab
}
