/*
Package cognition is associative memory as primitives.

Associate records one context against one class. Recall reads that class
back. Train records every contiguous token span of a context so a later
suffix can match it. Census, Snapshot, Restore, and Export read the same
memory. Text and numbers cross the boundary through a data.Adapter.
*/
package cognition

import (
	"bytes"
	"encoding/binary"
	"iter"
	"math"
	"sync"
	"sync/atomic"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

// basinFloor is the useful-strength floor for a graded basin. After negative
// feedback, a probability below this floor is pruned from the trie.
const basinFloor = core.Unit / 16

/*
Associate owns the association trie. Key 0 in that trie is the observation
clock and key 1 is the longest stored token span, written in the same
transaction as the association they describe. Basin keys are
b/<context>/<class>. Sensory keys are s/<context>. Terminal basin classes
are enter and exit only — wait is not an action and is rejected. A graded
association starts at the center of the unit interval. An ungraded
association starts at one. A graded observation whose feedback is zero
records the sensory transition and does not reinforce the basin. Negative
feedback that drives a basin below basinFloor deletes that sequence
(prune) instead of storing a useless leaf.
*/
type Associate struct {
	*core.PrimitiveError
	root      atomic.Pointer[iradix.Tree[[]byte]]
	classes   sync.Map
	reinforce *Reinforce
}

func NewAssociate() *Associate {
	memory := &Associate{
		PrimitiveError: core.NewPrimitiveError(),
		reinforce:      NewReinforce(),
	}
	memory.root.Store(iradix.New[[]byte]())

	return memory
}

func (op *Associate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil || op.reinforce == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil || op.root.Load() == nil {
				op.Error(core.ErrShape)
				return
			}

			var text data.Map[string]

			for pointer := range adapter.Next(data.NewValue(data.NewLiteral("context", "class"))) {
				text = *(*data.Map[string])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			context, contextOK := text.Values["context"]
			class, classOK := text.Values["class"]

			if !contextOK || !classOK {
				op.Error(core.ErrNotHeld)
				return
			}

			if context == "" {
				op.Error(core.ErrDomain)
				return
			}

			// Wait is precursor stance (abstention / internal prefix), never a
			// terminal basin class. Edges are region tokens; leaves are enter/exit.
			if class == "wait" {
				op.Error(core.ErrDomain)
				return
			}

			var numbers data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(data.NewMap(
				"feedback", "feedback",
				"graded", "graded",
			))) {
				numbers = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			feedback, feedbackOK := numbers.Values["feedback"]
			graded, gradedOK := numbers.Values["graded"]

			if !feedbackOK || !gradedOK {
				op.Error(core.ErrNotHeld)
				return
			}

			gradedFlag := 0.0

			if graded != 0 {
				gradedFlag = core.Unit
			}

			contextBytes := []byte(context)
			classBytes := []byte(class)
			recordBasin := len(classBytes) > 0 && (gradedFlag == 0 || feedback != 0)
			introduced := ""

			for {
				current := op.root.Load()

				if current == nil {
					op.Error(core.ErrShape)
					return
				}

				clock := uint64(0)

				if raw, held := current.Get([]byte{0}); held && len(raw) == 8 {
					clock = binary.BigEndian.Uint64(raw)
				}

				clock++
				span := uint64(1 + bytes.Count(contextBytes, []byte{'/'}))

				if raw, held := current.Get([]byte{1}); held && len(raw) == 8 {
					stored := binary.BigEndian.Uint64(raw)

					if stored > span {
						span = stored
					}
				}

				transaction := current.Txn()
				isNew := false
				prunedExisting := false

				if recordBasin {
					basin := make([]byte, 2+len(contextBytes)+1+len(classBytes))
					basin[0] = 'b'
					basin[1] = '/'
					copy(basin[2:], contextBytes)
					basin[2+len(contextBytes)] = '/'
					copy(basin[3+len(contextBytes):], classBytes)

					probability := core.Unit
					count := uint64(1)
					existing, found := current.Get(basin)

					if gradedFlag != 0 {
						probability = core.Unit / 2
					}

					if found && len(existing) == 24 {
						count = binary.LittleEndian.Uint64(existing[0:8]) + 1
						probability = math.Float64frombits(binary.LittleEndian.Uint64(existing[8:16]))
					}

					if !found {
						isNew = true
					}

					basinState := data.NewState(data.NewMap())
					basinBridge := data.NewAdapter(nil, basinState)
					basinIssued := data.NewOutputMap()
					basinIssued.Values["probability"] = probability
					basinIssued.Values["count"] = float64(count)
					basinIssued.Values["feedback"] = feedback
					basinIssued.Values["graded"] = gradedFlag

					for range basinBridge.Next(data.NewValue(basinIssued)) {
					}

					if err := basinBridge.Error(); err != nil {
						op.Error(err)
						return
					}

					for range op.reinforce.Next(data.NewValue(basinBridge)) {
					}

					if err := op.reinforce.Error(); err != nil {
						op.Error(err)
						return
					}

					var basinUpdated data.Map[float64]

					for pointer := range basinBridge.Next(data.NewValue(data.NewMap("probability", "probability"))) {
						basinUpdated = *(*data.Map[float64])(pointer)
					}

					if err := basinBridge.Error(); err != nil {
						op.Error(err)
						return
					}

					updated, held := basinUpdated.Values["probability"]

					if !held {
						op.Error(core.ErrNotHeld)
						return
					}

					// Losing / wrong sequences dampen. Once strength is useless,
					// prune the basin so a branch never keeps a dead leaf.
					pruned := gradedFlag != 0 && feedback < 0 && updated < basinFloor

					if pruned {
						if found {
							transaction.Delete(basin)
							prunedExisting = true
						}

						isNew = false
					} else {
						var packed [24]byte
						binary.LittleEndian.PutUint64(packed[0:8], count)
						binary.LittleEndian.PutUint64(packed[8:16], math.Float64bits(updated))
						binary.LittleEndian.PutUint64(packed[16:24], clock)
						transaction.Insert(basin, bytes.Clone(packed[:]))
					}
				}

				sensory := make([]byte, 2+len(contextBytes))
				sensory[0] = 's'
				sensory[1] = '/'
				copy(sensory[2:], contextBytes)

				probability := core.Unit
				count := uint64(1)

				if existing, found := current.Get(sensory); found && len(existing) == 24 {
					count = binary.LittleEndian.Uint64(existing[0:8]) + 1
					probability = math.Float64frombits(binary.LittleEndian.Uint64(existing[8:16]))
				}

				sensoryState := data.NewState(data.NewMap())
				sensoryBridge := data.NewAdapter(nil, sensoryState)
				sensoryIssued := data.NewOutputMap()
				sensoryIssued.Values["probability"] = probability
				sensoryIssued.Values["count"] = float64(count)
				sensoryIssued.Values["feedback"] = 0
				sensoryIssued.Values["graded"] = 0

				for range sensoryBridge.Next(data.NewValue(sensoryIssued)) {
				}

				if err := sensoryBridge.Error(); err != nil {
					op.Error(err)
					return
				}

				for range op.reinforce.Next(data.NewValue(sensoryBridge)) {
				}

				if err := op.reinforce.Error(); err != nil {
					op.Error(err)
					return
				}

				var sensoryUpdated data.Map[float64]

				for pointer := range sensoryBridge.Next(data.NewValue(data.NewMap("probability", "probability"))) {
					sensoryUpdated = *(*data.Map[float64])(pointer)
				}

				if err := sensoryBridge.Error(); err != nil {
					op.Error(err)
					return
				}

				updated, held := sensoryUpdated.Values["probability"]

				if !held {
					op.Error(core.ErrNotHeld)
					return
				}

				var packed [24]byte
				binary.LittleEndian.PutUint64(packed[0:8], count)
				binary.LittleEndian.PutUint64(packed[8:16], math.Float64bits(updated))
				binary.LittleEndian.PutUint64(packed[16:24], clock)
				transaction.Insert(sensory, bytes.Clone(packed[:]))

				var clockPacked [8]byte
				binary.BigEndian.PutUint64(clockPacked[:], clock)
				transaction.Insert([]byte{0}, bytes.Clone(clockPacked[:]))

				var spanPacked [8]byte
				binary.BigEndian.PutUint64(spanPacked[:], span)
				transaction.Insert([]byte{1}, bytes.Clone(spanPacked[:]))

				committed := transaction.Commit()

				if op.root.CompareAndSwap(current, committed) {
					if isNew {
						introduced = class
					}

					if prunedExisting && class != "" {
						if counter, held := op.classes.Load(class); held {
							if counted, ok := counter.(*atomic.Int32); ok && counted != nil {
								counted.Add(-1)
							}
						}
					}

					break
				}
			}

			if introduced != "" {
				counter, _ := op.classes.LoadOrStore(introduced, &atomic.Int32{})
				counted, _ := counter.(*atomic.Int32)

				if counted != nil {
					counted.Add(1)
				}
			}

			root := op.root.Load()
			records := 0
			span := 0.0
			var spanRaw []byte
			spanHeld := false

			if root != nil {
				records = root.Len()
			}

			if root != nil {
				if _, clockHeld := root.Get([]byte{0}); clockHeld {
					records--
				}
			}

			if root != nil {
				spanRaw, spanHeld = root.Get([]byte{1})
			}

			if spanHeld {
				records--
			}

			if spanHeld && len(spanRaw) == 8 {
				span = float64(binary.BigEndian.Uint64(spanRaw))
			}

			issued := data.NewOutputMap()
			issued.Values["records"] = float64(records)
			issued.Values["span"] = span

			for range adapter.Next(data.NewValue(issued)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
