package paper

import (
	"container/ring"
	"time"
)

/*
ringLatency samples one-way latency values from the websocket latency profile.
*/
type ringLatency struct {
	ring *ring.Ring
}

func newRingLatency(latencies *ring.Ring) *ringLatency {
	return &ringLatency{ring: latencies}
}

func (sampler *ringLatency) Next() time.Duration {
	value := sampler.ring.Value.(time.Duration)
	sampler.ring = sampler.ring.Next()

	return value
}
