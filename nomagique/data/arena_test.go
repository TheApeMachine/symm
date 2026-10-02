package data

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestArenaOwnerSequenceAdvancement(t *testing.T) {
	Convey("Given an ArenaOwner configured with capacity 256 and window 256", t, func() {
		owner := NewArenaOwner(256)
		owner.SetWindow(256)

		gen0 := owner.CurrentGeneration()
		So(gen0, ShouldNotBeNil)
		So(gen0.RefCount(), ShouldEqual, 1)

		m0 := owner.NewMeasurement("test")
		m0.WriteMetric("price", 100.0)
		So(m0.Source, ShouldEqual, "test")

		Convey("Advancing within generation 0 keeps gen0 active and unsealed", func() {
			for seq := int64(1); seq < 256; seq++ {
				owner.Advance(seq)
				measurement := owner.NewMeasurement("test")
				measurement.WriteMetric("price", float64(seq))
			}

			So(owner.CurrentGeneration(), ShouldEqual, gen0)
			So(gen0.IsFreed(), ShouldBeFalse)
		})

		Convey("Advancing past generation 0 boundary rotates to gen1 without freeing gen0", func() {
			owner.Advance(256)
			gen1 := owner.CurrentGeneration()
			So(gen1, ShouldNotEqual, gen0)
			So(gen0.IsFreed(), ShouldBeFalse)

			// Even at sequence 500 (still within safe margin of window + capacity), gen0 remains alive
			owner.Advance(500)
			So(gen0.IsFreed(), ShouldBeFalse)

			// Once sequence advances past endSeq + safeMargin (255 + 256 + 256 = 767), gen0 is retired
			owner.Advance(768)
			So(gen0.IsFreed(), ShouldBeTrue)
			So(gen1.IsFreed(), ShouldBeFalse)

			owner.Close()
		})

		Convey("Async Tee retention prevents deallocation even after synchronous pipeline advances", func() {
			pub := NewPublication(m0, gen0)
			pub.Retain()
			So(gen0.RefCount(), ShouldEqual, 2)

			// Advance well past gen0 retirement threshold
			owner.Advance(1024)

			// Generation is sealed by pipeline, but refcount from async Tee keeps underlying arena alive
			So(gen0.IsFreed(), ShouldBeFalse)

			// When async Tee finally releases, arena is freed
			pub.Release()
			So(gen0.IsFreed(), ShouldBeTrue)

			owner.Close()
		})
	})
}
