package response

import "time"

/*
LatencySampler returns the next simulated one-way network latency.
*/
type LatencySampler interface {
	Next() time.Duration
}

type zeroLatency struct{}

func (zeroLatency) Next() time.Duration { return 0 }

func ZeroLatency() LatencySampler { return zeroLatency{} }
