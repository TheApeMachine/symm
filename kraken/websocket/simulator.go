package websocket

import (
	"container/ring"
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/theapemachine/symm/types"
)

type LatencyType uint8

const (
	WEBSOCKET LatencyType = iota
	REST
	FILL
)

/*
Clock supplies deterministic waits for the paper/live latency simulator.
*/
type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, wait time.Duration) error
}

/*
WallClock uses the process clock for production latency replay.
*/
type WallClock struct{}

/*
Now returns wall time.
*/
func (WallClock) Now() time.Time {
	return time.Now()
}

/*
Sleep waits until duration elapses or the context ends.
*/
func (WallClock) Sleep(ctx context.Context, wait time.Duration) error {
	if wait <= 0 {
		return nil
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

/*
Simulator adds realistic latency to paper emulation by recording real
latencies and replaying them. One simulator belongs to one stack; it is never
a process-wide singleton.
*/
type Simulator struct {
	ctx           context.Context
	clock         Clock
	status        types.Status
	mu            sync.Mutex
	wsLatencies   *ring.Ring
	restLatencies *ring.Ring
	fillLatencies *ring.Ring
	seed          int64
	rng           *rand.Rand
}

/*
NewLatencySimulator constructs a per-stack simulator with an injected clock and
seed. A nil clock uses WallClock. A non-positive seed derives from Now.
*/
func NewLatencySimulator(ctx context.Context, clock Clock, seed int64) *Simulator {
	if clock == nil {
		clock = WallClock{}
	}

	if seed <= 0 {
		seed = clock.Now().UnixNano()
	}

	if ctx == nil {
		ctx = context.Background()
	}

	simulator := &Simulator{
		ctx:           ctx,
		clock:         clock,
		status:        types.INITIALIZING,
		wsLatencies:   ring.New(64),
		restLatencies: ring.New(64),
		fillLatencies: ring.New(64),
		seed:          seed,
		rng:           rand.New(rand.NewSource(seed)),
	}

	_ = simulator.Initialize()

	return simulator
}

/*
NewSimulator constructs a wall-clock simulator with a fresh seed for tests.
*/
func NewSimulator() *Simulator {
	return NewLatencySimulator(context.Background(), WallClock{}, 0)
}

/*
Seed returns the PRNG seed recorded for exact latency-bootstrap replay.
*/
func (simulator *Simulator) Seed() int64 {
	return simulator.seed
}

/*
Status reports simulator readiness.
*/
func (simulator *Simulator) Status() types.Status {
	return simulator.status
}

/*
Initialize seeds websocket and REST rings with bootstrap values until real
public/private measurements arrive. Fill stays random only.
*/
func (simulator *Simulator) Initialize() error {
	simulator.mu.Lock()
	defer simulator.mu.Unlock()

	wsLatencies := simulator.wsLatencies
	for idx := 0; idx < wsLatencies.Len(); idx++ {
		wsLatencies.Value = time.Duration(30+simulator.rng.Intn(90)) * time.Millisecond
		wsLatencies = wsLatencies.Next()
	}

	restLatencies := simulator.restLatencies
	for idx := 0; idx < restLatencies.Len(); idx++ {
		restLatencies.Value = time.Duration(30+simulator.rng.Intn(90)) * time.Millisecond
		restLatencies = restLatencies.Next()
	}

	fillLatencies := simulator.fillLatencies
	for idx := 0; idx < fillLatencies.Len(); idx++ {
		fillLatencies.Value = time.Duration(40+simulator.rng.Intn(360)) * time.Millisecond
		fillLatencies = fillLatencies.Next()
	}

	simulator.status = types.READY
	return nil
}

/*
Do waits through the injected clock under the stack context, then runs fn.
*/
func (simulator *Simulator) Do(latencyType LatencyType, fn func()) {
	var wait time.Duration

	simulator.mu.Lock()

	switch latencyType {
	case WEBSOCKET:
		if simulator.wsLatencies != nil && simulator.wsLatencies.Value != nil {
			wait = simulator.wsLatencies.Value.(time.Duration)
		}

		if simulator.wsLatencies != nil {
			simulator.wsLatencies = simulator.wsLatencies.Next()
		}
	case REST:
		if simulator.restLatencies != nil && simulator.restLatencies.Value != nil {
			wait = simulator.restLatencies.Value.(time.Duration)
		}

		if simulator.restLatencies != nil {
			simulator.restLatencies = simulator.restLatencies.Next()
		}
	case FILL:
		if simulator.fillLatencies != nil && simulator.fillLatencies.Value != nil {
			wait = simulator.fillLatencies.Value.(time.Duration)
		}

		if simulator.fillLatencies != nil {
			simulator.fillLatencies = simulator.fillLatencies.Next()
		}
	}

	simulator.mu.Unlock()

	_ = simulator.clock.Sleep(simulator.ctx, wait)
	fn()
}

/*
Record stores an observed latency sample for later replay.
*/
func (simulator *Simulator) Record(latencyType LatencyType, latency time.Duration) {
	simulator.mu.Lock()
	defer simulator.mu.Unlock()

	switch latencyType {
	case WEBSOCKET:
		simulator.wsLatencies.Value = latency
	case REST:
		simulator.restLatencies.Value = latency
	}
}
