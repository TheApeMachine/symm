/*
Package data retains bounded event-time observations and answers causal
retrieval over them.
*/
package data

import (
	"github.com/theapemachine/symm/nomagique/types"
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

type seriesRing[Value any] struct {
	sec    []float64
	nsec   []float64
	values []Value
	next   int
	count  int
}

/*
NewSeries creates fixed storage for each key observed by one owner.
No structs, pure Value closure holding rings state.
*/
type Series[Value any] types.Value[SeriesInput[Value], SeriesReading[Value]]
func NewSeries[Value any](capacity types.Integer) Series[Value] {
	capVal := 100
	if capacity != nil {
		capVal = capacity(nil)
	}
	rings := make(map[string]*seriesRing[Value])

	return func(input SeriesInput[Value]) SeriesReading[Value] {
		reading := SeriesReading[Value]{
			Key:   input.Key,
			Sec:   input.Sec,
			Nsec:  input.Nsec,
			Value: input.Value,
		}

		if capVal <= 0 {
			return reading
		}

		if input.Query {
			reading.Value, reading.Found = asOf(rings, input.Key, input.Sec, input.Nsec)
		} else {
			reading.Found = observe(rings, capVal, input.Key, input.Sec, input.Nsec, input.Value)
		}

		return reading
	}
}

func observe[Value any](
	rings map[string]*seriesRing[Value],
	capacity int,
	key string,
	sec float64,
	nsec float64,
	value Value,
) bool {
	if capacity <= 0 || key == "" || nsec < 0 || nsec >= 1e9 {
		return false
	}

	ring := rings[key]
	if ring == nil {
		ring = &seriesRing[Value]{
			sec:    make([]float64, capacity),
			nsec:   make([]float64, capacity),
			values: make([]Value, capacity),
		}
		rings[key] = ring
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
	ring.next = (ring.next + 1) % capacity

	if ring.count < capacity {
		ring.count++
	}

	return true
}

func asOf[Value any](
	rings map[string]*seriesRing[Value],
	key string,
	sec float64,
	nsec float64,
) (Value, bool) {
	var missing Value

	if key == "" {
		return missing, false
	}

	ring := rings[key]
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
