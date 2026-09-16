package strategy

import (
	"encoding/binary"

	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

/* Precursor encodes a market's ordered regions into a borrowed trie key. */
type Precursor struct {
	key []byte
}

func NewPrecursor() *Precursor {
	return &Precursor{}
}

func (precursor *Precursor) Encode(impulse *grid.Impulse) []byte {
	if !impulse.Ready || len(impulse.Regions) == 0 {
		return nil
	}

	size := 4 + len(impulse.Label) + 4 + 8*len(impulse.Regions)

	if cap(precursor.key) < size {
		precursor.key = make([]byte, size)
	}

	precursor.key = precursor.key[:size]
	binary.BigEndian.PutUint32(precursor.key, uint32(len(impulse.Label)))
	copy(precursor.key[4:], impulse.Label)
	offset := 4 + len(impulse.Label)
	binary.BigEndian.PutUint32(precursor.key[offset:], uint32(len(impulse.Regions)))

	for index, region := range impulse.Regions {
		binary.BigEndian.PutUint64(precursor.key[offset+4+index*8:], region.Condition)
	}

	return precursor.key
}
