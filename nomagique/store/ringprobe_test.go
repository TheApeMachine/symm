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

		op := NewRing[float64](parent)

		So(tests.CollectSeq(op.Next(nil)), ShouldResemble, []float64{1, 2})
		So(tests.CollectSeq(op.Next(nil)), ShouldResemble, []float64{3, 4})
		So(tests.CollectSeq(op.Next(nil)), ShouldResemble, []float64{1, 2})
	})
}
