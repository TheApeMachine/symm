package paper

import (
	"container/ring"
	"sync"
	"time"
)

/*
ringLatency samples one-way latency values from the websocket latency profile.
Next is safe for concurrent use.
*/
type ringLatency struct {
	mu   sync.Mutex
	ring *ring.Ring
}

func newRingLatency(latencies *ring.Ring) *ringLatency {
	if latencies == nil {
		return nil
	}

	return &ringLatency{ring: latencies}
}

func (sampler *ringLatency) Next() time.Duration {
	if sampler == nil || sampler.ring == nil {
		return 0
	}

	sampler.mu.Lock()
	defer sampler.mu.Unlock()

	value, ok := sampler.ring.Value.(time.Duration)
	sampler.ring = sampler.ring.Next()

	if !ok {
		return 0
	}

	return value
}
