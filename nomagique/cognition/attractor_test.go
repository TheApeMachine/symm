package cognition_test

import (
	"sync/atomic"
	"testing"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
)

func TestAttractorAndClassificationNext(t *testing.T) {
	Convey("Attractor matches basin records and Classification produces winner and contrast", t, func() {
		tree := iradix.New[[]byte]()
		txn := tree.Txn()

		pack := cognition.NewPack()
		pwEnter := cognition.PackedWeight{Count: 20, Mass: 16.0, WriteStep: 1}
		pwWait := cognition.PackedWeight{Count: 5, Mass: 4.0, WriteStep: 1}

		var valEnter, valWait [cognition.WeightSize]byte
		inEnter := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&pwEnter))
		}
		for out := range pack.Next(inEnter) {
			valEnter = *(*[cognition.WeightSize]byte)(out)
		}

		inWait := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&pwWait))
		}
		for out := range pack.Next(inWait) {
			valWait = *(*[cognition.WeightSize]byte)(out)
		}

		txn.Insert([]byte("b/r1/enter"), valEnter[:])
		txn.Insert([]byte("b/r1/wait"), valWait[:])
		tree = txn.Commit()

		var root atomic.Pointer[iradix.Tree[[]byte]]
		root.Store(tree)

		attractor := cognition.NewAttractor(&root)
		classification := cognition.NewClassification()

		ctx := []byte("r1")
		inCtx := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&ctx))
		}

		var candidates []cognition.ClassCandidate
		for out := range attractor.Next(inCtx) {
			candidates = append(candidates, *(*cognition.ClassCandidate)(out))
		}

		So(attractor.Error(), ShouldBeNil)
		So(len(candidates), ShouldEqual, 2)

		inCand := func(yield func(unsafe.Pointer) bool) {
			for i := range candidates {
				yield(unsafe.Pointer(&candidates[i]))
			}
		}

		var result cognition.ClassificationResult
		for out := range classification.Next(inCand) {
			result = *(*cognition.ClassificationResult)(out)
		}

		So(classification.Error(), ShouldBeNil)
		So(result.WinnerClass, ShouldEqual, "enter")
		So(result.RunnerUp, ShouldEqual, "wait")
		So(result.Confidence, ShouldBeGreaterThan, 0.5)
		So(result.Contrast, ShouldBeGreaterThan, 0)
	})
}
