package cognition

import (
	"bytes"
	"math"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Attractor seeks matching attractor basin records in the radix trie.
*/
type Attractor types.Value[[]byte, func(func([]byte, float64, uint64) bool)]

/*
NewAttractor creates a parameterless Value closure that seeks matching attractor basin records
in default memory.
*/
func NewAttractor() Attractor {
	return NewAttractorWithMemory(defaultMemoryRoot)
}

/*
NewAttractorWithMemory creates a Value closure that seeks matching attractor basin records
in the specified radix trie under b/<context>/.
*/
func NewAttractorWithMemory(
	root *atomic.Pointer[iradix.Tree[[]byte]],
) Attractor {
	weightDecoder := NewWeight()

	return func(context []byte) func(func([]byte, float64, uint64) bool) {
		return func(yield func([]byte, float64, uint64) bool) {
			if root == nil || len(context) == 0 {
				return
			}

			tree := root.Load()
			if tree == nil {
				return
			}

			exactPrefix := make([]byte, 2+len(context)+1)
			exactPrefix[0] = 'b'
			exactPrefix[1] = '/'
			copy(exactPrefix[2:], context)
			exactPrefix[2+len(context)] = '/'

			it := tree.Root().Iterator()
			it.SeekPrefix(exactPrefix)

			for k, v, ok := it.Next(); ok; k, v, ok = it.Next() {
				if !bytes.HasPrefix(k, exactPrefix) || len(v) < 24 {
					break
				}

				class := k[len(exactPrefix):]
				if len(class) == 0 {
					continue
				}

				pw := weightDecoder(v)
				
				if pw == [3]uint64{} {
					continue
				}

				if !yield(class, math.Float64frombits(pw[1]), pw[0]) {
					return
				}
			}
		}
	}
}
