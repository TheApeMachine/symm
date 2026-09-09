package agent

import (
	"slices"
	"time"
)

type transition struct {
	at         time.Time
	conditions []uint64
}

// Context retains distinct preceding region states only inside the producer's
// measured horizon. It is the agent's causal memory, not another runtime stage.
func (agent *Agent[Action]) Context(label string, at, from time.Time, current []uint64) []uint64 {
	if agent.histories == nil {
		agent.histories = make(map[string][]transition)
	}
	history := agent.histories[label]
	first := 0
	for first < len(history) && history[first].at.Before(from) {
		first++
	}
	history = slices.Delete(history, 0, first)

	if len(history) == 0 || !slices.Equal(history[len(history)-1].conditions, current) {
		history = append(history, transition{at: at, conditions: slices.Clone(current)})
	}
	agent.histories[label] = history
	var context []uint64
	for _, state := range history {
		context = append(context, uint64(1)<<63|2)
		context = append(context, state.conditions...)
	}
	return context
}
