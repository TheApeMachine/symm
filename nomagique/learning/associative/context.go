package associative

import (
	"encoding/binary"
	"iter"
	"slices"
	"sync"

	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

/*
Context turns the temporal trajectory of region sets into the sequence an agent recognises:
R_0 -> R_1 -> ... -> R_t.
It carries no trading semantics: region token sequences are addresses in the
cognitive memory trie. Each timestep set is structurally framed with its region count
to preserve temporal set boundaries without delimiter collisions.
*/
type Context struct {
	core.Base[grid.Impulse, cognition.Association]
	mu          sync.RWMutex
	history     [][]grid.Region
	lastVersion uint64
}

func NewContext() *Context {
	return &Context{
		history: make([][]grid.Region, 0, 16),
	}
}

/* Reset clears observation-local temporal history. */
func (op *Context) Reset() {
	op.mu.Lock()
	defer op.mu.Unlock()

	op.history = op.history[:0]
	op.lastVersion = 0
}

/* HistoryLen returns the current number of temporal steps accumulated. */
func (op *Context) HistoryLen() int {
	op.mu.RLock()
	defer op.mu.RUnlock()

	return len(op.history)
}

func (op *Context) Next(
	in iter.Seq[core.Primitive[grid.Impulse, grid.Impulse]],
) iter.Seq[core.Primitive[cognition.Association, cognition.Association]] {
	return func(yield func(core.Primitive[cognition.Association, cognition.Association]) bool) {
		for arriving := range in {
			if !yield(op.Carrier(op.Encode(arriving.Read()))) {
				return
			}
		}
	}
}

/*
Sequence encodes the temporal trajectory of active impulse regions into a binary token sequence.
Each timestep set is length-framed with uint32 count: [count (4B)][token1 (8B)]...[tokenK (8B)].
This structurally prevents collisions between e.g. [r1, r2] -> [r3] and [r1] -> [r2, r3].
*/
func (op *Context) Sequence(impulse grid.Impulse) []byte {
	op.mu.Lock()
	defer op.mu.Unlock()

	if !impulse.Ready || len(impulse.Regions) == 0 {
		return op.encodeHistoryLocked()
	}

	if impulse.Version == 0 || impulse.Version != op.lastVersion {
		stepRegions := slices.Clone(impulse.Regions)
		slices.SortFunc(stepRegions, func(left, right grid.Region) int {
			if left.Condition < right.Condition {
				return -1
			}

			if left.Condition > right.Condition {
				return 1
			}

			return 0
		})

		op.history = append(op.history, stepRegions)
		op.lastVersion = impulse.Version
	}

	return op.encodeHistoryLocked()
}

func (op *Context) encodeHistoryLocked() []byte {
	if len(op.history) == 0 {
		return nil
	}

	totalBytes := 0
	for _, step := range op.history {
		if len(step) > 0 {
			totalBytes += 4 + len(step)*8
		}
	}

	if totalBytes == 0 {
		return nil
	}

	sequence := make([]byte, 0, totalBytes)
	var countBuf [4]byte
	var token [8]byte

	for _, step := range op.history {
		if len(step) == 0 {
			continue
		}

		binary.BigEndian.PutUint32(countBuf[:], uint32(len(step)))
		sequence = append(sequence, countBuf[:]...)

		for _, region := range step {
			binary.BigEndian.PutUint64(token[:], region.Condition)
			sequence = append(sequence, token[:]...)
		}
	}

	return sequence
}

/*
Encode constructs an association frame from an impulse. Class and Feedback
are left unset because the learner chooses actions and receives feedback
separately through reinforcement evaluation.
*/
func (op *Context) Encode(impulse grid.Impulse) cognition.Association {
	sequence := op.Sequence(impulse)

	if len(sequence) == 0 {
		return cognition.Association{}
	}

	return cognition.Association{
		Context:   sequence,
		Step:      impulse.Version,
		Retention: cognition.DefaultConfig().DecayFactor(),
	}
}
