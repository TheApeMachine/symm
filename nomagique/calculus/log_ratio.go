package calculus

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
LogRatio computes log(current / previous) for positive finite observations.
*/
type LogRatio struct {
	initial types.Input[scalarMap]
	next    types.Input[scalarMap]
}

var _ types.IO[scalarMap] = (*LogRatio)(nil)

func NewLogRatio(initial types.Input[scalarMap]) *LogRatio {
	return &LogRatio{initial: initial, next: types.NewInput[scalarMap]()}
}

func (ratio *LogRatio) Write(input types.IO[scalarMap]) {
	mapping, err := stageScalar(input, "log_ratio")
	if err != nil {
		ratio.next = types.NewErrorInput(mapping, err)
		return
	}
	ratio.next = scalarInput(mapping)
}

func (ratio *LogRatio) Read() types.IO[scalarMap] {
	if ratio.next.Error() != "" {
		return ratio.next
	}

	mapping := ratio.next.Project().Read()
	current, hasCurrent := scalar(mapping, "current")
	previous, hasPrevious := scalar(mapping, "previous")
	if !hasCurrent || !hasPrevious {
		ratio.next = types.NewErrorInput(mapping,
			scalarValidation("log_ratio", "missing current or previous"))
		return ratio.next
	}
	if !finite(current, previous) || current <= 0 || previous <= 0 {
		ratio.next = types.NewErrorInput(mapping,
			scalarValidation("log_ratio", "positive finite operands required"))
		return ratio.next
	}

	putScalar(mapping, "result", math.Log(current/previous))
	ratio.initial = scalarInput(mapping)
	ratio.next = scalarInput(mapping)
	return ratio.next
}

func (ratio *LogRatio) Project() types.Value[scalarMap] { return ratio.next.Project() }
func (ratio *LogRatio) Error() string                   { return ratio.next.Error() }
func (ratio *LogRatio) Close() error {
	if ratio.initial != nil {
		if err := ratio.initial.Close(); err != nil {
			return err
		}
	}
	if ratio.next != nil {
		if err := ratio.next.Close(); err != nil {
			return err
		}
	}
	ratio.next = types.NewInput[scalarMap]()
	return nil
}
