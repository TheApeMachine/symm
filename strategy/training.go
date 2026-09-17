package strategy

import (
	"context"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
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
Training reads the impulse map, discovers spatial attractor basins, streams temporal
transitions, reinforces empirical associations into the radix trie, and evaluates
prospective trajectories.
*/
type Training[T interface {
	core.Ordered[T]
	comparable
}] struct {
	*runtime.System
	grid     *store.Grid[T]
	pipeline *nomagique.Number
	wake     chan struct{}
}

func NewTraining[T interface {
	core.Ordered[T]
	comparable
}](
	ctx context.Context,
	grid *store.Grid[T],
	offramps ...core.Primitive,
) *Training[T] {
	training := &Training[T]{
		grid: grid,
		wake: make(chan struct{}, 1),
	}
	trie := cognition.NewTrie()

	primitives := []core.Primitive{
		associative.NewGrid[T](grid),
		associative.NewRegion(),
		temporal.NewTransition(),
		cognition.NewAssociate(),
		trie,
		cognition.NewEvaluator(trie),
	}

	for _, offramp := range offramps {
		if offramp != nil {
			primitives = append(primitives, transport.NewTee(offramp))
		}
	}

	training.pipeline = nomagique.NewNumber(primitives...)
	training.System = runtime.NewSystem(ctx, "training")
	return training
}

/*
Wake triggers one step through the training pipeline.
*/
func (training *Training[T]) Wake() {
	select {
	case training.wake <- struct{}{}:
	default:
	}
}

/*
Step reads the current grid state and advances the pipeline by one observation.
*/
func (training *Training[T]) Step() {
	if training.Status() != runtime.READY || training.grid == nil {
		return
	}

	query := store.NewQuery[T, any](nil, core.Read)

	for range training.Next(query.Next(nil)) {
	}
}

/*
Start launches the reactive background loop that steps whenever woken.
*/
func (training *Training[T]) Start() {
	go func() {
		for {
			select {
			case <-training.Context().Done():
				return
			case <-training.wake:
				training.Step()
			}
		}
	}()
}

/*
Next evaluates the current owner-held metric publications in sequence order.
*/
func (training *Training[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return training.pipeline.Next(in)
}
