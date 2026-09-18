package strategy

import (
	"context"
	"iter"
	"sync"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/runtime"
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
	mu       sync.Mutex
	pipeline *nomagique.Number
	gridTap  *GridTopologyCollector[T]
	trie     *cognition.Trie
}

func NewTraining[T interface {
	core.Ordered[T]
	comparable
}](
	ctx context.Context,
) *Training[T] {
	trie := cognition.NewTrie()
	gridTap := NewGridTopologyCollector[T]()

	training := &Training[T]{
		gridTap: gridTap,
		trie:    trie,
		pipeline: nomagique.NewNumber(
			associative.NewGrid[T](),
			transport.NewTee(gridTap),
			associative.NewRegion(),
			temporal.NewTransition(),
			cognition.NewAssociate(),
			trie,
			cognition.NewEvaluator(trie),
		),
	}

	training.System = runtime.NewSystem(ctx, "training")
	return training
}

/*
Next evaluates the current owner-held metric publications in sequence order.
*/
func (training *Training[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		training.mu.Lock()
		defer training.mu.Unlock()

		for out := range training.pipeline.Next(in) {
			if !yield(out) {
				return
			}
		}
	}
}

func (training *Training[T]) LatestTopology() *GridTopologySnapshot {
	return training.gridTap.Snapshot()
}
