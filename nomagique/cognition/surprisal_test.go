package cognition_test

import (
	"sync/atomic"
	"testing"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
)

func TestSurprisalNext(t *testing.T) {
	Convey("Surprisal computes transition information bits directly from observed frequency", t, func() {
		tree := iradix.New[[]byte]()
		txn := tree.Txn()

		pack := cognition.NewPack()
		pw := cognition.PackedWeight{Count: 1, Mass: 1.0, WriteStep: 1}
		var val [cognition.WeightSize]byte
		inPack := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&pw))
		}
		for out := range pack.Next(inPack) {
			val = *(*[cognition.WeightSize]byte)(out)
		}

		txn.Insert([]byte("s/ctx"), val[:])
		tree = txn.Commit()

		var root atomic.Pointer[iradix.Tree[[]byte]]
		root.Store(tree)

		var stepCounter atomic.Uint64
		stepCounter.Store(10)

		surprisal := cognition.NewSurprisal(&root, &stepCounter)

		ctx := []byte("ctx")
		inCtx := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&ctx))
		}

		var surpBits float64
		for out := range surprisal.Next(inCtx) {
			surpBits = *(*float64)(out)
		}

		So(surprisal.Error(), ShouldBeNil)
		So(surpBits, ShouldBeGreaterThan, 0)
	})
}
