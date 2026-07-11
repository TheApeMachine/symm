package websocket

import (
	"container/ring"
	"encoding/json"
	"math/rand"
	"sync"
	"time"
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
LatencySimulator is the shared latency pool for public and private transports.
*/
func NewLatencySimulator() *Simulator {
	latencySimulatorOnce.Do(func() {
		latencySimulator = NewSimulator()
		latencySimulator.Initialize()
	})

	return latencySimulator
}

/*
Simulator adds more realistic behavior to the paper emulation,
by recording real latencies and replaying them when the paper
emulation is used. This allows for a much more realistic
simulation of the market, and avoids the optimism bias.
*/
type Simulator struct {
	wsLatencies   *ring.Ring
	restLatencies *ring.Ring
	fillLatencies *ring.Ring
}

func NewSimulator() *Simulator {
	return &Simulator{
		wsLatencies:   ring.New(64),
		restLatencies: ring.New(64),
		fillLatencies: ring.New(64),
	}
}

/*
Initialize seeds websocket and REST rings with bootstrap values until
real public/private measurements arrive. Fill stays random only.
*/
func (simulator *Simulator) Initialize() {
	simulator.wsLatencies.Do(func(item any) {
		simulator.wsLatencies.Value = time.Duration(30+rand.Intn(90)) * time.Millisecond
		simulator.wsLatencies = simulator.wsLatencies.Next()
	})

	simulator.restLatencies.Do(func(item any) {
		simulator.restLatencies.Value = time.Duration(30+rand.Intn(90)) * time.Millisecond
		simulator.restLatencies = simulator.restLatencies.Next()
	})

	simulator.fillLatencies.Do(func(item any) {
		simulator.fillLatencies.Value = time.Duration(40+rand.Intn(360)) * time.Millisecond
		simulator.fillLatencies = simulator.fillLatencies.Next()
	})
}

func (simulator *Simulator) Do(latencyType LatencyType, fn func()) {
	var latencyRing *ring.Ring

	switch latencyType {
	case WEBSOCKET:
		latencyRing = simulator.wsLatencies
	case REST:
		latencyRing = simulator.restLatencies
	case FILL:
		latencyRing = simulator.fillLatencies
	}

	var wait time.Duration

	if latencyRing.Value != nil {
		wait = latencyRing.Value.(time.Duration)
	}

	switch latencyType {
	case WEBSOCKET:
		simulator.wsLatencies = latencyRing.Next()
	case REST:
		simulator.restLatencies = latencyRing.Next()
	case FILL:
		simulator.fillLatencies = latencyRing.Next()
	}

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
	switch latencyType {
	case WEBSOCKET:
		simulator.wsLatencies.Value = latency
		simulator.wsLatencies = simulator.wsLatencies.Next()
	case REST:
		simulator.restLatencies.Value = latency
		simulator.restLatencies = simulator.restLatencies.Next()
	}
}
