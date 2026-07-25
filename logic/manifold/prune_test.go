package manifold

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

func TestRetainAboveMedian(t *testing.T) {
	Convey("Given a mixed contribution population", t, func() {
		particles := []pfluid.Particle{
			{Mass: 1, Energy: 1, Heat: 0},
			{Mass: 1, Energy: 1, Heat: 0},
			{Mass: 0.01, Energy: 1e-6, Heat: 0},
			{Mass: 0, Energy: 10, Heat: 10},
		}

		Convey("It keeps at-or-above-median contributors and drops dust", func() {
			So(retainAboveMedian(particles), ShouldResemble, []uint32{0, 1})
		})
	})

	Convey("Given a skewed multi-symbol intensity mix", t, func() {
		particles := []pfluid.Particle{
			{Mass: 1, Energy: 4, Heat: 0},
			{Mass: 1, Energy: 2, Heat: 0},
			{Mass: 1, Energy: 8, Heat: 0},
			{Mass: 1, Energy: 4, Heat: 0},
		}

		Convey("It must not collapse onto the single hottest oscillator", func() {
			kept := retainAboveMedian(particles)
			So(len(kept), ShouldBeGreaterThan, 1)
			So(kept, ShouldResemble, []uint32{0, 2, 3})
		})
	})

	Convey("Given only zero-contribution particles", t, func() {
		particles := []pfluid.Particle{
			{Mass: 0, Energy: 1, Heat: 1},
			{Mass: 1, Energy: 0, Heat: 0},
		}

		Convey("It returns an empty keep-set so prune will not wipe the domain", func() {
			So(retainAboveMedian(particles), ShouldBeEmpty)
		})
	})
}

func BenchmarkRetainAboveMedian(b *testing.B) {
	particles := make([]pfluid.Particle, 4096)

	for index := range particles {
		particles[index] = pfluid.Particle{
			Mass:   1,
			Energy: float32(1 + index%7),
			Heat:   1e-4,
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = retainAboveMedian(particles)
	}
}
