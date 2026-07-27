package category

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestPairMemoryObserve(t *testing.T) {
	Convey("Given a fresh pairMemory", t, func() {
		memory := newPairMemory()

		Convey("When observing positive strength", func() {
			memory.observe("SIM/USD", types.VerticalIgnition, 0.8)

			Convey("It should accumulate solo mass and total", func() {
				key := makeNodeKey("SIM/USD", types.VerticalIgnition)
				So(memory.solo[key], ShouldAlmostEqual, 0.8)
				So(memory.total["SIM/USD"], ShouldAlmostEqual, 0.8)
			})
		})

		Convey("When observing zero or negative strength", func() {
			memory.observe("SIM/USD", types.VerticalIgnition, 0)
			memory.observe("SIM/USD", types.OrganicTrend, -1)

			Convey("It should not record anything", func() {
				So(len(memory.solo), ShouldEqual, 0)
				So(len(memory.total), ShouldEqual, 0)
			})
		})

		Convey("When observing on a nil memory", func() {
			var nilMemory *pairMemory

			Convey("It should not panic", func() {
				So(func() {
					nilMemory.observe("SIM/USD", types.VerticalIgnition, 0.5)
				}, ShouldNotPanic)
			})
		})
	})
}

func TestPairMemoryCoobserve(t *testing.T) {
	Convey("Given a fresh pairMemory", t, func() {
		memory := newPairMemory()

		Convey("When coobserving two different categories", func() {
			memory.coobserve("SIM/USD", types.VerticalIgnition, types.OrganicTrend, 0.8, 0.6)

			Convey("It should accumulate joint mass with canonical ordering", func() {
				So(len(memory.joint), ShouldEqual, 1)
			})
		})

		Convey("When coobserving the same category type", func() {
			memory.coobserve("SIM/USD", types.VerticalIgnition, types.VerticalIgnition, 0.8, 0.8)

			Convey("It should skip self-pairs", func() {
				So(len(memory.joint), ShouldEqual, 0)
			})
		})

		Convey("When coobserving with zero strength", func() {
			memory.coobserve("SIM/USD", types.VerticalIgnition, types.OrganicTrend, 0, 0.6)

			Convey("It should skip the observation", func() {
				So(len(memory.joint), ShouldEqual, 0)
			})
		})
	})
}

func TestPairMemoryIndependent(t *testing.T) {
	Convey("Given a pairMemory with accumulated observations", t, func() {
		memory := newPairMemory()

		Convey("When solo mass is high but joint mass is low", func() {
			memory.solo[makeNodeKey("SIM/USD", types.VerticalIgnition)] = 10.0
			memory.solo[makeNodeKey("SIM/USD", types.OrganicTrend)] = 10.0
			memory.total["SIM/USD"] = 20.0

			memory.joint[makePairKey("SIM/USD", types.VerticalIgnition, types.OrganicTrend)] = 0.1

			mass, independent := memory.independent(
				"SIM/USD", types.VerticalIgnition, types.OrganicTrend, 0.8, 0.6,
			)

			Convey("It should report independence with positive mass", func() {
				So(independent, ShouldBeTrue)
				So(mass, ShouldBeGreaterThan, 0)
			})
		})

		Convey("When joint mass exceeds the product baseline", func() {
			memory.solo[makeNodeKey("SIM/USD", types.VerticalIgnition)] = 5.0
			memory.solo[makeNodeKey("SIM/USD", types.OrganicTrend)] = 5.0
			memory.total["SIM/USD"] = 10.0

			memory.joint[makePairKey("SIM/USD", types.VerticalIgnition, types.OrganicTrend)] = 10.0

			_, independent := memory.independent(
				"SIM/USD", types.VerticalIgnition, types.OrganicTrend, 0.8, 0.6,
			)

			Convey("It should not report independence", func() {
				So(independent, ShouldBeFalse)
			})
		})

		Convey("When total is zero", func() {
			_, independent := memory.independent(
				"SIM/USD", types.VerticalIgnition, types.OrganicTrend, 0.8, 0.6,
			)

			Convey("It should not report independence", func() {
				So(independent, ShouldBeFalse)
			})
		})

		Convey("When nil pairMemory", func() {
			var nilMemory *pairMemory
			_, independent := nilMemory.independent(
				"SIM/USD", types.VerticalIgnition, types.OrganicTrend, 0.8, 0.6,
			)

			Convey("It should return false without panicking", func() {
				So(independent, ShouldBeFalse)
			})
		})
	})
}

func BenchmarkPairMemoryObserve(b *testing.B) {
	memory := newPairMemory()

	b.ReportAllocs()

	for b.Loop() {
		memory.observe("SIM/USD", types.VerticalIgnition, 0.8)
	}
}

func BenchmarkPairMemoryCoobserve(b *testing.B) {
	memory := newPairMemory()

	b.ReportAllocs()

	for b.Loop() {
		memory.coobserve(
			"SIM/USD",
			types.VerticalIgnition, types.OrganicTrend,
			0.8, 0.6,
		)
	}
}

func BenchmarkPairMemoryIndependent(b *testing.B) {
	memory := newPairMemory()
	memory.solo[makeNodeKey("SIM/USD", types.VerticalIgnition)] = 10.0
	memory.solo[makeNodeKey("SIM/USD", types.OrganicTrend)] = 10.0
	memory.total["SIM/USD"] = 20.0

	memory.joint[makePairKey("SIM/USD", types.VerticalIgnition, types.OrganicTrend)] = 0.1

	b.ReportAllocs()

	for b.Loop() {
		memory.independent(
			"SIM/USD", types.VerticalIgnition, types.OrganicTrend, 0.8, 0.6,
		)
	}
}
