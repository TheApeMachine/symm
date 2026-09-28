package websocket

import (
	"context"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
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
	status        atomic.Int32
	wsLatencies   [64]atomic.Int64
	restLatencies [64]atomic.Int64
	fillLatencies [64]atomic.Int64
	wsCursor      atomic.Uint64
	restCursor    atomic.Uint64
	fillCursor    atomic.Uint64
	seed          int64
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
		ctx:   ctx,
		clock: clock,
		seed:  seed,
	}

	if err := simulator.Initialize(); err != nil {
		errnie.Error(err)
	}

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
func (simulator *Simulator) Status() runtime.Stage {
	return runtime.Stage(simulator.status.Load())
}

/*
Initialize seeds websocket and REST rings with bootstrap values until real
public/private measurements arrive. Fill stays random only.
*/
func (simulator *Simulator) Initialize() error {
	randomSource := rand.New(rand.NewSource(simulator.seed))

	for index := 0; index < 64; index++ {
		wsDur := time.Duration(30+randomSource.Intn(90)) * time.Millisecond
		simulator.wsLatencies[index].Store(int64(wsDur))

		restDur := time.Duration(30+randomSource.Intn(90)) * time.Millisecond
		simulator.restLatencies[index].Store(int64(restDur))

		fillDur := time.Duration(40+randomSource.Intn(360)) * time.Millisecond
		simulator.fillLatencies[index].Store(int64(fillDur))
	}

	simulator.status.Store(int32(runtime.READY))

	return nil
}

/*
Do waits through the injected clock under the stack context, then runs fn.
*/
func (simulator *Simulator) Do(latencyType LatencyType, fn func()) {
	var wait time.Duration

	switch latencyType {
	case WEBSOCKET:
		index := simulator.wsCursor.Add(1) % 64
		wait = time.Duration(simulator.wsLatencies[index].Load())
	case REST:
		index := simulator.restCursor.Add(1) % 64
		wait = time.Duration(simulator.restLatencies[index].Load())
	case FILL:
		index := simulator.fillCursor.Add(1) % 64
		wait = time.Duration(simulator.fillLatencies[index].Load())
	}

	if err := simulator.clock.Sleep(simulator.ctx, wait); err != nil {
		return
	}

	fn()
}

/*
Record stores an observed latency sample for later replay.
*/
func (simulator *Simulator) Record(latencyType LatencyType, latency time.Duration) {
	switch latencyType {
	case WEBSOCKET:
		index := simulator.wsCursor.Add(1) % 64
		simulator.wsLatencies[index].Store(int64(latency))
	case REST:
		index := simulator.restCursor.Add(1) % 64
		simulator.restLatencies[index].Store(int64(latency))
	}
}
