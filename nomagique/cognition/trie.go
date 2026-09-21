package cognition

import (
	"encoding/binary"
	"sync"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
)

/*
Trie is the learned structure the cognition nodes share: the sequences that
have been observed, and how strongly each has been reinforced.

A sequence is keyed by the basin it was observed in, so what a node walks is
the record of what actually happened rather than a summary of it. One trie is
owned here and handed to the nodes that read and write it, because a learner
whose writer and reader hold different structures learns nothing.
*/
type Trie struct {
	root  atomic.Pointer[iradix.Tree[[]byte]]
	steps atomic.Uint64
	mutex sync.Mutex
}

func NewTrie() *Trie {
	trie := &Trie{}
	trie.root.Store(iradix.New[[]byte]())

	return trie
}

/*
Root exposes the structure for readers, which observe it without locking
because the tree is immutable and replaced atomically.
*/
func (trie *Trie) Root() *atomic.Pointer[iradix.Tree[[]byte]] {
	return &trie.root
}

/*
Steps counts the observations reinforced into the trie.
*/
func (trie *Trie) Steps() *atomic.Uint64 {
	return &trie.steps
}

/*
Reinforce records one observation of a sequence, strengthening it by one and
stamping the step it was last seen at. Writers are serialized against each
other so a concurrent reinforcement cannot lose a count; readers never wait.
*/
func (trie *Trie) Reinforce(key []byte) (weight, lastSeen uint64) {
	trie.mutex.Lock()
	defer trie.mutex.Unlock()

	step := trie.steps.Add(1)
	current := trie.root.Load()

	weight = 1

	if existing, found := current.Get(key); found && len(existing) >= 16 {
		weight = binary.BigEndian.Uint64(existing[:8]) + 1
	}

	record := make([]byte, 16)
	binary.BigEndian.PutUint64(record[:8], weight)
	binary.BigEndian.PutUint64(record[8:], step)

	updated, _, _ := current.Insert(key, record)
	trie.root.Store(updated)

	return weight, step
}

/*
Weight reports how strongly a sequence has been reinforced, and whether it
has been observed at all.
*/
func (trie *Trie) Weight(key []byte) (weight uint64, found bool) {
	record, found := trie.root.Load().Get(key)

	if !found || len(record) < 16 {
		return 0, false
	}

	return binary.BigEndian.Uint64(record[:8]), true
}

/*
WeightOf reads a reinforcement record, which readers walking the trie hold as
raw bytes.
*/
func WeightOf(record []byte) uint64 {
	if len(record) < 16 {
		return 0
	}

	return binary.BigEndian.Uint64(record[:8])
}
