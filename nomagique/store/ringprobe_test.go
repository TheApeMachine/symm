package store

import (
	container "container/ring"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestRingNext(t *testing.T) {
	Convey("Ring plays one child sequence per run and advances the parent", t, func() {
		child := func(values ...float64) *container.Ring {
			ring := container.New(len(values))

			for _, value := range values {
				ring.Value = value
				ring = ring.Next()
			}

			return ring
		}

		parent := container.New(2)
		parent.Value = child(1, 2)
		parent = parent.Next()
		parent.Value = child(3, 4)
		parent = parent.Next()

		op := NewRingOver[float64](parent)

		So(tests.CollectSeq(op.Next(nil)), ShouldResemble, []float64{1, 2})
		So(tests.CollectSeq(op.Next(nil)), ShouldResemble, []float64{3, 4})
		So(tests.CollectSeq(op.Next(nil)), ShouldResemble, []float64{1, 2})
	})
}

func TestRingWrite(t *testing.T) {
	Convey("A ring written into plays what was written, in order", t, func() {
		child := func(values ...float64) *Ring[float64] {
			held := NewRing[float64]()

			for _, value := range values {
				held.Write(value)
			}

			return held
		}

		parent := NewRing[float64]()
		parent.Write(child(1, 2, 3).Held())
		parent.Write(child(4, 5).Held())

		op := NewRingOver[float64](parent.Held())

		Convey("One run is one child, and the parent then advances", func() {
			So(tests.CollectSeq(op.Next(nil)), ShouldResemble, []float64{1, 2, 3})
			So(tests.CollectSeq(op.Next(nil)), ShouldResemble, []float64{4, 5})
		})

		Convey("The parent loops rather than ending", func() {
			tests.CollectSeq(op.Next(nil))
			tests.CollectSeq(op.Next(nil))
			So(tests.CollectSeq(op.Next(nil)), ShouldResemble, []float64{1, 2, 3})
		})
	})
}
