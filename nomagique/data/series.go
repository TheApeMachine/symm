/*
Package data retains bounded event-time observations and answers causal
retrieval over them.
*/
package data

import (
	"fmt"
	"sync"
)

/*
Series retains one bounded ring of timestamped values per key and answers the
newest value observed no later than a queried event time. The clock is any
normalized (seconds, nanoseconds) coordinate pair, the value is any payload,
and the series never explains an event with a later observation. Retention is
a ring: the oldest observation is evicted when a key's ring is full.
*/
type Series[Value any] struct {
	mutex    sync.RWMutex
	capacity int
	rings    map[string]*seriesRing[Value]
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
*/
func NewSeries[Value any](capacity int) (*Series[Value], error) {
	if capacity <= 0 {
		return nil, fmt.Errorf(
			"data: series capacity %d must be positive",
			capacity,
		)
	}

	return &Series[Value]{
		capacity: capacity,
		rings:    make(map[string]*seriesRing[Value]),
	}, nil
}

/*
MustNewSeries creates a series or panics for an invalid capacity.
*/
func MustNewSeries[Value any](capacity int) *Series[Value] {
	series, err := NewSeries[Value](capacity)

	if err != nil {
		panic(err)
	}

	return series
}

/*
Observe retains one timestamped value without imposing arrival-time order. A
repeated event time replaces the earlier value in place.
*/
func (series *Series[Value]) Observe(
	key string,
	sec float64,
	nsec float64,
	value Value,
) bool {
	if series == nil || key == "" || nsec < 0 || nsec >= 1e9 {
		return false
	}

	series.mutex.Lock()
	defer series.mutex.Unlock()

	ring := series.rings[key]

	if ring == nil {
		ring = &seriesRing[Value]{
			sec:    make([]float64, series.capacity),
			nsec:   make([]float64, series.capacity),
			values: make([]Value, series.capacity),
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
AsOf returns the newest retained value observed no later than the queried
event time.
*/
func (series *Series[Value]) AsOf(
	key string,
	sec float64,
	nsec float64,
) (Value, bool) {
	var missing Value

	if series == nil || key == "" {
		return missing, false
	}

	series.mutex.RLock()
	defer series.mutex.RUnlock()

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
