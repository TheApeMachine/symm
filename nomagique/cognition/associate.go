package cognition

import (
	"bytes"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewAssociate creates a Value closure that transforms sequential transitions into empirical associations.
When the first transition arrives, it yields an array with no class (nil).
When subsequent transitions arrive, it yields [2][]byte{precursor, current}.
No structs, pure Value closure.
*/
type Associate types.Value[[]byte, [2][]byte]
func NewAssociate(precursor ...types.Bytes) Associate {
	var prec []byte
	if len(precursor) > 0 && precursor[0] != nil {
		prec = precursor[0](nil)
	}

	return func(current []byte) [2][]byte {
		if len(current) == 0 {
			return [2][]byte{}
		}

		if len(prec) == 0 {
			prec = bytes.Clone(current)
			return [2][]byte{bytes.Clone(current), nil}
		}

		out := [2][]byte{bytes.Clone(prec), bytes.Clone(current)}
		prec = bytes.Clone(current)
		return out
	}
}
