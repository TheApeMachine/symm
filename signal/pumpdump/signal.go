package pumpdump

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
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
	lastMeasurement     *data.Measurement
	lastBarMeasurement  *data.Measurement
	clock               volumeclock.Clock
	spreadEstimator     causalEstimator
	volumeRateEstimator causalEstimator
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

	signal.System = runtime.NewSystem(ctx, "pumpdump", signal)

	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil || prior.Label == "" {
		return nil
	}

	priceEntry := data.Pull(prior.Read("price"))
	if priceEntry == nil || priceEntry.Metric == nil {
		if prior.Source == "spot:trade" {
			signal.Error(errnie.Err(errnie.Validation, "[pumpdump] price is required", nil))
		}

		return nil
	}
	price := priceEntry.Metric.Raw()

	qtyEntry := data.Pull(prior.Read("qty"))
	if qtyEntry == nil || qtyEntry.Metric == nil || qtyEntry.Metric.Raw() <= 0 {
		return nil
	}
	qty := qtyEntry.Metric.Raw()

	snapshot, err := broker.ReadSnapshot(prior)

	if err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[pumpdump] stamped book unreadable", err))
		return nil
	}

	var bid, ask float64
	hasTouch := snapshot != nil

	if hasTouch {
		bestBid, bestAsk := snapshot.Touch()
		bid, ask = bestBid.Price.Float64(), bestAsk.Price.Float64()
	}

	atNanos := float64(prior.At.UnixNano())
	midpoint := 0.0

	signal.mu.Lock()
	defer signal.mu.Unlock()

	state, found := signal.states[prior.Label]

	if !found {
		state = &symbolState{}
		signal.states[prior.Label] = state
	}

	out := map[string]float64{}

	if hasTouch {
		midpoint = (bid + ask) * 0.5
		spread := ask - bid

		hasBaseline, baseline := state.spreadEstimator.Step(spread)

		if hasBaseline && baseline > 0 && !core.Negligible(baseline, spread) {
			out["spread_ratio"] = spread / baseline
		}
	}

	tick := state.clock.Step(price, qty, atNanos, midpoint)
	bar, closed := tick.Bar, tick.Closed

	if closed {
		if core.Resolvable(bar.Duration) {
			volumeRate := bar.Quantity / bar.Duration
			tradeRate := bar.Trades / bar.Duration
			out["volume_rate"] = volumeRate
			out["trade_rate"] = tradeRate

			hasVolBase, volBase := state.volumeRateEstimator.Step(volumeRate)

			if hasVolBase && volBase > 0 && !core.Negligible(volBase, volumeRate) {
				volRatio := volumeRate / volBase
				out["volume_ratio"] = volRatio
			}
		}

		if bar.FromMid > 0 && bar.AtMid > 0 {
			midReturn := math.Log(bar.AtMid / bar.FromMid)
			posReturn := math.Max(midReturn, 0)
			negReturn := math.Max(-midReturn, 0)
			out["positive_midpoint_return"] = posReturn
			out["negative_midpoint_return"] = negReturn
		}
	}

	if state.lastMeasurement == nil {
		state.lastMeasurement = data.NewMeasurement(prior.Epoch, prior.Label, signal.Name(), prior.At, prior.From)
	}

	sourceMeas := state.lastMeasurement
	if closed && state.lastBarMeasurement != nil {
		sourceMeas = state.lastBarMeasurement
	}

	res := sourceMeas.Next(prior, out)
	if closed {
		state.lastBarMeasurement = res
	}

	state.lastMeasurement = res

	if closed {
		res.From = time.Unix(0, int64(bar.StartNanos)).UTC()
	}

	if !closed {
		if activeBar, open := state.clock.ActiveBar(); open && activeBar.StartNanos > 0 {
			res.From = time.Unix(0, int64(activeBar.StartNanos)).UTC()
		}
	}

	return res
}
