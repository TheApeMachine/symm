package manifold

import (
	"encoding/binary"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
)

func f32bits(value float32) uint32 {
	return math.Float32bits(value)
}

func f32read(target []byte, offset int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(target[offset : offset+4]))
}

func EncodeFieldsSlabTest(t *testing.T) {
	Convey("Given a small synthetic field", t, func() {
		cells := 2 * 2 * 2
		momRho := make([]float32, cells*4)
		energy := make([]float32, cells)
		waveReal := make([]float32, cells)
		waveImag := make([]float32, cells)

		for index := range momRho {
			momRho[index] = float32(index + 1)
		}

		for index := range energy {
			energy[index] = float32(index) * 0.5
			waveReal[index] = float32(index) * 0.25
			waveImag[index] = -float32(index) * 0.25
		}

		Convey("It should pack the exact SFF1 header and field layout", func() {
			slab := encodeFieldsSlab(7, 2, 2, 2, 0.5, 1, 2, 3, 4, momRho, energy, waveReal, waveImag)

			So(string(slab[0:4]), ShouldEqual, "SFF1")
			So(binary.LittleEndian.Uint16(slab[4:6]), ShouldEqual, 1)
			So(binary.LittleEndian.Uint16(slab[6:8]), ShouldEqual, 64)
			So(binary.LittleEndian.Uint64(slab[8:16]), ShouldEqual, 7)
			So(binary.LittleEndian.Uint32(slab[16:20]), ShouldEqual, 2)
			So(binary.LittleEndian.Uint32(slab[20:24]), ShouldEqual, 2)
			So(binary.LittleEndian.Uint32(slab[24:28]), ShouldEqual, 2)
			So(f32read(slab, 28), ShouldEqual, 0.5)
			So(f32read(slab, 32), ShouldEqual, 1)
			So(f32read(slab, 36), ShouldEqual, 2)
			So(f32read(slab, 40), ShouldEqual, 3)
			So(f32read(slab, 44), ShouldEqual, 4)

			energyOffset := 64 + cells*4*4
			waveRealOffset := energyOffset + cells*4
			waveImagOffset := waveRealOffset + cells*4

			So(binary.LittleEndian.Uint32(slab[48:52]), ShouldEqual, uint32(energyOffset))
			So(binary.LittleEndian.Uint32(slab[52:56]), ShouldEqual, uint32(waveRealOffset))
			So(binary.LittleEndian.Uint32(slab[56:60]), ShouldEqual, uint32(waveImagOffset))
			So(binary.LittleEndian.Uint32(slab[60:64]), ShouldEqual, uint32(len(slab)))

			// momRho keeps the 4-float-per-cell interleave starting at 64.
			So(f32read(slab, 64), ShouldEqual, 1)
			So(f32read(slab, 64+4*4), ShouldEqual, 5)
			So(f32read(slab, energyOffset), ShouldEqual, 0)
			So(f32read(slab, energyOffset+4), ShouldEqual, 0.5)
			So(f32read(slab, waveRealOffset+4), ShouldEqual, 0.25)
			So(f32read(slab, waveImagOffset+4), ShouldEqual, -0.25)
			So(len(slab), ShouldEqual, waveImagOffset+cells*4)
		})
	})
}

func EncodeParticlesSlabTest(t *testing.T) {
	Convey("Given a two-oscillator resident state", t, func() {
		state := &sensorium.State{
			N:      2,
			Pos:    []float32{0, 1, 2, 3, 4, 5},
			Vel:    []float32{6, 7, 8, 9, 10, 11},
			Mass:   []float32{1, 2},
			Heat:   []float32{3, 4},
			Energy: []float32{5, 6},
			Phase:  []float32{0.1, 0.2},
			Omega:  []float32{1, 2},
			Amp:    []float32{0.5, 0.7},
		}

		Convey("It should pack the exact SPF1 header and 12-float interleave", func() {
			slab := encodeParticlesSlab(9, state, 4, 6, 2)

			So(string(slab[0:4]), ShouldEqual, "SPF1")
			So(binary.LittleEndian.Uint64(slab[8:16]), ShouldEqual, 9)
			So(binary.LittleEndian.Uint32(slab[16:20]), ShouldEqual, 2)
			So(binary.LittleEndian.Uint32(slab[20:24]), ShouldEqual, 12)
			So(f32read(slab, 24), ShouldEqual, 4)
			So(f32read(slab, 28), ShouldEqual, 6)
			So(f32read(slab, 32), ShouldEqual, 2)
			So(binary.LittleEndian.Uint32(slab[36:40]), ShouldEqual, uint32(len(slab)))

			expectedRow := []float32{0, 1, 2, 6, 7, 8, 1, 3, 5, 0.1, 1, 0.5}
			expectedSecond := []float32{3, 4, 5, 9, 10, 11, 2, 4, 6, 0.2, 2, 0.7}

			for index, value := range expectedRow {
				So(f32read(slab, 64+index*4), ShouldEqual, value)
			}

			for index, value := range expectedSecond {
				So(f32read(slab, 64+12*4+index*4), ShouldEqual, value)
			}
		})
	})
}

func EncodePhaseSlabTest(t *testing.T) {
	Convey("Given a resident state with more oscillators than the cap", t, func() {
		count := 500
		state := &sensorium.State{
			N:     count,
			Phase: make([]float32, count),
			Omega: make([]float32, count),
		}

		for index := 0; index < count; index++ {
			state.Phase[index] = float32(index) * 0.01
			state.Omega[index] = float32(index) * 0.1
		}

		reading := sensorium.Reading{
			Divergence:       1.5,
			GuidanceSpeed:    2.5,
			CoherenceMag2:    3.5,
			PressureGradNorm: 4.5,
			ViscosityProxy:   5.5,
			KuramotoR:        0.75,
		}
		modeOmega := []float32{1, 2, 3}
		modeReal := []float32{0.1, 0.2, 0.3}
		modeImag := []float32{-0.1, -0.2, -0.3}
		modeLinewidth := []float32{0.5, 0.5, 0.5}

		Convey("It should pack the reading, downsample oscillators, and append the modes", func() {
			slab := encodePhaseSlab(3, reading, state, modeOmega, modeReal, modeImag, modeLinewidth)

			So(string(slab[0:4]), ShouldEqual, "SPH1")
			So(binary.LittleEndian.Uint64(slab[8:16]), ShouldEqual, 3)
			So(f32read(slab, 16), ShouldEqual, 1.5)
			So(f32read(slab, 20), ShouldEqual, 2.5)
			So(f32read(slab, 24), ShouldEqual, 3.5)
			So(f32read(slab, 28), ShouldEqual, 4.5)
			So(f32read(slab, 32), ShouldEqual, 5.5)
			So(f32read(slab, 36), ShouldEqual, 0.75)

			// 500 oscillators downsample to the 256 cap by uniform stride:
			// source = index*500/256, which stays below 500 for every index.
			So(binary.LittleEndian.Uint32(slab[40:44]), ShouldEqual, 256)
			So(binary.LittleEndian.Uint32(slab[44:48]), ShouldEqual, 3)
			So(binary.LittleEndian.Uint32(slab[48:52]), ShouldEqual, uint32(len(slab)))

			// First sampled oscillator is index 0; second is index 1 (500/256).
			So(f32read(slab, 64), ShouldEqual, 0)
			So(f32read(slab, 64+256*4), ShouldEqual, 0)
			So(f32read(slab, 64+4), ShouldEqual, 0.01)
			So(f32read(slab, 64+256*4+4), ShouldEqual, 0.1)

			modesOffset := 64 + 256*8
			So(f32read(slab, modesOffset), ShouldEqual, 1)
			So(f32read(slab, modesOffset+3*4), ShouldEqual, 0.2)
			So(f32read(slab, modesOffset+3*4*2), ShouldEqual, -0.3)
			So(f32read(slab, modesOffset+3*4*3), ShouldEqual, 0.5)
		})
	})
}

func BenchmarkEncodeParticlesSlab(b *testing.B) {
	state := &sensorium.State{
		N:     4096,
		Pos:   make([]float32, 4096*3),
		Vel:   make([]float32, 4096*3),
		Mass:  make([]float32, 4096),
		Heat:  make([]float32, 4096),
		Energy: make([]float32, 4096),
		Phase: make([]float32, 4096),
		Omega: make([]float32, 4096),
		Amp:   make([]float32, 4096),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = encodeParticlesSlab(1, state, 1, 1, 1)
	}
}
