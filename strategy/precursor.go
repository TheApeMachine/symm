package strategy

import (
	"encoding/binary"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

/* Precursor encodes a market's ordered regions and position state into a borrowed trie key. */
type Precursor struct {
	*core.PrimitiveError
	key      []byte
	question cognition.Question
	command  cognition.Command
}

func NewPrecursor() *Precursor {
	return &Precursor{PrimitiveError: core.NewPrimitiveError()}
}

func (precursor *Precursor) Encode(impulse *grid.Impulse, holding bool) []byte {
	if !impulse.Ready || len(impulse.Regions) == 0 {
		return nil
	}

	size := 4 + len(impulse.Label) + 1 + 4 + 8*len(impulse.Regions)

	if cap(precursor.key) < size {
		precursor.key = make([]byte, size)
	}

	precursor.key = precursor.key[:size]
	binary.BigEndian.PutUint32(precursor.key, uint32(len(impulse.Label)))
	copy(precursor.key[4:], impulse.Label)
	offset := 4 + len(impulse.Label)
	precursor.key[offset] = 0

	if holding {
		precursor.key[offset] = 1
	}

	offset++
	binary.BigEndian.PutUint32(precursor.key[offset:], uint32(len(impulse.Regions)))

	for index, region := range impulse.Regions {
		binary.BigEndian.PutUint64(precursor.key[offset+4+index*8:], region.Condition)
	}

	return precursor.key
}

func (precursor *Precursor) Next(input iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range input {
			precursor.question = cognition.Question{Context: precursor.Encode((*grid.Impulse)(arriving), false), Exact: true}
			precursor.command = cognition.Command{Evaluate: &precursor.question}

			if !yield(unsafe.Pointer(&precursor.command)) {
				return
			}
		}
	}
}
