package cognition

import (
	"bytes"
	"math"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewReinforce creates a Value closure that reinforces empirical basin (b/<context>/<class>)
and sensory (s/<context>) weights into the immutable radix tree, advancing the monotonic
step counter and yielding the active evaluation context bytes downstream.
No structs, pure Value closure.
*/
func NewReinforce(
	root *atomic.Pointer[iradix.Tree[[]byte]],
	stepCounter *atomic.Uint64,
) types.Value[[2][]byte, types.Value[float64, []byte]] {
	weightDecoder := NewWeight()
	packEncoder := NewPack()

	return func(assoc [2][]byte) types.Value[float64, []byte] {
		return func(feedback float64) []byte {
			contextBytes := assoc[0]
			classBytes := assoc[1]

			if root == nil || stepCounter == nil || len(contextBytes) == 0 {
				return nil
			}

			basinKey := makeBasinKey(classBytes, contextBytes)
			sensoryKey := makeSensoryKey(contextBytes)

			for {
				oldRoot := root.Load()

				if oldRoot == nil {
					return nil
				}

				step := stepCounter.Load() + 1
				txn := oldRoot.Txn()

				if len(classBytes) > 0 && feedback != 0 {
					pw := [3]uint64{uint64(core.Unit), math.Float64bits(feedback), step}

					if existing, found := oldRoot.Get(basinKey); found && len(existing) >= 24 {
						if decoded := weightDecoder(existing); decoded != [3]uint64{} {
							pw = decoded
						}

						pw[0]++
						currentMass := math.Float64frombits(pw[1])
						pw[1] = math.Float64bits(math.Max(currentMass+feedback, 0))
						pw[2] = step
					}

					packed := packEncoder(pw)
					txn.Insert(basinKey, bytes.Clone(packed))
				}

				sPW := [3]uint64{uint64(core.Unit), math.Float64bits(core.Unit), step}

				if existing, found := oldRoot.Get(sensoryKey); found && len(existing) >= 24 {
					if decoded := weightDecoder(existing); decoded != [3]uint64{} {
						sPW = decoded
					}

					sPW[0]++
					currentMass := math.Float64frombits(sPW[1])
					sPW[1] = math.Float64bits(currentMass + core.Unit)
					sPW[2] = step
				}

				sPacked := packEncoder(sPW)
				txn.Insert(sensoryKey, bytes.Clone(sPacked))

				newRoot := txn.Commit()

				if root.CompareAndSwap(oldRoot, newRoot) {
					stepCounter.Add(uint64(core.Unit))
					break
				}
			}

			if len(classBytes) > 0 {
				return bytes.Clone(classBytes)
			}
			return bytes.Clone(contextBytes)
		}
	}
}

/*
NewMemory creates the concurrent radix trie memory and returns its root and step counter pointers
so that independent metric closures (Attractor, Classification, Surprisal) can be constructed.
*/
func NewMemory() (*atomic.Pointer[iradix.Tree[[]byte]], *atomic.Uint64) {
	var root atomic.Pointer[iradix.Tree[[]byte]]
	root.Store(iradix.New[[]byte]())
	var stepCounter atomic.Uint64
	return &root, &stepCounter
}
