package cmd

import (
	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
restoreThesis replaces the active thesis only when optional recovery state is
valid. Invalid state remains observable without mutating or blocking the live
runtime.
*/
func restoreThesis(
	thesis *types.Thesis,
	channel chan<- []byte,
	encoded []byte,
	message string,
	bookManager *spot.BookManager,
) *types.Thesis {
	restored := types.NewThesis(bookManager)

	if err := sonic.Unmarshal(encoded, restored); err != nil {
		errnie.Error(errnie.Err(errnie.UnprocessableContent, message, err))
		return thesis
	}

	return restored
}
