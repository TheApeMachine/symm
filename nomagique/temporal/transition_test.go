package temporal_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
)

func TestTransitionNext(t *testing.T) {
	Convey("A -> B emits one transition", t, func() {
		transition := temporal.NewTransition()

		signatureA := []byte("0,0;1,0")
		signatureB := []byte("1,0;2,0")

		in := func(yield func(unsafe.Pointer) bool) {
			if !yield(unsafe.Pointer(&signatureA)) {
				return
			}

			yield(unsafe.Pointer(&signatureB))
		}

		var emissions []string
		for out := range transition.Next(in) {
			emissions = append(emissions, string(*(*[]byte)(out)))
		}

		So(transition.Error(), ShouldBeNil)
		So(len(emissions), ShouldEqual, 1)
		So(emissions[0], ShouldEqual, "0,0;1,0->1,0;2,0")
	})

	Convey("sequential observations stream chain of transitions", t, func() {
		transition := temporal.NewTransition()

		sigOne := []byte("A")
		sigTwo := []byte("B")
		sigThree := []byte("C")

		in := func(yield func(unsafe.Pointer) bool) {
			if !yield(unsafe.Pointer(&sigOne)) {
				return
			}

			if !yield(unsafe.Pointer(&sigTwo)) {
				return
			}

			yield(unsafe.Pointer(&sigThree))
		}

		var emissions []string
		for out := range transition.Next(in) {
			emissions = append(emissions, string(*(*[]byte)(out)))
		}

		So(transition.Error(), ShouldBeNil)
		So(len(emissions), ShouldEqual, 2)
		So(emissions[0], ShouldEqual, "A->B")
		So(emissions[1], ShouldEqual, "B->C")
	})
}
