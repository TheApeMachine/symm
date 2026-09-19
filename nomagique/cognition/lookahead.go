package cognition

import (
	"bytes"
	"iter"
	"math"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Lookahead explores forward continuation paths in the sensory tree.
*/
type Lookahead types.Value[[]byte, iter.Seq2[[]byte, float64]]

/*
NewLookahead creates a parameterless Value closure that explores forward continuation paths in the default sensory tree.
*/
func NewLookahead() Lookahead {
	return NewLookaheadWithMemory(defaultMemoryRoot)
}

/*
NewLookaheadWithMemory creates a Value closure that explores forward continuation paths in the specified sensory tree.
*/
func NewLookaheadWithMemory(
	root *atomic.Pointer[iradix.Tree[[]byte]],
) Lookahead {
	weightDecoder := NewWeight()

	return func(prefix []byte) iter.Seq2[[]byte, float64] {
		return func(yield func([]byte, float64) bool) {
			if root == nil || len(prefix) == 0 {
				return
			}

			tree := root.Load()
			if tree == nil {
				return
			}

			searchPrefix := append([]byte("s/"), prefix...)
			it := tree.Root().Iterator()
			it.SeekPrefix(searchPrefix)

			for k, v, ok := it.Next(); ok; k, v, ok = it.Next() {
				if !bytes.HasPrefix(k, searchPrefix) || len(v) < 24 {
					break
				}

				seq := k[len("s/"):]
				if len(seq) <= len(prefix) {
					continue
				}

				pw := weightDecoder(v)
				if pw == [3]uint64{} || pw[0] == 0 {
					continue
				}
				mass := math.Float64frombits(pw[1])
				if mass <= 0 {
					continue
				}

				logP := math.Log(mass)
				
				if !yield(seq, logP) {
					return
				}
			}
		}
	}
}
