package cognition_test

import (
	"sync/atomic"
	"testing"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
)

func TestReinforceNext(t *testing.T) {
	Convey("Reinforce updates basin and sensory records in the radix trie", t, func() {
		var root atomic.Pointer[iradix.Tree[[]byte]]
		root.Store(iradix.New[[]byte]())

		var stepCounter atomic.Uint64
		reinforcePrim := cognition.NewReinforce(&root, &stepCounter)

		assoc := cognition.Association{
			Context:  []byte("r1/r2"),
			Class:    []byte("enter"),
			Feedback: 1.0,
			Graded:   true,
		}

		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&assoc))
		}

		var results []cognition.Association
		for out := range reinforcePrim.Next(in) {
			results = append(results, *(*cognition.Association)(out))
		}

		So(reinforcePrim.Error(), ShouldBeNil)
		So(len(results), ShouldEqual, 1)
		So(stepCounter.Load(), ShouldEqual, 1)

		tree := root.Load()
		basinVal, foundBasin := tree.Get([]byte("b/r1/r2/enter"))
		So(foundBasin, ShouldBeTrue)

		weightDecoder := cognition.NewWeight()
		inB := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&basinVal))
		}
		var unpacked cognition.PackedWeight
		for out := range weightDecoder.Next(inB) {
			unpacked = *(*cognition.PackedWeight)(out)
		}
		So(unpacked.Count, ShouldEqual, 1)
		So(unpacked.Mass, ShouldEqual, 1.0)

		sensoryVal, foundSensory := tree.Get([]byte("s/r1/r2"))
		So(foundSensory, ShouldBeTrue)

		inS := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&sensoryVal))
		}
		var unpackedSensory cognition.PackedWeight
		for out := range weightDecoder.Next(inS) {
			unpackedSensory = *(*cognition.PackedWeight)(out)
		}
		So(unpackedSensory.Count, ShouldEqual, 1)
		So(unpackedSensory.Mass, ShouldEqual, 1.0)
	})
}
