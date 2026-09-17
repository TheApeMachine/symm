package strategy

import (
	"context"
	"iter"
	"sync/atomic"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type Action string

const (
	ActionEnter Action = "enter"
	ActionExit  Action = "exit"
	ActionWait  Action = "wait"
)

func LegalActions(holding bool) []Action {
	if !holding {
		return []Action{ActionEnter, ActionWait}
	}

	return []Action{ActionExit, ActionWait}
}

/*
Training reads the impulse map and addresses one shared radix trie. The inference
pipeline compares the observed action counts. Ties select wait;
unseen contexts produce no action. The cold learner owns trie writes.
Training does not submit orders: quoted outcome evidence is not execution proof.
*/
type Training[T core.Ordered[T]] struct {
	*runtime.System
	root        atomic.Pointer[iradix.Tree[[]byte]]
	stepCounter atomic.Uint64
	pipeline    *nomagique.Number
	Reinforce   *cognition.Reinforce
}

func NewTraining[T core.Ordered[T]](
	ctx context.Context,
	price *broker.Price,
	members ...map[string]core.Identifiable[T],
) *Training[T] {
	training := &Training[T]{}
	training.root.Store(iradix.New[[]byte]())

	training.Reinforce = cognition.NewReinforce(&training.root, &training.stepCounter)

	training.pipeline = nomagique.NewNumber(
		associative.NewGrid(members...),
		associative.NewRegion(),
		cognition.NewEvaluator(&training.root, &training.stepCounter),
	)

	training.System = runtime.NewSystem(ctx, "training")
	return training
}

/*
Next evaluates the current owner-held metric publications in sequence order.
*/
func (training *Training[T]) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return training.pipeline.Next(in)
}
