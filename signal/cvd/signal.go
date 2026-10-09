package cvd

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
onlineTracker is a causal baseline: Score reports value against the state
before it, then incorporates it. The baseline is defined from the second
value, the z-score once the prior dispersion is positive.
*/
type onlineTracker struct {
	count float64
	mean  float64
	m2    float64
}

type trackerScore struct {
	hasBaseline bool
	baseline    float64
	hasZ        bool
	zScore      float64
}

func (tracker *onlineTracker) Score(value float64) trackerScore {
	priorCount := tracker.count
	priorMean := tracker.mean
	priorM2 := tracker.m2

	tracker.count++
	delta := value - tracker.mean
	tracker.mean += delta / tracker.count
	tracker.m2 += delta * (value - tracker.mean)

	if priorCount == 0 {
		return trackerScore{}
	}

	score := trackerScore{hasBaseline: true, baseline: priorMean}

	if priorCount > 1 && priorM2 > 0 {
		score.hasZ = true
		score.zScore = (value - priorMean) / math.Sqrt(priorM2/(priorCount-1))
	}

	return score
}

/*
emit writes name's baseline, divergence, and z-score as far as they are
defined; the ratio to the baseline only where the baseline is positive.
*/
func (score trackerScore) emit(out map[string]float64, name string, value float64, ratio bool) {
	if !score.hasBaseline {
		return
	}

	out[name+"_baseline"] = score.baseline
	out[name+"_divergence"] = value - score.baseline

	if ratio && score.baseline > 0 {
		out[name+"_ratio"] = value / score.baseline
	}

	if score.hasZ {
		out[name+"_zscore"] = score.zScore
	}
}

type symbolState struct {
	buyCount              float64
	sellCount             float64
	totalCount            float64
	buyQty                float64
	sellQty               float64
	buyNotional           float64
	sellNotional          float64
	lastAt                time.Time
	firstAtNano           float64
	prevPrice             float64
	prevNetNotionalRate   float64
	prevGrossNotionalRate float64
	hasPrevRates          bool
	grossRateTracker      onlineTracker
	signedNetTracker      onlineTracker
	midpointReturnTracker onlineTracker
	historyPoints         [][2]float64
	historyDistances      []float64
}

type Signal struct {
	*runtime.System
	states map[string]*symbolState
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		states: make(map[string]*symbolState),
	}

	signal.System = runtime.NewSystem(ctx, "cvd", signal)

	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil {
		return nil
	}

	side := prior.Meta("side")

	if side != "buy" && side != "sell" {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"[signal.cvd] no side",
			nil,
		))

		return nil
	}

	priceEntry := data.Pull(prior.Read("price"))
	qtyEntry := data.Pull(prior.Read("qty"))

	if priceEntry == nil || priceEntry.Metric == nil || qtyEntry == nil || qtyEntry.Metric == nil {
		return nil
	}

	price := priceEntry.Metric.Raw
	qty := qtyEntry.Metric.Raw

	if price <= 0 || qty <= 0 {
		return nil
	}

	key := prior.Label + "@" + strconv.FormatInt(prior.Epoch, 10)
	state, found := signal.states[key]

	if !found {
		state = &symbolState{
			firstAtNano: float64(prior.At.UnixNano()),
		}
		signal.states[key] = state
	}

	// Rates need elapsed time since the symbol's previous trade; the first
	// trade and same-timestamp trades leave them undefined.
	var timeDelta float64
	hasDelta := !state.lastAt.IsZero() && prior.At.After(state.lastAt)

	if hasDelta {
		timeDelta = prior.At.Sub(state.lastAt).Seconds()
	}

	var tradeBuyQty, tradeSellQty float64
	var tradeBuyCount, tradeSellCount float64

	if side == "buy" {
		tradeBuyCount = 1
		tradeBuyQty = qty
	}

	if side == "sell" {
		tradeSellCount = 1
		tradeSellQty = qty
	}

	state.buyCount += tradeBuyCount
	state.sellCount += tradeSellCount
	state.totalCount += 1
	state.buyQty += tradeBuyQty
	state.sellQty += tradeSellQty

	tradeBuyNotional := tradeBuyQty * price
	tradeSellNotional := tradeSellQty * price
	tradeGrossNotional := tradeBuyNotional + tradeSellNotional
	tradeNetNotional := tradeBuyNotional - tradeSellNotional

	state.buyNotional += tradeBuyNotional
	state.sellNotional += tradeSellNotional

	grossExecutedQty := state.buyQty + state.sellQty
	netExecutedQty := state.buyQty - state.sellQty
	cumulativeVolumeDelta := netExecutedQty

	grossNotional := state.buyNotional + state.sellNotional
	netNotional := state.buyNotional - state.sellNotional
	cumulativeNotionalDelta := netNotional

	signedNetFraction := netNotional / grossNotional

	out := map[string]float64{
		"trade_count:buy":           state.buyCount,
		"trade_count:sell":          state.sellCount,
		"trade_count":               state.totalCount,
		"signed_count_fraction":     (state.buyCount - state.sellCount) / state.totalCount,
		"executed_quantity:buy":     state.buyQty,
		"executed_quantity:sell":    state.sellQty,
		"gross_executed_quantity":   grossExecutedQty,
		"net_executed_quantity":     netExecutedQty,
		"cumulative_volume_delta":   cumulativeVolumeDelta,
		"aggressive_notional:buy":   state.buyNotional,
		"aggressive_notional:sell":  state.sellNotional,
		"gross_notional":            grossNotional,
		"net_notional":              netNotional,
		"cumulative_notional_delta": cumulativeNotionalDelta,
		"signed_net_fraction":       signedNetFraction,
		"mean_trade_notional":       grossNotional / state.totalCount,
		"cvd_epoch_from":            state.firstAtNano,
	}

	signedNet := state.signedNetTracker.Score(signedNetFraction)
	signedNet.emit(out, "signed_net_fraction", signedNetFraction, false)

	var gross trackerScore
	hasRates := hasDelta

	if hasRates {
		grossNotionalRate := tradeGrossNotional / timeDelta
		netNotionalRate := tradeNetNotional / timeDelta

		out["trade_rate"] = 1.0 / timeDelta
		out["gross_notional_rate"] = grossNotionalRate
		out["net_notional_rate"] = netNotionalRate
		out["buy_notional_rate"] = tradeBuyNotional / timeDelta
		out["sell_notional_rate"] = tradeSellNotional / timeDelta

		gross = state.grossRateTracker.Score(grossNotionalRate)
		gross.emit(out, "gross_notional_rate", grossNotionalRate, true)

		if state.hasPrevRates {
			out["net_notional_rate_velocity"] = (netNotionalRate - state.prevNetNotionalRate) / timeDelta
			out["gross_notional_rate_velocity"] = (grossNotionalRate - state.prevGrossNotionalRate) / timeDelta
		}

		state.prevNetNotionalRate = netNotionalRate
		state.prevGrossNotionalRate = grossNotionalRate
		state.hasPrevRates = true
	}

	if state.prevPrice > 0 {
		midpointLogReturn := math.Log(price / state.prevPrice)
		flowAligned := midpointLogReturn

		if side == "sell" {
			flowAligned = -midpointLogReturn
		}

		out["response_midpoint:from"] = state.prevPrice
		out["response_midpoint:at"] = price
		out["midpoint_log_return"] = midpointLogReturn
		out["flow_aligned_midpoint_return"] = flowAligned
		out["midpoint_response_per_net_notional"] = midpointLogReturn / tradeNetNotional

		if hasDelta {
			midpointReturnRate := midpointLogReturn / timeDelta
			out["midpoint_return_rate"] = midpointReturnRate
			state.midpointReturnTracker.Score(midpointReturnRate).emit(
				out, "midpoint_return_rate", midpointReturnRate, false,
			)
		}
	}

	if signedNet.hasZ && gross.hasZ {
		state.historyPath(out, [2]float64{signedNet.zScore, gross.zScore})
	}

	state.lastAt = prior.At
	state.prevPrice = price

	return prior.Next(signal.Name(), out)
}

/*
historyPath scores how far this trade's (signed-net-fraction, gross-rate)
z-score point lies from the nearest retained one, and that distance's rank
among past nearest distances.
*/
func (state *symbolState) historyPath(out map[string]float64, target [2]float64) {
	if len(state.historyPoints) > 0 {
		minDist := math.Inf(1)

		for _, point := range state.historyPoints {
			dx := target[0] - point[0]
			dy := target[1] - point[1]
			minDist = math.Min(minDist, math.Sqrt(dx*dx+dy*dy))
		}

		out["historical_path_distance"] = minDist

		if len(state.historyDistances) > 0 {
			below := 0

			for _, past := range state.historyDistances {
				if past <= minDist {
					below++
				}
			}

			out["historical_path_percentile"] = float64(below) / float64(len(state.historyDistances))
		}

		state.historyDistances = append(state.historyDistances, minDist)

		if len(state.historyDistances) > 256 {
			state.historyDistances = state.historyDistances[len(state.historyDistances)-256:]
		}
	}

	state.historyPoints = append(state.historyPoints, target)

	if len(state.historyPoints) > 256 {
		state.historyPoints = state.historyPoints[len(state.historyPoints)-256:]
	}
}
