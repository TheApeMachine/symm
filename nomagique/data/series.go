/*
Package data retains bounded event-time observations and answers causal
retrieval over them.
*/
package data

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
SeriesInput is one observation or one as-of query. A query input reads the
newest retained value observed no later than its event time; an observation
input retains its timestamped value without imposing arrival-time order.

The clock is any normalized (seconds, nanoseconds) coordinate pair. Retention
is a ring: the oldest observation is evicted when a key's ring is full.
*/
type SeriesInput[Value any] struct {
	Key   string
	Sec   float64
	Nsec  float64
	Value Value
	Query bool
}

/*
SeriesReading is the retained or retrieved value for one key, with Found
reporting whether the input was admissible and the ring answered.
*/
type SeriesReading[Value any] struct {
	Key   string
	Sec   float64
	Nsec  float64
	Value Value
	Found bool
}

/*
Series retains one bounded ring of timestamped values per key and answers the
newest value observed no later than a queried event time. The series never
explains an event with a later observation.
*/
type Series[Value any] struct {
	*core.PrimitiveError

	capacity int
	rings    map[string]*seriesRing[Value]
	reading  SeriesReading[Value]
}

type seriesRing[Value any] struct {
	sec    []float64
	nsec   []float64
	values []Value
	next   int
	count  int
}

/*
NewSeries creates fixed storage for each key observed by one owner. A
non-positive capacity is recorded through Error and the primitive yields
nothing.
*/
func NewSeries[Value any](capacity int) *Series[Value] {
	series := &Series[Value]{PrimitiveError: core.NewPrimitiveError(), capacity: capacity}

	if capacity <= 0 {
		series.Error(fmt.Errorf(
			"data: series capacity %d must be positive: %w", capacity, core.ErrDomain,
		))
	}

	return series
}

/*
Next folds each arriving observation into its key's ring, or answers each
arriving as-of query from it.
*/
func (series *Series[Value]) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*SeriesInput[Value])(arriving)
			series.reading = SeriesReading[Value]{
				Key:   input.Key,
				Sec:   input.Sec,
				Nsec:  input.Nsec,
				Value: input.Value,
			}

			if input.Query {
				series.reading.Value, series.reading.Found = series.asOf(
					input.Key, input.Sec, input.Nsec,
				)
			} else {
				series.reading.Found = series.observe(
					input.Key, input.Sec, input.Nsec, input.Value,
				)
			}

			if !yield(unsafe.Pointer(&series.reading)) {
				return
			}
		}
	}
}

/*
observe retains one timestamped value. A repeated event time replaces the
earlier value in place.
*/
func (series *Series[Value]) observe(
	key string,
	sec float64,
	nsec float64,
	value Value,
) bool {
	if series.capacity <= 0 || key == "" || nsec < 0 || nsec >= 1e9 {
		return false
	}

	ring := series.rings[key]

	if ring == nil {
		ring = &seriesRing[Value]{
			sec:    make([]float64, series.capacity),
			nsec:   make([]float64, series.capacity),
			values: make([]Value, series.capacity),
		}

		if series.rings == nil {
			series.rings = make(map[string]*seriesRing[Value])
		}

		series.rings[key] = ring
	}

	for index := range ring.count {
		if ring.sec[index] == sec && ring.nsec[index] == nsec {
			ring.values[index] = value
			return true
		}
	}

	ring.sec[ring.next] = sec
	ring.nsec[ring.next] = nsec
	ring.values[ring.next] = value
	ring.next = (ring.next + 1) % series.capacity

	if ring.count < series.capacity {
		ring.count++
	}

	return true
}

/*
asOf returns the newest retained value observed no later than the queried
event time.
*/
func (series *Series[Value]) asOf(
	key string,
	sec float64,
	nsec float64,
) (Value, bool) {
	var missing Value

	if key == "" {
		return missing, false
	}

	ring := series.rings[key]

	if ring == nil {
		return missing, false
	}

	bestIndex := -1
	bestSec, bestNsec := 0.0, 0.0

	for index := range ring.count {
		if ring.sec[index] > sec ||
			ring.sec[index] == sec && ring.nsec[index] > nsec {
			continue
		}

		if bestIndex < 0 || ring.sec[index] > bestSec ||
			ring.sec[index] == bestSec && ring.nsec[index] > bestNsec {
			bestIndex = index
			bestSec, bestNsec = ring.sec[index], ring.nsec[index]
		}
	}

	if bestIndex < 0 {
		return missing, false
	}

	return ring.values[bestIndex], true
}
