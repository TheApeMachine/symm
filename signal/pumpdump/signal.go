package pumpdump

import (
	"context"
	"math"
	"sync"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/volumeclock"
)

type causalEstimator struct {
	count float64
	mean  float64
	m2    float64
}

func (ce *causalEstimator) Step(value float64) (hasBaseline bool, baseline float64) {
	priorCount := ce.count
	priorMean := ce.mean

	ce.count++
	delta := value - ce.mean
	ce.mean += delta / ce.count
	ce.m2 += delta * (value - ce.mean)

	if priorCount == 0 {
		return false, 0
	}

	return true, priorMean
}

type symbolState struct {
	clock               volumeclock.Clock
	spreadEstimator     causalEstimator
	volumeRateEstimator causalEstimator
}

type Signal struct {
	*runtime.System
	mu     sync.Mutex
	states map[string]*symbolState
	keys   []string
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		states: make(map[string]*symbolState),
		keys: []string{
			"spread_ratio",
			"volume_rate",
			"trade_rate",
			"volume_ratio",
			"positive_midpoint_return",
			"negative_midpoint_return",
		},
	}

	signal.System = runtime.NewSystem(ctx, "pumpdump", signal)
	return signal
}

func (signal *Signal) Step(value *data.Value) *data.Value {
	if signal.Status() != runtime.READY || value == nil || len(value.Layers) == 0 {
		return value
	}

	price := data.Attribute[float64](value.Layers[0], "data", 0, "price")
	qty := data.Attribute[float64](value.Layers[0], "data", 0, "qty")
	if price <= 0 || qty <= 0 {
		return value
	}

	snapshot, _ := broker.ReadSnapshot(value)
	var bid, ask float64
	hasTouch := snapshot != nil && len(snapshot.Bids) > 0 && len(snapshot.Asks) > 0
	if hasTouch {
		bestBid, bestAsk := snapshot.Touch()
		bid, ask = bestBid.Price.Float64(), bestAsk.Price.Float64()
	}

	atNanos := float64(value.Timestamp)
	midpoint := 0.0

	signal.mu.Lock()
	defer signal.mu.Unlock()

	state, found := signal.states[value.Scope]
	if !found {
		state = &symbolState{}
		signal.states[value.Scope] = state
	}

	var spreadRatio float64
	if hasTouch {
		midpoint = (bid + ask) * 0.5
		spread := ask - bid
		hasBaseline, baseline := state.spreadEstimator.Step(spread)
		if hasBaseline && baseline > 0 && !core.Negligible(baseline, spread) {
			spreadRatio = spread / baseline
		}
	}

	tick := state.clock.Step(price, qty, atNanos, midpoint)
	bar, closed := tick.Bar, tick.Closed

	var volumeRate, tradeRate, volRatio, posReturn, negReturn float64
	if closed {
		if core.Resolvable(bar.Duration) {
			volumeRate = bar.Quantity / bar.Duration
			tradeRate = bar.Trades / bar.Duration
			hasVolBase, volBase := state.volumeRateEstimator.Step(volumeRate)
			if hasVolBase && volBase > 0 && !core.Negligible(volBase, volumeRate) {
				volRatio = volumeRate / volBase
			}
		}

		if bar.FromMid > 0 && bar.AtMid > 0 {
			midReturn := math.Log(bar.AtMid / bar.FromMid)
			posReturn = math.Max(midReturn, 0)
			negReturn = math.Max(-midReturn, 0)
		}
	}

	return data.AppendVector(value, []float64{
		spreadRatio,
		volumeRate,
		tradeRate,
		volRatio,
		posReturn,
		negReturn,
	}, signal.keys...)
}
