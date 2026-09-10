package associative

import (
	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
NewAgent composes one learner: the regions it is shown become the sequence it
recognises, and what it recognises is written into a memory that is its own.

Each agent owns its store because what one learned about a precursor is its own
memory and not a register the others write through. Two agents shown the same
tape reach their own readings of it, and that difference is the point.

Retention is the composition, not the store: the memory is read, the link the
observation names is moved, and the Pipe decides the result is kept.
*/
func NewAgent(memory core.Primitive) core.Primitive {
	intent := store.NewRetained(core.From(map[string][]byte{}))
	observed := store.NewRetained(core.From(map[string][]byte{}))

	// One observation at a time. A fold would collapse a whole fragment into
	// its last frame, so an agent would be taught only what the tape happened
	// to end on and never what ran into anything.
	return transport.NewMap(transport.NewPipe(
		// What lit up becomes the sequence, carrying whatever the agent was
		// told followed it and how that was graded.
		NewContext(transport.NewApply(intent, nil)),
		observed,
		transport.NewDiscard(),

		// The memory as it stands, moved by that observation, and kept.
		transport.NewApply(memory, nil),
		cognition.NewObserve(transport.NewApply(observed, nil)),
		store.NewRadix[iradix.Tree[any]](memory),
		memory,
	))
}

/* NewMemory is one agent's own store, empty until it has recognised something. */
func NewMemory() core.Primitive {
	return store.NewRetained(core.From(iradix.New[[]byte]()))
}

/*
NewRecall composes the reading half over the same memory: what the agent
associates with the sequence it is being shown, without changing anything.
*/
func NewRecall(memory, asked core.Primitive) core.Primitive {
	return transport.NewPipe(
		transport.NewApply(memory, nil),
		cognition.NewEvaluate(transport.NewApply(asked, nil)),
	)
}
