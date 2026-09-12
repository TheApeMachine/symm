package associative

import (
	"encoding/binary"
	"errors"
	"iter"
	"slices"
	"sync"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

/*
ContextCommand discriminates one context intent. Exactly one field is set; any
other shape is a failure recorded in Error and ends the stream.

Encode folds one impulse into the temporal history and answers with the whole
encoded sequence; Reset clears observation-local history.
*/
type ContextCommand struct {
	Encode *grid.Impulse
	Reset  *ResetSignal
}

/*
ContextResult is the encoded temporal sequence: the byte framing the
cognition backoff depends on, [count uint32][count*8B tokens] frames, and how
many temporal steps it rests on.
*/
type ContextResult struct {
	Sequence []byte
	History  int
}

/* Reset clears observation-local temporal history. */
type ResetSignal struct{}

/*
Context turns the temporal trajectory of region sets into the sequence an agent recognises:
R_0 -> R_1 -> ... -> R_t.

It carries no trading semantics: region token sequences are addresses in the
cognitive memory trie. Each timestep set is structurally framed with its region count
to preserve temporal set boundaries without delimiter collisions. It is a
streaming Primitive over an unsafe.Pointer wire.
*/
type Context struct {
	err         error
	out         ContextResult
	mu          sync.RWMutex
	history     [][]grid.Region
	lastVersion uint64
}

/* NewContext instantiates the temporal context Primitive. */
func NewContext() core.Primitive {
	return &Context{
		history: make([][]grid.Region, 0, 16),
	}
}

/*
Next executes each arriving command and yields its result. An invalid command
is recorded in Error and ends the stream.
*/
func (op *Context) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if op.err != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			command := (*ContextCommand)(arriving)

			if err := op.execute(command); err != nil {
				op.Error(err)
				return
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Context) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
execute dispatches one command to its intent and yields its answer into the
result slot.
*/
func (op *Context) execute(command *ContextCommand) error {
	if (command.Encode == nil) == (command.Reset == nil) {
		return errShape("context command must set exactly one intent")
	}

	if command.Reset != nil {
		return op.reset()
	}

	return op.encode(*command.Encode)
}

/* reset clears observation-local temporal history. */
func (op *Context) reset() error {
	op.mu.Lock()
	defer op.mu.Unlock()

	op.history = op.history[:0]
	op.lastVersion = 0
	op.out = ContextResult{}

	return nil
}

/*
encode folds one impulse into the history and reads the whole encoded
sequence.

Each timestep set is length-framed with uint32 count: [count (4B)][token1 (8B)]...[tokenK (8B)].
This structurally prevents collisions between e.g. [r1, r2] -> [r3] and [r1] -> [r2, r3].
*/
func (op *Context) encode(impulse grid.Impulse) error {
	op.mu.Lock()
	defer op.mu.Unlock()

	if impulse.Ready && len(impulse.Regions) > 0 &&
		(impulse.Version == 0 || impulse.Version != op.lastVersion) {
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

		const maxHistory = 128

		if len(op.history) > maxHistory {
			op.history = op.history[len(op.history)-maxHistory:]
		}
	}

	op.out = ContextResult{Sequence: op.encodeHistoryLocked(), History: len(op.history)}

	return nil
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
