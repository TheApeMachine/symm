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
touch records the gap since the previous cut for symbol and returns the updated
mean inter-cut span.
*/
func (state *symbolCadence) touch(at time.Time) time.Duration {
	if state == nil {
		return 0
	}

	if state.last.IsZero() {
		state.last = at
		return 0
	}

	if at.After(state.last) {
		gap := at.Sub(state.last)
		state.n++
		state.mean += (gap - state.mean) / time.Duration(state.n)
	}

	state.last = at

	return state.mean
}

/*
cadenceBook holds per-symbol inter-cut tempo used to decay idle edges.
*/
type cadenceBook struct {
	symbols map[string]*symbolCadence
}

/*
touch records symbol cadence at at and returns the updated mean inter-cut span.
*/
func (book *cadenceBook) touch(symbol string, at time.Time) time.Duration {
	if book.symbols == nil {
		book.symbols = map[string]*symbolCadence{}
	}

	state := book.symbols[symbol]

	if state == nil {
		book.symbols[symbol] = &symbolCadence{last: at}
		return 0
	}

	return state.touch(at)
}

/*
decayIdle scales down symbol edges that were not strengthened on this cut.
Weight is multiplied by mean/(mean+age) using the symbol's observed inter-cut
mean, so longer silence relative to that cadence reduces coupling.
*/
func (book *cadenceBook) decayIdle(graph *Graph, symbol string, at time.Time, mean time.Duration) {
	if book == nil || graph == nil || mean <= 0 {
		return
	}

	keys := graph.edgesBySymbol[symbol]
	remaining := keys[:0]

	for _, key := range keys {
		relation := graph.EdgeIndex[key]

		if relation == nil {
			continue
		}

		if graph.touched != nil {
			if _, ok := graph.touched[key]; ok {
				remaining = append(remaining, key)
				continue
			}
		}

		if relation.At.IsZero() || !at.After(relation.At) {
			remaining = append(remaining, key)
			continue
		}

		age := at.Sub(relation.At)
		relation.Weight *= float64(mean) / float64(mean+age)

		if relation.Weight <= 0 {
			delete(graph.EdgeIndex, key)
			for index, edge := range graph.Edges {
				if edge == relation {
					graph.Edges = append(graph.Edges[:index], graph.Edges[index+1:]...)
					break
				}
			}
			continue
		}

		remaining = append(remaining, key)
	}

	graph.edgesBySymbol[symbol] = remaining
}
