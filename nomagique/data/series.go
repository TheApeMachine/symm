package data

import (
	"context"

	"github.com/bytedance/sonic"
)

type SeriesInput struct {
	Key   string  `json:"key"`
	Sec   float64 `json:"sec"`
	Nsec  float64 `json:"nsec"`
	Value any     `json:"value"`
	Query bool    `json:"query"`
}

type SeriesReading struct {
	Key   string  `json:"key"`
	Sec   float64 `json:"sec"`
	Nsec  float64 `json:"nsec"`
	Value any     `json:"value"`
	Found bool    `json:"found"`
}

type seriesRing struct {
	sec    []float64
	nsec   []float64
	values []any
	next   int
	count  int
}

type SeriesServer struct {
	capacity   int
	rings      map[string]*seriesRing
	Downstream func(context.Context, SeriesReading) error
}

func NewSeries() *SeriesServer {
	return &SeriesServer{
		capacity: 100,
		rings:    make(map[string]*seriesRing),
	}
}

func (s *SeriesServer) Evaluate(ctx context.Context, input SeriesInput) (SeriesReading, error) {
	reading := SeriesReading{
		Key:   input.Key,
		Sec:   input.Sec,
		Nsec:  input.Nsec,
		Value: input.Value,
	}

	if s.capacity <= 0 {
		if s.Downstream != nil {
			return reading, s.Downstream(ctx, reading)
		}
		return reading, nil
	}

	reading.Found = s.observe(input.Key, input.Sec, input.Nsec, input.Value)

	if input.Query {
		reading.Value, reading.Found = s.asOf(input.Key, input.Sec, input.Nsec)
	}

	if s.Downstream != nil {
		return reading, s.Downstream(ctx, reading)
	}

	return reading, nil
}

func (s *SeriesServer) Write(ctx context.Context, call Series_write) error {
	args, err := call.Args().Series()
	if err != nil {
		return err
	}

	payloadPtr, err := args.Payload()
	if err != nil {
		return err
	}

	var input SeriesInput
	if payloadPtr.IsValid() {
		data := payloadPtr.Data()
		if len(data) > 0 {
			if unmarshalErr := sonic.Unmarshal(data, &input); unmarshalErr != nil {
				return unmarshalErr
			}
		}
	}

	_, err = s.Evaluate(ctx, input)
	return err
}

func (s *SeriesServer) Done(ctx context.Context, call Series_done) error {
	return nil
}

func (s *SeriesServer) observe(
	key string,
	sec float64,
	nsec float64,
	value any,
) bool {
	ring, exists := s.rings[key]
	if !exists {
		ring = &seriesRing{
			sec:    make([]float64, s.capacity),
			nsec:   make([]float64, s.capacity),
			values: make([]any, s.capacity),
		}
		s.rings[key] = ring
	}

	if ring.count > 0 {
		lastIdx := (ring.next - 1 + s.capacity) % s.capacity
		if sec < ring.sec[lastIdx] || (sec == ring.sec[lastIdx] && nsec <= ring.nsec[lastIdx]) {
			return false
		}
	}

	ring.sec[ring.next] = sec
	ring.nsec[ring.next] = nsec
	ring.values[ring.next] = value
	ring.next = (ring.next + 1) % s.capacity
	if ring.count < s.capacity {
		ring.count++
	}

	return true
}

func (s *SeriesServer) asOf(
	key string,
	sec float64,
	nsec float64,
) (any, bool) {
	ring, exists := s.rings[key]
	if !exists || ring.count == 0 {
		return nil, false
	}

	var best any
	found := false
	maxSec := -1.0
	maxNsec := -1.0

	for i := 0; i < ring.count; i++ {
		idx := (ring.next - 1 - i + s.capacity) % s.capacity
		rSec := ring.sec[idx]
		rNsec := ring.nsec[idx]

		if rSec < sec || (rSec == sec && rNsec <= nsec) {
			if rSec > maxSec || (rSec == maxSec && rNsec > maxNsec) {
				maxSec = rSec
				maxNsec = rNsec
				best = ring.values[idx]
				found = true
			}
		}
	}

	return best, found
}
