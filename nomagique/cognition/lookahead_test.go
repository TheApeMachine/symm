package cognition_test

import (
	"sync/atomic"
	"testing"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
)

func TestLookaheadNext(t *testing.T) {
	Convey("Lookahead explores future branch trajectories along empirical paths", t, func() {
		tree := iradix.New[[]byte]()
		txn := tree.Txn()

		pack := cognition.NewPack()
		w1 := cognition.PackedWeight{Count: 10, Mass: 0.8, WriteStep: 1}
		w2 := cognition.PackedWeight{Count: 2, Mass: 0.2, WriteStep: 1}

		in1 := func(yield func(unsafe.Pointer) bool) { yield(unsafe.Pointer(&w1)) }
		var val1 [cognition.WeightSize]byte
		for out := range pack.Next(in1) {
			val1 = *(*[cognition.WeightSize]byte)(out)
		}

		in2 := func(yield func(unsafe.Pointer) bool) { yield(unsafe.Pointer(&w2)) }
		var val2 [cognition.WeightSize]byte
		for out := range pack.Next(in2) {
			val2 = *(*[cognition.WeightSize]byte)(out)
		}

		txn.Insert([]byte("s/ctx/r1"), val1[:])
		txn.Insert([]byte("s/ctx/r2"), val2[:])
		tree = txn.Commit()

		var root atomic.Pointer[iradix.Tree[[]byte]]
		root.Store(tree)

		lookahead := cognition.NewLookahead(&root)

		ctx := []byte("ctx")
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&ctx))
		}

		var paths []cognition.LookaheadPath
		for out := range lookahead.Next(in) {
			paths = append(paths, *(*cognition.LookaheadPath)(out))
		}

		So(lookahead.Error(), ShouldBeNil)
		So(len(paths), ShouldEqual, 2)
		So(paths[0].Sequence, ShouldEqual, "ctx/r1")
		So(paths[1].Sequence, ShouldEqual, "ctx/r2")
		So(paths[0].Score, ShouldBeGreaterThan, paths[1].Score)
	})
}
