package store_test

import (
	container "container/ring"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestRingNext(t *testing.T) {
	Convey("Ring plays one child sequence per Play command and advances the parent", t, func() {
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

		op := store.NewRingOver[float64](parent)
		play := func() []float64 {
			command := store.RingCommand[float64]{Play: true}

			return tests.CollectSeq[float64](op.Next(transport.NewOne(unsafe.Pointer(&command)).Next(nil)))
		}

		So(play(), ShouldResemble, []float64{1, 2})
		So(play(), ShouldResemble, []float64{3, 4})
		So(play(), ShouldResemble, []float64{1, 2})
	})
}

func TestRingWrite(t *testing.T) {
	Convey("Ring writes values and nested child rings through commands", t, func() {
		parent := store.NewRing[[]float64]()
		child := store.NewRing[[]float64]()

		for _, frame := range [][]float64{{1, 2}, {3, 4}} {
			held := frame
			command := store.RingCommand[[]float64]{Write: &store.RingValue[[]float64]{Value: &held}}

			for range child.Next(transport.NewOne(unsafe.Pointer(&command)).Next(nil)) {
			}
		}

		slot := store.RingSlot[[]float64]{Value: store.RingValue[[]float64]{Child: child}}
		nest := store.RingCommand[[]float64]{WriteAt: &slot}

		for range parent.Next(transport.NewOne(unsafe.Pointer(&nest)).Next(nil)) {
		}

		So(parent.Error(), ShouldBeNil)
		So(child.Error(), ShouldBeNil)

		play := store.RingCommand[[]float64]{Play: true}
		out := tests.CollectSeq[[]float64](parent.Next(transport.NewOne(unsafe.Pointer(&play)).Next(nil)))

		So(out, ShouldResemble, [][]float64{{1, 2}, {3, 4}})
		So(parent.Error(), ShouldBeNil)
	})
}
