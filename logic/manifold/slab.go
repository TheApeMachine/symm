package manifold

import (
	"encoding/binary"
	"math"
	"sync/atomic"
	"unsafe"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

const (
	slabVersion          = uint16(1)
	slabHeaderBytes      = 64
	particleStrideFloats = 12
)

var fieldSlabMagic = [4]byte{'S', 'F', 'F', '1'}
var particleSlabMagic = [4]byte{'S', 'P', 'F', '1'}

/*
slabEncoder writes manifold state in the exact Float32 layouts consumed by the
browser GPU. Field payloads preserve Metal's momentum.xyz/density.w texture.
*/
type slabEncoder struct {
	config   pmanifold.Config
	sequence atomic.Uint64
}

/*
Fields copies one complete resident domain through one cgo call into its final
wire allocation. The payload is immediately usable as four GPU texture views.
*/
func (encoder *slabEncoder) Fields(physics *pmanifold.Solver) ([]byte, error) {
	cellCount := int(encoder.config.GridX * encoder.config.GridY * encoder.config.GridZ)
	momRhoOffset := slabHeaderBytes
	energyOffset := momRhoOffset + cellCount*4*4
	waveRealOffset := energyOffset + cellCount*4
	waveImaginaryOffset := waveRealOffset + cellCount*4
	byteLength := waveImaginaryOffset + cellCount*4
	payload := make([]byte, byteLength)

	scale, err := physics.ReadDomainInto(
		float32View(payload, momRhoOffset, cellCount*4),
		float32View(payload, energyOffset, cellCount),
		float32View(payload, waveRealOffset, cellCount),
		float32View(payload, waveImaginaryOffset, cellCount),
	)

	if err != nil {
		return nil, err
	}

	copy(payload, fieldSlabMagic[:])
	putUint16(payload, 4, slabVersion)
	putUint16(payload, 6, slabHeaderBytes)
	putUint64(payload, 8, encoder.sequence.Add(1))
	putUint32(payload, 16, encoder.config.GridX)
	putUint32(payload, 20, encoder.config.GridY)
	putUint32(payload, 24, encoder.config.GridZ)
	putFloat32(payload, 28, normalizedGridSpacing(encoder.config))
	putFloat32(payload, 32, inverseScale(scale.Density))
	putFloat32(payload, 36, inverseScale(scale.Momentum))
	putFloat32(payload, 40, inverseScale(scale.Energy))
	putFloat32(payload, 44, inverseScale(scale.Wave))
	putUint32(payload, 48, uint32(energyOffset))
	putUint32(payload, 52, uint32(waveRealOffset))
	putUint32(payload, 56, uint32(waveImaginaryOffset))
	putUint32(payload, 60, uint32(byteLength))

	return payload, nil
}

/*
Particles writes one interleaved particle buffer. No per-particle wire objects
are allocated; the browser binds attributes at these fixed float offsets.
*/
func (encoder *slabEncoder) Particles(
	oscillators []pmanifold.Oscillator,
) []byte {
	byteLength := slabHeaderBytes + len(oscillators)*particleStrideFloats*4
	payload := make([]byte, byteLength)
	values := float32View(payload, slabHeaderBytes, len(oscillators)*particleStrideFloats)
	maximumHeat, maximumEnergy, maximumMass := float32(0), float32(0), float32(0)

	for index, oscillator := range oscillators {
		offset := index * particleStrideFloats
		amplitude := float32(oscillator.Amplitude)
		heat := float32(oscillator.Heat)
		energy := amplitude * amplitude
		values[offset+0] = float32(oscillator.PosX / encoder.config.DomainX)
		values[offset+1] = float32(oscillator.PosY / encoder.config.DomainY)
		values[offset+2] = float32(oscillator.PosZ / encoder.config.DomainZ)
		values[offset+3] = float32(oscillator.VelX / encoder.config.DomainX)
		values[offset+4] = float32(oscillator.VelY / encoder.config.DomainY)
		values[offset+5] = float32(oscillator.VelZ / encoder.config.DomainZ)
		values[offset+6] = amplitude
		values[offset+7] = heat
		values[offset+8] = energy
		values[offset+9] = float32(oscillator.Phase)
		values[offset+10] = float32(oscillator.Omega)
		values[offset+11] = amplitude
		maximumHeat = max(maximumHeat, float32(math.Abs(float64(heat))))
		maximumEnergy = max(maximumEnergy, float32(math.Abs(float64(energy))))
		maximumMass = max(maximumMass, float32(math.Abs(float64(amplitude))))
	}

	copy(payload, particleSlabMagic[:])
	putUint16(payload, 4, slabVersion)
	putUint16(payload, 6, slabHeaderBytes)
	putUint64(payload, 8, encoder.sequence.Add(1))
	putUint32(payload, 16, uint32(len(oscillators)))
	putUint32(payload, 20, particleStrideFloats)
	putFloat32(payload, 24, inverseScale(maximumHeat))
	putFloat32(payload, 28, inverseScale(maximumEnergy))
	putFloat32(payload, 32, inverseScale(maximumMass))
	putUint32(payload, 36, uint32(byteLength))

	return payload
}

func float32View(payload []byte, offset, length int) []float32 {
	if length == 0 {
		return nil
	}

	return unsafe.Slice((*float32)(unsafe.Pointer(&payload[offset])), length)
}

func inverseScale(maximum float32) float32 {
	if maximum == 0 {
		return 0
	}

	return 1 / maximum
}

func normalizedGridSpacing(config pmanifold.Config) float32 {
	maximumAxis := max(config.GridX, config.GridY, config.GridZ)
	return 1 / float32(maximumAxis)
}

func putUint16(payload []byte, offset int, value uint16) {
	binary.LittleEndian.PutUint16(payload[offset:], value)
}

func putUint32(payload []byte, offset int, value uint32) {
	binary.LittleEndian.PutUint32(payload[offset:], value)
}

func putUint64(payload []byte, offset int, value uint64) {
	binary.LittleEndian.PutUint64(payload[offset:], value)
}

func putFloat32(payload []byte, offset int, value float32) {
	putUint32(payload, offset, math.Float32bits(value))
}
