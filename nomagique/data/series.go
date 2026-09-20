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

	reading := seriesReadingState{
		key:   key,
		sec:   sec,
		nsec:  nsec,
		value: val,
		found: true,
	}

	if !query {
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
	}

	server.reading = reading
	return nil
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
