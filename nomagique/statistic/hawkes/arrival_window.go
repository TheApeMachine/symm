package hawkes

import "slices"

/*
arrivalWindow retains one chronological marked-event history. A positive
capacity is a resource bound and evicts the globally oldest event, regardless
of side, so memory pressure cannot distort buy/sell asymmetry. A zero
capacity leaves retention to an explicit time horizon through retainFrom.

The stream returned by stream remains valid until the next window method
call.
*/
type arrivalWindow struct {
	events    []markedEvent
	buy       []float64
	sell      []float64
	capacity  int
	workspace *arrivalWorkspace
}

/*
newArrivalWindow returns a reusable two-sided arrival window with one
optional total-event resource capacity.
*/
func newArrivalWindow(capacity int) *arrivalWindow {
	if capacity < 0 {
		panic("hawkes: arrival window capacity cannot be negative")
	}

	return &arrivalWindow{
		events:    make([]markedEvent, 0, capacity),
		buy:       make([]float64, 0, capacity),
		sell:      make([]float64, 0, capacity),
		capacity:  capacity,
		workspace: newArrivalWorkspace(),
	}
}

/*
appendBuy records the next buy arrival.
*/
func (window *arrivalWindow) appendBuy(arrivalSec float64) {
	window.append(markedEvent{atSec: arrivalSec, side: sideBuy})
}

/*
appendSell records the next sell arrival.
*/
func (window *arrivalWindow) appendSell(arrivalSec float64) {
	window.append(markedEvent{atSec: arrivalSec, side: sideSell})
}

/*
retainFrom discards observations before the statistically selected memory
horizon while preserving every event at the boundary.
*/
func (window *arrivalWindow) retainFrom(startSec float64) {
	first := 0

	for first < len(window.events) && window.events[first].atSec < startSec {
		first++
	}

	if first == 0 {
		return
	}

	copy(window.events, window.events[first:])
	window.events = window.events[:len(window.events)-first]
}

/*
stream returns the current sorted arrival view without allocating.
*/
func (window *arrivalWindow) stream() arrivalStream {
	window.buy = window.buy[:0]
	window.sell = window.sell[:0]

	for _, event := range window.events {
		if event.side == sideBuy {
			window.buy = append(window.buy, event.atSec)

			continue
		}

		window.sell = append(window.sell, event.atSec)
	}

	return window.workspace.stream(window.buy, window.sell)
}

func (window *arrivalWindow) append(event markedEvent) {
	window.events = append(window.events, event)

	if len(window.events) > 1 && event.atSec < window.events[len(window.events)-2].atSec {
		slices.SortStableFunc(window.events, func(left, right markedEvent) int {
			switch {
			case left.atSec < right.atSec:
				return -1
			case left.atSec > right.atSec:
				return 1
			default:
				return 0
			}
		})
	}

	if window.capacity == 0 || len(window.events) <= window.capacity {
		return
	}

	copy(window.events, window.events[1:])
	window.events = window.events[:window.capacity]
}
