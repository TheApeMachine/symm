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
func NewAssociate() Associate {
	var precursor []byte

	return func(current []byte) [2][]byte {
		if len(current) == 0 {
			return [2][]byte{}
		}

		if len(precursor) == 0 {
			precursor = bytes.Clone(current)
			return [2][]byte{bytes.Clone(current), nil}
		}

		out := [2][]byte{bytes.Clone(precursor), bytes.Clone(current)}
		precursor = bytes.Clone(current)
		return out
	}
}
