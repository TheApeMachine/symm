package manifold

import (
	"encoding/binary"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSlabEncoderParticles(t *testing.T) {
	Convey("Given one resident oscillator", t, func() {
		encoder := slabEncoder{config: Domain{
			DomainX: 2,
			DomainY: 4,
			DomainZ: 8,
		}}
		payload := encoder.Particles([]Oscillator{{
			PosX:      1,
			PosY:      2,
			PosZ:      4,
			VelX:      2,
			VelY:      4,
			VelZ:      8,
			Amplitude: 2,
			Heat:      4,
			Phase:     0.5,
			Omega:     3,
		}})

		Convey("It should emit the direct interleaved GPU layout and exact scales", func() {
			So(string(payload[:4]), ShouldEqual, "SPF1")
			So(binary.LittleEndian.Uint16(payload[4:]), ShouldEqual, slabVersion)
			So(binary.LittleEndian.Uint32(payload[16:]), ShouldEqual, uint32(1))
			So(binary.LittleEndian.Uint32(payload[20:]), ShouldEqual, uint32(particleStrideFloats))
			So(math.Float32frombits(binary.LittleEndian.Uint32(payload[24:])), ShouldEqual, float32(0.25))
			So(math.Float32frombits(binary.LittleEndian.Uint32(payload[28:])), ShouldEqual, float32(0.25))
			So(math.Float32frombits(binary.LittleEndian.Uint32(payload[32:])), ShouldEqual, float32(0.5))
			So(float32View(payload, slabHeaderBytes, particleStrideFloats), ShouldResemble, []float32{
				0.5, 0.5, 0.5,
				1, 1, 1,
				2, 4, 4, 0.5, 3, 2,
			})
		})
	})

	Convey("Given an empty resident domain", t, func() {
		encoder := slabEncoder{}

		Convey("It should emit a valid header-only particle slab", func() {
			payload := encoder.Particles(nil)
			So(payload, ShouldHaveLength, slabHeaderBytes)
			So(string(payload[:4]), ShouldEqual, "SPF1")
			So(binary.LittleEndian.Uint32(payload[16:]), ShouldEqual, uint32(0))
		})
	})
}

func BenchmarkSlabEncoderParticles(b *testing.B) {
	encoder := slabEncoder{config: Domain{
		DomainX: 1,
		DomainY: 1,
		DomainZ: 1,
	}}
	oscillators := make([]Oscillator, oscillatorPoolCapacity)
	b.ReportAllocs()

	for b.Loop() {
		_ = encoder.Particles(oscillators)
	}
}
