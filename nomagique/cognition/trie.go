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
Trie owns the immutable radix trie for associative cognitive learning and inference.
Arriving Association inputs reinforce empirical basin (b/<context>/<class>) and
sensory (s/<context>) weights, advance the monotonic step counter, and yield the
active evaluation context byte slice directly downstream for evaluation.
*/
type Trie struct {
	*core.PrimitiveError
	Root          atomic.Pointer[iradix.Tree[[]byte]]
	StepCounter   atomic.Uint64
	weightDecoder *Weight
	packEncoder   *Pack
	out           []byte
}

func NewTrie() *Trie {
	trie := &Trie{
		PrimitiveError: core.NewPrimitiveError(),
		weightDecoder:  NewWeight(),
		packEncoder:    NewPack(),
	}

	trie.Root.Store(iradix.New[[]byte]())
	return trie
}

func (trie *Trie) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil || trie.Error() != nil {
			return
		}

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
				oldRoot := trie.Root.Load()
				step := trie.StepCounter.Load() + 1
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
						for outW := range trie.weightDecoder.Next(inW) {
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
					for outP := range trie.packEncoder.Next(inPack) {
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
					for outW := range trie.weightDecoder.Next(inW) {
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
				for outP := range trie.packEncoder.Next(inPack) {
					sPacked = *(*[WeightSize]byte)(outP)
				}
				txn.Insert(sensoryKey, bytes.Clone(sPacked[:]))

				newRoot := txn.Commit()
				if trie.Root.CompareAndSwap(oldRoot, newRoot) {
					trie.StepCounter.Add(1)
					break
				}
			}

			if len(assoc.Class) > 0 {
				trie.out = bytes.Clone(assoc.Class)
			}

			if len(assoc.Class) == 0 {
				trie.out = bytes.Clone(assoc.Context)
			}

			if len(trie.out) > 0 {
				if !yield(unsafe.Pointer(&trie.out)) {
					return
				}
			}
		}
	}
}
