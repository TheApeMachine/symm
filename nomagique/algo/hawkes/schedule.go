package hawkes

import "math"

/*
schedule invalidates a parameter epoch after newly observed events reach the
sampling-error scale ceil(sqrt(N)) of that fit. Event-count scheduling adapts
to each symbol's activity instead of assigning one wall-clock cooldown to every
market.
*/
type schedule struct {
	threshold int
	changed   int
}

func (schedule *schedule) Reset(fittedEvents int) {
	schedule.threshold = int(math.Ceil(math.Sqrt(float64(fittedEvents))))
	schedule.changed = 0
}

func (schedule *schedule) Observe(changedEvents int) {
	if changedEvents > 0 {
		schedule.changed += changedEvents
	}
}

func (schedule *schedule) Ready() bool {
	return schedule.threshold > 0 && schedule.changed >= schedule.threshold
}

func (schedule *schedule) Remaining() int {
	remaining := schedule.threshold - schedule.changed

	if remaining > 0 {
		return remaining
	}

	return 0
}
