package websocket

import (
	"container/ring"
	"encoding/json"
	"math/rand"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

type LatencyType uint8

const (
	WEBSOCKET LatencyType = iota
	REST
	FILL
)

var (
	latencySimulator     *Simulator
	latencySimulatorOnce sync.Once
)

/*
NewLatencySimulator is the shared latency pool for public and private transports.
*/
func NewLatencySimulator(booter *system.Booter) *Simulator {
	latencySimulatorOnce.Do(func() {
		latencySimulator = NewSimulator()
		if err := latencySimulator.Initialize(); err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal, err.Error(), err,
			))
		}
	})

	return latencySimulator
}

/*
Simulator adds more realistic behavior to the paper emulation
by recording real latencies and replaying them when the paper
emulation is used. This allows for a much more realistic
simulation of the market and avoids the optimism bias.
*/
type Simulator struct {
	booter        *system.Booter
	status        types.Status
	mu            sync.Mutex
	wsLatencies   *ring.Ring
	restLatencies *ring.Ring
	fillLatencies *ring.Ring
}

func NewSimulator() *Simulator {
	return &Simulator{
		status:        types.INITIALIZING,
		wsLatencies:   ring.New(64),
		restLatencies: ring.New(64),
		fillLatencies: ring.New(64),
	}
}

func (simulator *Simulator) Status() types.Status {
	return simulator.status
}

/*
Initialize seeds websocket and REST rings with bootstrap values until
real public/private measurements arrive. Fill stays random only.
*/
func (simulator *Simulator) Initialize() error {
	errnie.Info("initializing simulator")

	simulator.mu.Lock()
	defer simulator.mu.Unlock()

	wsLatencies := simulator.wsLatencies
	for idx := 0; idx < wsLatencies.Len(); idx++ {
		wsLatencies.Value = time.Duration(30+rand.Intn(90)) * time.Millisecond
		wsLatencies = wsLatencies.Next()
	}

	restLatencies := simulator.restLatencies
	for idx := 0; idx < restLatencies.Len(); idx++ {
		restLatencies.Value = time.Duration(30+rand.Intn(90)) * time.Millisecond
		restLatencies = restLatencies.Next()
	}

	fillLatencies := simulator.fillLatencies
	for idx := 0; idx < fillLatencies.Len(); idx++ {
		fillLatencies.Value = time.Duration(40+rand.Intn(360)) * time.Millisecond
		fillLatencies = fillLatencies.Next()
	}

	simulator.status = types.READY
	return nil
}

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

	time.Sleep(wait)
	fn()
}

func (simulator *Simulator) Emit(
	paper *Paper, latencyType LatencyType, channel string, payload json.Marshaler,
) error {
	var err error

	simulator.Do(latencyType, func() {
		err = paper.Emit(channel, payload)
	})

	return err
}

func (simulator *Simulator) Record(latencyType LatencyType, latency time.Duration) {
	simulator.mu.Lock()
	defer simulator.mu.Unlock()

	switch latencyType {
	case WEBSOCKET:
		simulator.wsLatencies.Value = latency
		simulator.wsLatencies = simulator.wsLatencies.Next()
	case REST:
		simulator.restLatencies.Value = latency
		simulator.restLatencies = simulator.restLatencies.Next()
	}
}
