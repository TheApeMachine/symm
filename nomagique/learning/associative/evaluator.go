package associative

import (
	"iter"

	iradix "github.com/hashicorp/go-immutable-radix/v2"

	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
Evaluator judges what the agent answered against what the record named.

An impulse carries both: the regions the tape lit up, and the moment the tape
itself says that observation was. So the question and its answer are already in
hand — the agent is asked what it recognises before anything is written, and
what it is taught is graded by whether it named the moment the record did.

That grade is the whole difference between storing and learning. A learner
written to unconditionally strengthens whatever it saw most; one written to on
its own answer strengthens what it got right and stops reinforcing what it did
not, which is what makes the sequence it holds a precursor rather than a
frequency count.
*/
type EvaluatorOp struct {
	core.Base[grid.Impulse, cognition.Association]
	recall  *Recall
	context *Context
}

/*
Evaluator judges against the memory it is given, which is the same one the
agent it follows is writing. Given none it judges against its own, which is a
learner being taught on what it can already recognise of itself.

Named as the type is because that is what it reads as in a composition: the
stage is an Evaluator, not a call that makes one.
*/
func Evaluator(memory ...*store.Retained[*iradix.Tree[[]byte]]) *EvaluatorOp {
	return &EvaluatorOp{recall: NewRecall(memory...), context: NewContext()}
}

func (op *EvaluatorOp) Next(
	in iter.Seq[core.Primitive[grid.Impulse, grid.Impulse]],
) iter.Seq[core.Primitive[cognition.Association, cognition.Association]] {
	return func(
		yield func(core.Primitive[cognition.Association, cognition.Association]) bool,
	) {
		for arriving := range in {
			impulse := arriving.Read()
			assoc := op.context.Encode(impulse)

			// A situation the tape did not name is one there is no answer to
			// judge. It is carried through as it stands rather than graded
			// against a moment nobody stated.
			if len(assoc.Context) == 0 || len(assoc.Class) == 0 {
				if !yield(op.Carrier(assoc)) {
					return
				}

				continue
			}
			reading, err := op.recall.Recall(cognition.Evaluation{
				Context: assoc.Context,
				Config:  cognition.DefaultConfig(),
				Step:    impulse.Version,
			})

			if err != nil {
				op.Error(err)

				return
			}

			/*
				What the agent said, against what the record said. A learner
				with nothing to say yet is not wrong — it is untaught, and the
				observation teaches it on having been seen. Once it does have
				an answer, agreeing is what earns reinforcement.
			*/
			if reading.WinnerClass != "" {
				assoc.Graded = true
				assoc.Feedback = 0

				if reading.WinnerClass == string(assoc.Class) {
					assoc.Feedback = reading.Confidence
				}
			}

			if !yield(op.Carrier(assoc)) {
				return
			}
		}
	}
}
