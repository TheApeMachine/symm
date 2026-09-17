package cognition_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
)

func TestEvaluatorNext(t *testing.T) {
	Convey("Evaluator coordinates the atomic cognition primitives over arriving contexts", t, func() {
		trie := cognition.NewTrie()

		// Train associations:
		// ctx -> enter (positive feedback)
		// ctx -> wait (neutral / negative)
		// ctx/next -> sensory continuation
		trainAssocs := []cognition.Association{
			{Context: []byte("ctx"), Class: []byte("enter"), Feedback: 1.0, Graded: true},
			{Context: []byte("ctx"), Class: []byte("wait"), Feedback: 0.2, Graded: true},
			{Context: []byte("ctx/next"), Class: nil},
		}

		inTrain := func(yield func(unsafe.Pointer) bool) {
			for i := range trainAssocs {
				yield(unsafe.Pointer(&trainAssocs[i]))
			}
		}
		for range trie.Next(inTrain) {
		}

		evaluator := cognition.NewEvaluator(trie)

		ctx := []byte("ctx")
		inEval := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&ctx))
		}

		var evals []cognition.Evaluation
		for out := range evaluator.Next(inEval) {
			evals = append(evals, *(*cognition.Evaluation)(out))
		}

		So(evaluator.Error(), ShouldBeNil)
		So(len(evals), ShouldEqual, 1)

		eval := evals[0]
		So(eval.WinnerClass, ShouldEqual, "enter")
		So(eval.RunnerUp, ShouldEqual, "wait")
		So(eval.Confidence, ShouldBeGreaterThan, 0.5)
		So(eval.Surprisal, ShouldBeGreaterThan, 0)
		So(eval.Ambiguity, ShouldBeGreaterThanOrEqualTo, 0)
		So(len(eval.Lookahead), ShouldBeGreaterThanOrEqualTo, 1)
		So(eval.Lookahead[0].Sequence, ShouldEqual, "ctx/next")
	})
}
