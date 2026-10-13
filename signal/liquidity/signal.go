package liquidity

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type symbolState struct {
	lastMeasurement *data.Measurement
}

type Signal struct {
	*runtime.System
	mu     sync.Mutex
	states map[string]*symbolState
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		states: make(map[string]*symbolState),
	}

	signal.System = runtime.NewSystem(ctx, "liquidity", signal)

	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	snapshot, err := broker.ReadSnapshot(prior)

	if err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[liquidity] stamped book unreadable", err))
		return nil
	}

	if snapshot == nil {
		return nil
	}

	bestBid, bestAsk := snapshot.Touch()
	bid := bestBid.Price.Float64()
	ask := bestAsk.Price.Float64()

	signal.mu.Lock()
	defer signal.mu.Unlock()

	state, exists := signal.states[prior.Label]
	if !exists {
		state = &symbolState{}
		signal.states[prior.Label] = state
	}

	midpoint := (bid + ask) / 2.0
	spread := ask - bid

	out := map[string]float64{
		"best_bid_price":  bid,
		"best_ask_price":  ask,
		"relative_spread": spread / midpoint,
	}

	if state.lastMeasurement == nil {
		state.lastMeasurement = data.NewMeasurement(prior.Epoch, prior.Label, signal.Name(), prior.At, prior.From)
	}

	state.lastMeasurement = state.lastMeasurement.Next(prior, out)
	return state.lastMeasurement
}
