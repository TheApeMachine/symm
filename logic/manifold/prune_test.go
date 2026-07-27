package manifold

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

func TestRetainAboveDustTail(t *testing.T) {
	Convey("Given a mixed contribution population", t, func() {
		particles := []pfluid.Particle{
			{Mass: 1, Energy: 1, Heat: 0},
			{Mass: 1, Energy: 1, Heat: 0},
			{Mass: 0, Energy: 1e-6, Heat: 0},
			{Mass: 0, Energy: 10, Heat: 10},
		}

		Convey("It keeps meaningful contributors and drops only the inert dust tail", func() {
			So(retainAboveDustTail(particles), ShouldResemble, []uint32{0, 1})
		})
	})

	Convey("Given a skewed multi-symbol intensity mix", t, func() {
		particles := []pfluid.Particle{
			{Mass: 1, Energy: 4, Heat: 0},
			{Mass: 1, Energy: 2, Heat: 0},
			{Mass: 1, Energy: 8, Heat: 0},
			{Mass: 1, Energy: 4, Heat: 0},
		}

		Convey("It keeps the whole active cohort instead of halving the population", func() {
			kept := retainAboveDustTail(particles)
			So(len(kept), ShouldEqual, 4)
			So(kept, ShouldResemble, []uint32{0, 1, 2, 3})
		})
	})

	Convey("Given one weak positive among many active contributors", t, func() {
		particles := []pfluid.Particle{
			{Mass: 1, Energy: 8, Heat: 0},
			{Mass: 1, Energy: 7, Heat: 0},
			{Mass: 1, Energy: 6, Heat: 0},
			{Mass: 1, Energy: 5, Heat: 0},
			{Mass: 1, Energy: 4, Heat: 0},
			{Mass: 1, Energy: 3, Heat: 0},
			{Mass: 1, Energy: 2, Heat: 0},
			{Mass: 1, Energy: 0.001, Heat: 0},
		}

		Convey("It trims only the weakest dust tail contributor", func() {
			So(retainAboveDustTail(particles), ShouldResemble, []uint32{0, 1, 2, 3, 4, 5, 6})
		})
	})

	Convey("Given only zero-contribution particles", t, func() {
		particles := []pfluid.Particle{
			{Mass: 0, Energy: 1, Heat: 1},
			{Mass: 1, Energy: 0, Heat: 0},
		}

		Convey("It returns an empty keep-set so prune will not wipe the domain", func() {
			So(retainAboveDustTail(particles), ShouldBeEmpty)
		})
	})
}

func BenchmarkRetainAboveDustTail(b *testing.B) {
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
		_ = retainAboveDustTail(particles)
	}
}
