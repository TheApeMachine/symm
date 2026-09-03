package adaptive

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Number is the unboxed 8-byte carrier for all streaming values in nomagique.
Its canonical definition lives in nomagique/types.
*/
type Number = types.Number

/*
Node is the closed engine contract for all transformations, filters, equations,
and reductions in nomagique.
*/
type Node = types.Node
