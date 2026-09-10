package associative

import (
	"iter"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Agent owns one learner: the regions it is shown become the sequence it
recognises, and what it recognises is written into a memory that is its own.
*/
type Agent struct {
	core.Base[grid.Impulse, *iradix.Tree[[]byte]]
	memory  *store.Retained[*iradix.Tree[[]byte]]
	context *Context
	observe *cognition.Observe
	recall  *cognition.Evaluate
}

func NewAgent(memory *store.Retained[*iradix.Tree[[]byte]]) *Agent {
	if memory == nil {
		memory = NewMemory()
	}

	return &Agent{
		memory:  memory,
		context: NewContext(),
		observe: cognition.NewObserve(),
		recall:  cognition.NewEvaluate(),
	}
}

func NewMemory() *store.Retained[*iradix.Tree[[]byte]] {
	return store.NewRetained(iradix.New[[]byte]())
}

func (op *Agent) Next(
	in iter.Seq[core.Primitive[grid.Impulse, grid.Impulse]],
) iter.Seq[core.Primitive[*iradix.Tree[[]byte], *iradix.Tree[[]byte]]] {
	return func(yield func(core.Primitive[*iradix.Tree[[]byte], *iradix.Tree[[]byte]]) bool) {
		for arriving := range in {
			tree, err := op.Learn(arriving.Read())

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

func (op *Agent) Learn(impulse grid.Impulse) (*iradix.Tree[[]byte], error) {
	assoc := op.context.Encode(impulse)
	write, err := op.observe.Record(cognition.ObserveInput{
		Tree:        op.memory.Read(),
		Association: assoc,
	})

	if err != nil {
		return nil, err
	}

	tree, err := transport.Evaluate(store.NewRadix(op.memory.Read()), transport.Values(op.observe.Map(write)))

	if err != nil {
		return nil, err
	}

	op.memory.Carrier(tree)
	return tree, nil
}

func (op *Agent) Recall(evaluation cognition.Evaluation) (cognition.Evaluation, error) {
	return op.recall.Recall(cognition.EvaluateInput{
		Tree:       op.memory.Read(),
		Evaluation: evaluation,
	})
}

/*
Recall reads what the agent associates with the sequence it is being shown,
without changing anything.
*/
type Recall struct {
	core.Base[cognition.Evaluation, cognition.Evaluation]
	memory   *store.Retained[*iradix.Tree[[]byte]]
	evaluate *cognition.Evaluate
}

func NewRecall(memory *store.Retained[*iradix.Tree[[]byte]]) *Recall {
	if memory == nil {
		memory = NewMemory()
	}

	return &Recall{memory: memory, evaluate: cognition.NewEvaluate()}
}

func (op *Recall) Next(
	in iter.Seq[core.Primitive[cognition.Evaluation, cognition.Evaluation]],
) iter.Seq[core.Primitive[cognition.Evaluation, cognition.Evaluation]] {
	return func(yield func(core.Primitive[cognition.Evaluation, cognition.Evaluation]) bool) {
		for arriving := range in {
			reading, err := op.evaluate.Recall(cognition.EvaluateInput{
				Tree:       op.memory.Read(),
				Evaluation: arriving.Read(),
			})

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
