package associative

import (
	"iter"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

/*
Agent owns one learner: the regions it is shown become the sequence it
recognises, and what it recognises is written into the cognition engine that
is its memory.
*/
type Agent struct {
	core.Base[cognition.Association, *iradix.Tree[[]byte]]
	engine  *cognition.Engine
	context *Context
}

/*
NewAgent owns an engine of its own, or continues one it is given.

A composition declares its agent before it has a memory to hand it, and an
agent with nothing behind it is a learner that has not learned yet — which is
where every learner starts.
*/
func NewAgent(engine ...*cognition.Engine) *Agent {
	held := cognition.NewEngine(cognition.DefaultConfig())

	if len(engine) > 0 && engine[0] != nil {
		held = engine[0]
	}

	return &Agent{engine: held, context: NewContext()}
}

/* Engine is the trie this agent writes and reads. */
func (op *Agent) Engine() *cognition.Engine { return op.engine }

/* Tree is the current immutable snapshot of what has been learned. */
func (op *Agent) Tree() *iradix.Tree[[]byte] { return op.engine.Root() }

func (op *Agent) Next(
	in iter.Seq[core.Primitive[cognition.Association, cognition.Association]],
) iter.Seq[core.Primitive[*iradix.Tree[[]byte], *iradix.Tree[[]byte]]] {
	return func(yield func(core.Primitive[*iradix.Tree[[]byte], *iradix.Tree[[]byte]]) bool) {
		for arriving := range in {
			tree, err := op.LearnAssociation(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(tree)) {
				return
			}
		}
	}
}

func (op *Agent) LearnAssociation(assoc cognition.Association) (*iradix.Tree[[]byte], error) {
	if len(assoc.Context) == 0 {
		return op.engine.Root(), nil
	}

	if assoc.Graded {
		op.engine.Observe(assoc.Context, assoc.Class, assoc.Feedback)
	}

	if !assoc.Graded {
		op.engine.Observe(assoc.Context, assoc.Class)
	}

	return op.engine.Root(), nil
}

func (op *Agent) Learn(impulse grid.Impulse) (*iradix.Tree[[]byte], error) {
	return op.LearnAssociation(op.context.Encode(impulse))
}

func (op *Agent) Recall(evaluation cognition.Evaluation) (cognition.Evaluation, error) {
	return op.engine.Evaluate(evaluation.Context), nil
}

/*
Recall reads what the agent associates with the sequence it is being shown,
without changing anything.
*/
type Recall struct {
	core.Base[cognition.Evaluation, cognition.Evaluation]
	engine *cognition.Engine
}

func NewRecall(engine ...*cognition.Engine) *Recall {
	held := cognition.NewEngine(cognition.DefaultConfig())

	if len(engine) > 0 && engine[0] != nil {
		held = engine[0]
	}

	return &Recall{engine: held}
}

func (op *Recall) Recall(
	evaluation cognition.Evaluation,
) (cognition.Evaluation, error) {
	return op.engine.Evaluate(evaluation.Context), nil
}

func (op *Recall) Next(
	in iter.Seq[core.Primitive[cognition.Evaluation, cognition.Evaluation]],
) iter.Seq[core.Primitive[cognition.Evaluation, cognition.Evaluation]] {
	return func(yield func(core.Primitive[cognition.Evaluation, cognition.Evaluation]) bool) {
		for arriving := range in {
			reading, err := op.Recall(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}
