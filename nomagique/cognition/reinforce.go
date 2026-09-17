package cognition

import (
	"bytes"
	"iter"
	"math"
	"sync/atomic"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Association is one observed precursor and the class that followed it.
Without a grade it observes a positive association. A graded association
carries a dimensionless reinforcement amount: positive strengthens, negative
inhibits, and zero only observes sensory context.
*/
type Association struct {
	Context  []byte
	Class    []byte
	Feedback float64
	Graded   bool
}

/*
Reinforce updates basin and sensory transition records in the radix trie.
It advances the monotonic clock and incorporates empirical counts and graded
feedback via atomic compare-and-swap on the immutable radix tree.
*/
type Reinforce struct {
	*core.PrimitiveError
	root        *atomic.Pointer[iradix.Tree[[]byte]]
	stepCounter *atomic.Uint64
}

func NewReinforce(
	root *atomic.Pointer[iradix.Tree[[]byte]],
	stepCounter *atomic.Uint64,
) *Reinforce {
	return &Reinforce{
		PrimitiveError: core.NewPrimitiveError(),
		root:           root,
		stepCounter:    stepCounter,
	}
}

func (reinforcePrim *Reinforce) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if reinforcePrim.Error() != nil || reinforcePrim.root == nil {
			return
		}

		weightDecoder := NewWeight()
		packEncoder := NewPack()
		var out Association

		for arriving := range in {
			if arriving == nil {
				continue
			}

			assoc := *(*Association)(arriving)
			if len(assoc.Context) == 0 {
				continue
			}

			basinKey := makeBasinKey(assoc.Class, assoc.Context)
			sensoryKey := makeSensoryKey(assoc.Context)

			for {
				oldRoot := reinforcePrim.root.Load()
				var step uint64
				if reinforcePrim.stepCounter != nil {
					step = reinforcePrim.stepCounter.Load() + 1
				}

				txn := oldRoot.Txn()

				if len(assoc.Class) > 0 && (!assoc.Graded || assoc.Feedback != 0) {
					pw := PackedWeight{
						Count:     1,
						Mass:      1.0,
						WriteStep: step,
					}

					if assoc.Graded {
						pw.Mass = math.Max(assoc.Feedback, 0)
					}

					if existing, found := oldRoot.Get(basinKey); found && len(existing) >= WeightSize {
						inW := func(yieldW func(unsafe.Pointer) bool) {
							yieldW(unsafe.Pointer(&existing))
						}
						for outW := range weightDecoder.Next(inW) {
							pw = *(*PackedWeight)(outW)
						}

						pw.Count++
						if assoc.Graded {
							pw.Mass = math.Max(pw.Mass+assoc.Feedback, 0)
						}

						if !assoc.Graded {
							pw.Mass += core.Unit
						}

						pw.WriteStep = step
					}

					var packed [WeightSize]byte
					inPack := func(yieldP func(unsafe.Pointer) bool) {
						yieldP(unsafe.Pointer(&pw))
					}
					for outP := range packEncoder.Next(inPack) {
						packed = *(*[WeightSize]byte)(outP)
					}
					txn.Insert(basinKey, bytes.Clone(packed[:]))
				}

				sPW := PackedWeight{
					Count:     1,
					Mass:      core.Unit,
					WriteStep: step,
				}

				if existing, found := oldRoot.Get(sensoryKey); found && len(existing) >= WeightSize {
					inW := func(yieldW func(unsafe.Pointer) bool) {
						yieldW(unsafe.Pointer(&existing))
					}
					for outW := range weightDecoder.Next(inW) {
						sPW = *(*PackedWeight)(outW)
					}

					sPW.Count++
					sPW.Mass += core.Unit
					sPW.WriteStep = step
				}

				var sPacked [WeightSize]byte
				inPack := func(yieldP func(unsafe.Pointer) bool) {
					yieldP(unsafe.Pointer(&sPW))
				}
				for outP := range packEncoder.Next(inPack) {
					sPacked = *(*[WeightSize]byte)(outP)
				}
				txn.Insert(sensoryKey, bytes.Clone(sPacked[:]))

				newRoot := txn.Commit()
				if reinforcePrim.root.CompareAndSwap(oldRoot, newRoot) {
					if reinforcePrim.stepCounter != nil {
						reinforcePrim.stepCounter.Add(1)
					}
					break
				}
			}

			out = assoc
			if !yield(unsafe.Pointer(&out)) {
				return
			}
		}
	}
}
