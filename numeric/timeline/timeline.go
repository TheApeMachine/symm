package timeline

import (
	"slices"
	"time"
)

/*
Timeline is a sorted sequence of event timestamps.
*/
type Timeline struct {
	times []time.Time
}

/*
New copies and sorts event timestamps.
*/
func New(times []time.Time) Timeline {
	if len(times) < 2 {
		return Timeline{times: times}
	}

	for index := 1; index < len(times); index++ {
		if times[index].Before(times[index-1]) {
			sorted := slices.Clone(times)
			slices.SortFunc(sorted, func(left, right time.Time) int {
				return left.Compare(right)
			})

			return Timeline{times: sorted}
		}
	}

	return Timeline{times: times}
}

func (timeline Timeline) Times() []time.Time {
	return timeline.times
}

func (timeline Timeline) Len() int {
	return len(timeline.times)
}

/*
Gaps returns strictly positive inter-arrival gaps in seconds.
*/
func (timeline Timeline) Gaps() []float64 {
	if len(timeline.times) < 2 {
		return nil
	}

	gaps := make([]float64, 0, len(timeline.times)-1)

	for index := 1; index < len(timeline.times); index++ {
		gap := timeline.times[index].Sub(timeline.times[index-1]).Seconds()

		if gap > 0 {
			gaps = append(gaps, gap)
		}
	}

	return gaps
}

/*
Span returns seconds from the first event through until.
*/
func (timeline Timeline) Span(until time.Time) float64 {
	if len(timeline.times) == 0 {
		return 0
	}

	span := until.Sub(timeline.times[0]).Seconds()

	if span <= 0 {
		return 0
	}

	return span
}
