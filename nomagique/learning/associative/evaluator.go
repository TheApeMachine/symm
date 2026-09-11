package associative

import (
	"iter"

	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/errnie"
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
func Evaluator(engine ...*cognition.Engine) *EvaluatorOp {
	return &EvaluatorOp{recall: NewRecall(engine...), context: NewContext()}
}

/*
Evaluate judges what the agent answered against what the record named,
for a single arriving impulse.
*/
func (op *EvaluatorOp) Evaluate(impulse grid.Impulse) (cognition.Association, cognition.Evaluation, error) {
	assoc := op.context.Encode(impulse)

	if len(assoc.Context) == 0 || len(assoc.Class) == 0 {
		return assoc, cognition.Evaluation{}, nil
	}
	reading, err := op.recall.Recall(cognition.Evaluation{
		Context: assoc.Context,
		Config:  cognition.DefaultConfig(),
		Step:    impulse.Version,
	})

	if err != nil {
		return assoc, cognition.Evaluation{}, errnie.Error(errnie.Err(
			errnie.Internal,
			"evaluator: recall evaluation failed",
			err,
		))
	}

	if reading.WinnerClass != "" {
		if reading.WinnerClass == string(assoc.Class) {
			assoc.Graded = true
			assoc.Feedback = 1.0

			if impulse.Graded && impulse.Grade > 0 {
				assoc.Feedback = impulse.Grade
			}
		}

		if reading.WinnerClass != string(assoc.Class) {
			assoc.Graded = true
			assoc.Feedback = 0.5

			if op.recall != nil && op.recall.engine != nil {
				op.recall.engine.Observe(assoc.Context, []byte(reading.WinnerClass), -reading.Confidence)
			}
		}
	}

	return assoc, reading, nil
}

func (op *EvaluatorOp) Next(
	in iter.Seq[core.Primitive[grid.Impulse, grid.Impulse]],
) iter.Seq[core.Primitive[cognition.Association, cognition.Association]] {
	return func(
		yield func(core.Primitive[cognition.Association, cognition.Association]) bool,
	) {
		for arriving := range in {
			assoc, _, err := op.Evaluate(arriving.Read())

			if err != nil {
				op.Error(err)

				return
			}

			if !yield(op.Carrier(assoc)) {
				return
			}
		}
	}
}
