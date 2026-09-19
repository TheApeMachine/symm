package cognition

import (
	"bytes"
	"encoding/binary"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewWeight creates a Value closure that unpacks stored records into [3]uint64.
Index 0: Count
Index 1: Mass (math.Float64frombits)
Index 2: WriteStep
No structs, pure Value closure.
*/
type Weight types.Value[[]byte, [3]uint64]
func NewWeight() Weight {
	return func(record []byte) [3]uint64 {
		var pw [3]uint64
		if len(record) < 24 {
			return pw
		}

		if err := binary.Read(bytes.NewReader(record), binary.LittleEndian, &pw); err != nil {
			return [3]uint64{}
		}

		return pw
	}
}

/*
NewPack creates a Value closure that packs a [3]uint64 into its wire representation.
No structs, pure Value closure.
*/
type Pack types.Value[[3]uint64, []byte]
func NewPack() Pack {
	return func(pw [3]uint64) []byte {
		var buf bytes.Buffer
		if err := binary.Write(&buf, binary.LittleEndian, pw); err != nil {
			return nil
		}
		return buf.Bytes()
	}
}
