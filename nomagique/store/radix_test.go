package store_test

import (
	"testing"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* ask is one question put to a store: a selector, and data when writing. */
func ask(fields map[string][]byte) core.Primitive {
	return store.NewQuery[map[string][]byte](transport.NewIO(core.From(fields)))
}

/*
Retention is a composition, exactly as it is for a cumulative sum: the store
reads its state and hands back the next one, and the Pipe decides it is kept.
*/
func TestRadixRetainsThroughComposition(t *testing.T) {
	retained := store.NewRetained(core.From(iradix.New[[]byte]()))
	memory := transport.NewPipe(
		store.NewRadix[iradix.Tree[any]](retained), retained,
	)

	tests.Drain(t, memory, ask(map[string][]byte{
		"selector": []byte("b/enter\x00abc"), "data": []byte("one"),
	}))
	tests.Sound(t, memory)

	tests.Drain(t, memory, ask(map[string][]byte{
		"selector": []byte("b/exit\x00abc"), "data": []byte("two"),
	}))
	tests.Sound(t, memory)

	// Selecting alone is a question: the store hands itself back, and whoever
	// asked walks the opening inside their own fold.
	answered := tests.Drain(t, memory, ask(map[string][]byte{
		"selector": []byte("b/"),
	}))
	tests.Sound(t, memory)

	if len(answered) == 0 {
		t.Fatal("expected the store to answer")
	}
	tree, held := answered[len(answered)-1].(*iradix.Tree[[]byte])

	if !held {
		t.Fatalf("expected a store, received %T", answered[len(answered)-1])
	}
	found := map[string]string{}
	iterator := tree.Root().Iterator()
	iterator.SeekPrefix([]byte("b/"))

	for key, value, more := iterator.Next(); more; key, value, more = iterator.Next() {
		found[string(key)] = string(value)
	}

	if len(found) != 2 {
		t.Fatalf("expected both writes to have survived, received %v", found)
	}

	if found["b/enter\x00abc"] != "one" || found["b/exit\x00abc"] != "two" {
		t.Fatalf("expected what was written, received %v", found)
	}
}
