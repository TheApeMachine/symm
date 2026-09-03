package logic

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Pick routes all weight (1.0) to a chosen branch index, setting all other branch weights to 0.0.
*/
type Pick struct {
	Index int
}

func (pick Pick) Route(number types.Number) (types.Number, types.Number, types.Number, types.Number) {
	if pick.Index == 0 {
		return 1, 0, 0, 0
	}

	if pick.Index == 1 {
		return 0, 1, 0, 0
	}

	if pick.Index == 2 {
		return 0, 0, 1, 0
	}

	if pick.Index == 3 {
		return 0, 0, 0, 1
	}

	return 0, 0, 0, 0
}
