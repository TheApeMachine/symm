package logic

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Mix statically divides weight evenly across active branches, or applies predefined weights.
*/
type Mix struct {
	A types.Number
	B types.Number
	C types.Number
	D types.Number
}

func (mix Mix) Route(number types.Number) (types.Number, types.Number, types.Number, types.Number) {
	return mix.A, mix.B, mix.C, mix.D
}
