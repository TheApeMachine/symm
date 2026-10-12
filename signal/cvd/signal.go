package cvd

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/volumeclock"
)

/*
onlineTracker is a causal baseline: Score reports value against the state
before it, then incorporates it. The baseline is defined from the second
value, the z-score once core.PriorScale admits the prior dispersion.
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

	if scale, scorable := core.PriorScale(priorCount, priorM2, value, priorMean); scorable {
		score.hasZ = true
		score.zScore = (value - priorMean) / scale
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

	if ratio && score.baseline > 0 && !core.Negligible(score.baseline, value) {
		out[name+"_ratio"] = value / score.baseline
	}

	if score.hasZ {
		out[name+"_zscore"] = score.zScore
	}
}

/*
emitLog writes name's baseline, divergence, ratio, and z-score for a positive
multiplicative value the tracker scored as logValue = ln(value): the baseline
is the geometric mean exp(prior mean of ln), the divergence the log residual,
the ratio exp(residual). This is pumpdump's notional-rate treatment.
*/
func (score trackerScore) emitLog(out map[string]float64, name string, logValue float64) {
	if !score.hasBaseline {
		return
	}

	residual := logValue - score.baseline
	out[name+"_baseline"] = math.Exp(score.baseline)
	out[name+"_divergence"] = residual
	out[name+"_ratio"] = math.Exp(residual)

	if score.hasZ {
		out[name+"_zscore"] = score.zScore
	}
}

/*
window is the executed flow of the open volume bar, split by aggressor side.
*/
type window struct {
	buyCount     float64
	sellCount    float64
	buyQty       float64
	sellQty      float64
	buyNotional  float64
	sellNotional float64
}

/*
symbolState windows its flow totals on the same volume clock as pumpdump: a
bar's target is the median prior trade quantity, fixed when it opens. Totals
are reported once per closed bar, never accumulated since an epoch.
*/
type symbolState struct {
	lastMeasurement        *data.Measurement
	clock                  volumeclock.Clock
	window                 window
	lastAt                 time.Time
	prevMidpoint           float64
	prevNetNotionalRate    float64
	prevGrossNotionalRate  float64
	hasPrevRates           bool
	grossRateTracker       onlineTracker
	signedNetTracker       onlineTracker
	midpointReturnTracker  onlineTracker
	midpointModulus        core.LogModulus
	midpointModulusTracker onlineTracker
	historyPoints          [][2]float64
	historyDistances       []float64
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

	key := prior.Label + "@" + strconv.FormatInt(prior.Epoch, 10)
	state, found := signal.states[key]

	if !found {
		state = &symbolState{}
		signal.states[key] = state
	}

	side := prior.Meta("side")

	if side != "buy" && side != "sell" {
		if state.lastMeasurement != nil {
			return state.lastMeasurement
		}

		if prior.Source == "spot:trade" {
			errnie.Error(errnie.Err(
				errnie.NotFound,
				"[signal.cvd] no side",
				nil,
			))
		}

		return nil
	}

	priceEntry := data.Pull(prior.Read("price"))
	qtyEntry := data.Pull(prior.Read("qty"))

	if priceEntry == nil || priceEntry.Metric == nil || qtyEntry == nil || qtyEntry.Metric == nil {
		if state.lastMeasurement != nil {
			return state.lastMeasurement
		}

		return nil
	}

	price := priceEntry.Metric.Raw
	qty := qtyEntry.Metric.Raw

	if price <= 0 || qty <= 0 {
		if state.lastMeasurement != nil {
			return state.lastMeasurement
		}

		return nil
	}

	// Rates need elapsed time since the symbol's previous trade that the venue
	// clock resolves (core.Resolvable); the first trade, same-timestamp
	// trades, and trades closer than that leave them undefined.
	var timeDelta float64

	if !state.lastAt.IsZero() && prior.At.After(state.lastAt) {
		timeDelta = prior.At.Sub(state.lastAt).Seconds()
	}

	hasDelta := core.Resolvable(timeDelta)

	var tradeBuyQty, tradeSellQty float64

	if side == "buy" {
		tradeBuyQty = qty
	}

	if side == "sell" {
		tradeSellQty = qty
	}

	tradeBuyNotional := tradeBuyQty * price
	tradeSellNotional := tradeSellQty * price
	tradeGrossNotional := tradeBuyNotional + tradeSellNotional
	tradeNetNotional := tradeBuyNotional - tradeSellNotional

	// The response is priced on the book midpoint, (best bid + best ask) / 2,
	// of the snapshot ingress stamped onto the trade at its sequence number:
	// the book as it stood at this trade, not whatever the live book holds when
	// this consumer runs. A trade stamped without a book has no midpoint.
	snapshot, err := broker.ReadSnapshot(prior)

	if err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[cvd] stamped book unreadable", err))
		return nil
	}

	var midpoint float64

	hasBook := snapshot != nil

	if hasBook {
		bid, ask := snapshot.Touch()
		midpoint = (bid.Price.Float64() + ask.Price.Float64()) / 2.0
	}

	out := map[string]float64{}
	tick := state.clock.Step(price, qty, float64(prior.At.UnixNano()), midpoint)

	if tick.Counted {
		if side == "buy" {
			state.window.buyCount++
		} else {
			state.window.sellCount++
		}

		state.window.buyQty += tradeBuyQty
		state.window.sellQty += tradeSellQty
		state.window.buyNotional += tradeBuyNotional
		state.window.sellNotional += tradeSellNotional
	}

	var signedNet trackerScore

	if tick.Closed {
		signedNet = state.closeWindow(out)
	}

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

		// A per-trade rate is multiplicative and spans orders of magnitude, so
		// it is scored on ln(rate), as pumpdump scores its notional rate.
		gross = state.grossRateTracker.Score(math.Log(grossNotionalRate))
		gross.emitLog(out, "gross_notional_rate", math.Log(grossNotionalRate))

		if state.hasPrevRates {
			out["net_notional_rate_velocity"] = (netNotionalRate - state.prevNetNotionalRate) / timeDelta
			out["gross_notional_rate_velocity"] = (grossNotionalRate - state.prevGrossNotionalRate) / timeDelta
		}

		state.prevNetNotionalRate = netNotionalRate
		state.prevGrossNotionalRate = grossNotionalRate
		state.hasPrevRates = true
	}

	if hasBook && state.prevMidpoint > 0 {
		midpointLogReturn := math.Log(midpoint / state.prevMidpoint)
		flowAligned := midpointLogReturn

		if side == "sell" {
			flowAligned = -midpointLogReturn
		}

		out["response_midpoint:from"] = state.prevMidpoint
		out["response_midpoint:at"] = midpoint
		out["midpoint_log_return"] = midpointLogReturn
		out["flow_aligned_midpoint_return"] = flowAligned
		out["midpoint_response_per_net_notional"] = midpointLogReturn / tradeNetNotional

		if hasDelta {
			midpointReturnRate := midpointLogReturn / timeDelta
			out["midpoint_return_rate"] = midpointReturnRate

			// The baseline and divergence stay in rate units; the z-score is
			// taken on the signed log-modulus scale, since the rate inherits
			// the interval's orders-of-magnitude spread.
			score := state.midpointReturnTracker.Score(midpointReturnRate)
			score.hasZ = false
			score.emit(out, "midpoint_return_rate", midpointReturnRate, false)

			if compressed, defined := state.midpointModulus.Step(midpointReturnRate); defined {
				if modulusScore := state.midpointModulusTracker.Score(compressed); modulusScore.hasZ {
					out["midpoint_return_rate_zscore"] = modulusScore.zScore
				}
			}
		}
	}

	if hasBook {
		state.prevMidpoint = midpoint
	}

	if signedNet.hasZ && gross.hasZ {
		state.historyPath(out, [2]float64{signedNet.zScore, gross.zScore})
	}

	state.lastAt = prior.At

	var res *data.Measurement

	if state.lastMeasurement == nil {
		res = data.NewFromPrior(prior, signal.Name(), out)
	}

	if res == nil {
		res = state.lastMeasurement.Next(prior.At, out)
	}

	state.lastMeasurement = res

	if tick.Closed {
		res.From = time.Unix(0, int64(tick.Bar.StartNanos)).UTC()
	}

	return res
}

/*
closeWindow writes the closed bar's flow totals and scores its signed net
fraction against earlier bars, then starts the next bar empty.
*/
func (state *symbolState) closeWindow(out map[string]float64) trackerScore {
	flow := state.window
	state.window = window{}

	count := flow.buyCount + flow.sellCount
	grossQty := flow.buyQty + flow.sellQty
	netQty := flow.buyQty - flow.sellQty
	grossNotional := flow.buyNotional + flow.sellNotional
	netNotional := flow.buyNotional - flow.sellNotional
	signedNetFraction := netNotional / grossNotional

	out["trade_count:buy"] = flow.buyCount
	out["trade_count:sell"] = flow.sellCount
	out["trade_count"] = count
	out["signed_count_fraction"] = (flow.buyCount - flow.sellCount) / count
	out["executed_quantity:buy"] = flow.buyQty
	out["executed_quantity:sell"] = flow.sellQty
	out["gross_executed_quantity"] = grossQty
	out["net_executed_quantity"] = netQty
	out["cumulative_volume_delta"] = netQty
	out["aggressive_notional:buy"] = flow.buyNotional
	out["aggressive_notional:sell"] = flow.sellNotional
	out["gross_notional"] = grossNotional
	out["net_notional"] = netNotional
	out["cumulative_notional_delta"] = netNotional
	out["signed_net_fraction"] = signedNetFraction
	out["mean_trade_notional"] = grossNotional / count

	signedNet := state.signedNetTracker.Score(signedNetFraction)
	signedNet.emit(out, "signed_net_fraction", signedNetFraction, false)

	return signedNet
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
