package public

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
)

type latencyProbe struct {
	mu        sync.Mutex
	pending   map[int]time.Time
	nextReqID atomic.Int64
	latency   *NetworkLatency
	interval  time.Duration
}

func newLatencyProbe(latency *NetworkLatency) *latencyProbe {
	if latency == nil {
		latency = SharedNetworkLatency()
	}

	return &latencyProbe{
		pending:  make(map[int]time.Time),
		latency:  latency,
		interval: PingIntervalFromViper(),
	}
}

func (probe *latencyProbe) start(ctx context.Context, write func(map[string]any) error) {
	if probe == nil || write == nil {
		return
	}

	go probe.run(ctx, write)
}

func (probe *latencyProbe) run(ctx context.Context, write func(map[string]any) error) {
	ticker := time.NewTicker(probe.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := probe.sendPing(write); err != nil {
				errnie.Error(err)
			}
		}
	}
}

func (probe *latencyProbe) sendPing(write func(map[string]any) error) error {
	reqID := int(probe.nextReqID.Add(1))
	sentAt := time.Now()

	probe.mu.Lock()
	probe.pending[reqID] = sentAt
	probe.mu.Unlock()

	return write(map[string]any{
		"method": "ping",
		"req_id": reqID,
	})
}

func (probe *latencyProbe) observePong(frame map[string]any) {
	reqID, ok := frame["req_id"].(float64)

	if !ok {
		return
	}

	probe.mu.Lock()
	sentAt, tracked := probe.pending[int(reqID)]
	delete(probe.pending, int(reqID))
	probe.mu.Unlock()

	if !tracked {
		return
	}

	probe.latency.RecordRTT(time.Since(sentAt))
}
