package trader

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"golang.design/x/lockfree/wf"
)

/*
lane is one lossless bounded SPSC edge in the analytical data plane. Its
payload ring has exactly one producer and one consumer; wake and space tokens
only park those owners when the ring is respectively empty or full.
*/
type lane[T any] struct {
	ring            *wf.RingBuffer[T]
	wake            chan<- struct{}
	space           chan struct{}
	spinLimit       int
	highWater       atomic.Int64
	saturations     atomic.Uint64
	saturationNanos atomic.Uint64
}

func newLane[T any](
	capacity int,
	spinLimit int,
	wake chan<- struct{},
) (*lane[T], error) {
	if capacity <= 0 || capacity&(capacity-1) != 0 {
		return nil, fmt.Errorf("stream lane: power-of-two capacity required")
	}

	if spinLimit < 0 {
		return nil, fmt.Errorf("stream lane: non-negative spin limit required")
	}

	if wake == nil {
		return nil, fmt.Errorf("stream lane: wake channel required")
	}

	return &lane[T]{
		ring:      wf.NewRingBuffer[T](capacity),
		wake:      wake,
		space:     make(chan struct{}, 1),
		spinLimit: spinLimit,
	}, nil
}

/*
Push publishes one lossless value, applying explicit backpressure after the
configured bounded spin budget when the consumer is behind.
*/
func (lane *lane[T]) Push(ctx context.Context, value T) error {
	if lane.ring.Put(value) {
		lane.observeDepth()
		lane.notifyWake()
		return nil
	}

	lane.saturations.Add(1)
	started := time.Now()

	for {
		for range lane.spinLimit {
			runtime.Gosched()

			if lane.ring.Put(value) {
				lane.saturationNanos.Add(uint64(time.Since(started)))
				lane.observeDepth()
				lane.notifyWake()
				return nil
			}
		}

		select {
		case <-ctx.Done():
			lane.saturationNanos.Add(uint64(time.Since(started)))
			return ctx.Err()
		case <-lane.space:
			if lane.ring.Put(value) {
				lane.saturationNanos.Add(uint64(time.Since(started)))
				lane.observeDepth()
				lane.notifyWake()
				return nil
			}
		}
	}
}

/*
TryPush attempts a single non-blocking put. Returns false immediately when the
ring is full, never spins or parks the caller. The lane saturation counter is
incremented on failure so the drop is observable via telemetry.
*/
func (lane *lane[T]) TryPush(value T) bool {
	if !lane.ring.Put(value) {
		lane.saturations.Add(1)
		return false
	}

	lane.observeDepth()
	lane.notifyWake()
	return true
}

/*
Pop removes one value and notifies the sole producer that bounded capacity is
available. The payload operation itself remains the wait-free ring Get.
*/
func (lane *lane[T]) Pop() (T, bool) {
	value, ok := lane.ring.Get()

	if !ok {
		return value, false
	}

	select {
	case lane.space <- struct{}{}:
	default:
	}

	return value, true
}

func (lane *lane[T]) notifyWake() {
	select {
	case lane.wake <- struct{}{}:
	default:
	}
}

func (lane *lane[T]) observeDepth() {
	depth := int64(lane.ring.Len())

	for {
		previous := lane.highWater.Load()

		if depth <= previous || lane.highWater.CompareAndSwap(previous, depth) {
			return
		}
	}
}

/*
LaneTelemetry is the observable bounded-lane pressure accumulated at runtime.
*/
type LaneTelemetry struct {
	Capacity           int
	Depth              int
	HighWater          int64
	Saturations        uint64
	SaturationDuration time.Duration
}

func (lane *lane[T]) telemetry() LaneTelemetry {
	return LaneTelemetry{
		Capacity:           lane.ring.Cap(),
		Depth:              lane.ring.Len(),
		HighWater:          lane.highWater.Load(),
		Saturations:        lane.saturations.Load(),
		SaturationDuration: time.Duration(lane.saturationNanos.Load()),
	}
}
