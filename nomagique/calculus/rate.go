package calculus

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

/*
Rate calculates finite count divided by positive finite duration.
*/
func Rate(input *types.Frame) {
	count, hasCount := input.Get(SymbolCount)
	duration, hasDuration := input.Get(SymbolDuration)

	if !hasCount || !hasDuration || !utils.IsFinite(count) || !utils.IsFinite(duration) {
		input.Err = fmt.Errorf("calculus: rate requires finite count and duration")

		return
	}

	if duration <= 0 {
		input.Err = fmt.Errorf("calculus: rate duration must be positive")

		return
	}

	result := count / duration

	if !utils.IsFinite(result) {
		input.Err = fmt.Errorf("calculus: rate overflowed")

		return
	}

	input.Put(SymbolRate, result)
}
