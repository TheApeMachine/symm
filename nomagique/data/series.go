package data

import (
	"context"

	"github.com/theapemachine/errnie"
)

type seriesRing struct {
	sec    []float64
	nsec   []float64
	values []float64
	next   int
	count  int
}

type SeriesServer struct {
	capacity int
	rings    map[string]*seriesRing
	reading  seriesReadingState
}

type seriesReadingState struct {
	key   string
	sec   float64
	nsec  float64
	value float64
	found bool
}

func NewSeries() *SeriesServer {
	return &SeriesServer{
		capacity: 100,
		rings:    make(map[string]*seriesRing),
	}
}

func (server *SeriesServer) Write(ctx context.Context, call Series_write) error {
	args := call.Args()
	key, err := args.Key()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"series: failed to read key",
			err,
		))
	}

	sec := args.Sec()
	nsec := args.Nsec()
	val := args.Value()
	query := args.Query()

	if query {
		server.reading = server.latest(key)
		return nil
	}

	ring, exists := server.rings[key]

	if !exists {
		ring = &seriesRing{
			sec:    make([]float64, server.capacity),
			nsec:   make([]float64, server.capacity),
			values: make([]float64, server.capacity),
		}
		server.rings[key] = ring
	}

	ring.sec[ring.next] = sec
	ring.nsec[ring.next] = nsec
	ring.values[ring.next] = val
	ring.next = (ring.next + 1) % server.capacity

	if ring.count < server.capacity {
		ring.count++
	}

	server.reading = seriesReadingState{
		key:   key,
		sec:   sec,
		nsec:  nsec,
		value: val,
		found: true,
	}

	return nil
}

/*
latest reads back the newest sample retained under one key. A key nothing has
been written under is reported as not found rather than as a zero sample, so a
series that has never been observed stays distinguishable from one observing
zero.
*/
func (server *SeriesServer) latest(key string) seriesReadingState {
	ring, exists := server.rings[key]

	if !exists || ring.count == 0 {
		return seriesReadingState{key: key}
	}

	newest := (ring.next - 1 + server.capacity) % server.capacity

	return seriesReadingState{
		key:   key,
		sec:   ring.sec[newest],
		nsec:  ring.nsec[newest],
		value: ring.values[newest],
		found: true,
	}
}

func (server *SeriesServer) Done(ctx context.Context, call Series_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"series: alloc results failed",
			err,
		))
	}

	if err := results.SetKey(server.reading.key); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"series: set key failed",
			err,
		))
	}
	results.SetSec(server.reading.sec)
	results.SetNsec(server.reading.nsec)
	results.SetValue(server.reading.value)
	results.SetFound(server.reading.found)

	server.reading = seriesReadingState{}
	return nil
}
