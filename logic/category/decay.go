package category

import "time"

/*
symbolCadence tracks inter-cut gaps for one symbol so idle edges decay on the
symbol's own event tempo rather than a fixed wall-clock window.
*/
type symbolCadence struct {
	last time.Time
	mean time.Duration
	n    int
}

/*
touchCadence records the gap since the previous cut for symbol and returns the
updated mean inter-cut span.
*/
func (graph *Graph) touchCadence(symbol string, at time.Time) time.Duration {
	if graph.cadence == nil {
		graph.cadence = map[string]*symbolCadence{}
	}

	state := graph.cadence[symbol]

	if state == nil {
		graph.cadence[symbol] = &symbolCadence{last: at}
		return 0
	}

	if !state.last.IsZero() && at.After(state.last) {
		gap := at.Sub(state.last)
		state.n++
		state.mean += (gap - state.mean) / time.Duration(state.n)
	}

	state.last = at

	return state.mean
}

/*
decayIdle scales down edges for symbol that were not strengthened on this cut.
Weight is multiplied by mean/(mean+age) using the symbol's observed inter-cut
mean, so longer silence relative to that cadence reduces coupling.
*/
func (graph *Graph) decayIdle(symbol string, at time.Time, mean time.Duration) {
	if mean <= 0 {
		return
	}

	for key, relation := range graph.edges {
		if key.symbol != symbol || relation == nil {
			continue
		}

		if graph.touched != nil {
			if _, ok := graph.touched[key]; ok {
				continue
			}
		}

		if relation.At.IsZero() || !at.After(relation.At) {
			continue
		}

		age := at.Sub(relation.At)
		relation.Weight *= float64(mean) / float64(mean+age)

		if relation.Weight <= 0 {
			delete(graph.edges, key)
		}
	}
}
