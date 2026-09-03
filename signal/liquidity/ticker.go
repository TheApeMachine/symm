package liquidity

import (
	"fmt"
	"math"
	"sync"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
)

type symbolState struct {
	joint     equation.JointEstimator
	velBid    equation.LocalRegression
	velAsk    equation.LocalRegression
	velSpread equation.LocalRegression
	lastSec   float64
	lastNsec  float64
	hasTime   bool
}

/*
Ticker is the touch-snapshot market entity. It drives a nomagique v2
equation pipeline (JointEstimator and LocalRegression) per symbol
and projects data.Measurement metrics directly.
*/
type Ticker struct {
	states map[string]*symbolState
	mu     sync.RWMutex
}

/*
NewTicker constructs the Ticker entity.
*/
func NewTicker() *Ticker {
	return &Ticker{
		states: make(map[string]*symbolState),
	}
}

func (ticker *Ticker) Close() error { return nil }

/*
Step receives one market data point, executes the nomagique equation pipeline,
and projects the measurement.
*/
func (ticker *Ticker) Step(trade kraken.TickerData) *data.Measurement[float64] {
	if trade.Bid == nil || trade.Ask == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("liquidity: ticker requires bid and ask")}
	}

	bidPrice := trade.Bid.Float64()
	askPrice := trade.Ask.Float64()
	bidQty := trade.BidQty
	askQty := trade.AskQty

	if bidPrice <= 0 || askPrice <= 0 || askPrice <= bidPrice {
		return &data.Measurement[float64]{Err: fmt.Errorf("liquidity: positive order violated (%f <= %f)", askPrice, bidPrice)}
	}

	sec := float64(trade.Timestamp.Unix())
	nsec := float64(trade.Timestamp.Nanosecond())

	ticker.mu.Lock()
	state, found := ticker.states[trade.Symbol]

	if !found {
		state = &symbolState{}
		ticker.states[trade.Symbol] = state
	}

	if state.hasTime {
		if sec < state.lastSec || (sec == state.lastSec && nsec < state.lastNsec) {
			ticker.mu.Unlock()

			return nil
		}
	}

	state.lastSec = sec
	state.lastNsec = nsec
	state.hasTime = true

	id := fmt.Sprintf("liquidity:%s:%d", trade.Symbol, trade.Timestamp.UnixNano())
	measurement := data.NewMeasurement[float64](id, trade.Symbol, "liquidity", trade.Timestamp, trade.Timestamp)
	measurement.Metadata = make(map[string]float64)

	// Touch quantities
	bidNotional := bidPrice * bidQty
	askNotional := askPrice * askQty
	midpoint := (bidPrice + askPrice) / 2.0
	spread := askPrice - bidPrice
	relativeSpread := spread / midpoint
	twoSidedNotional := math.Min(bidNotional, askNotional)
	imbalance := (bidNotional - askNotional) / (bidNotional + askNotional)

	putMetric(measurement, "best_bid_price", bidPrice, data.UnitRate)
	putMetric(measurement, "best_ask_price", askPrice, data.UnitRate)
	putMetric(measurement, "touch_quantity:bid", bidQty, data.UnitCount)
	putMetric(measurement, "touch_quantity:ask", askQty, data.UnitCount)
	putMetric(measurement, "touch_notional:bid", bidNotional, data.UnitRate)
	putMetric(measurement, "touch_notional:ask", askNotional, data.UnitRate)
	putMetric(measurement, "midpoint", midpoint, data.UnitRate)
	putMetric(measurement, "spread", spread, data.UnitRate)
	putMetric(measurement, "relative_spread", relativeSpread, data.UnitDimensionless)
	putMetric(measurement, "two_sided_touch_notional", twoSidedNotional, data.UnitRate)
	putMetric(measurement, "touch_notional_imbalance", imbalance, data.UnitDimensionless)

	// Input vector X = [ln(bidNotional), ln(askNotional), ln(relativeSpread)]
	values := [3]float64{
		math.Log(bidNotional),
		math.Log(askNotional),
		math.Log(relativeSpread),
	}

	hadMean := state.joint.HasMean()

	// Step the nomagique JointEstimator equation
	state.joint.Step(values, sec, nsec)

	if hadMean {
		putMetric(measurement, "touch_notional_baseline:bid", state.joint.Baseline(0), data.UnitRate)
		putMetric(measurement, "depth_ratio:bid", state.joint.Ratio(0), data.UnitDimensionless)
		putMetric(measurement, "depth_divergence:bid", state.joint.Residual(0), data.UnitNat)

		putMetric(measurement, "touch_notional_baseline:ask", state.joint.Baseline(1), data.UnitRate)
		putMetric(measurement, "depth_ratio:ask", state.joint.Ratio(1), data.UnitDimensionless)
		putMetric(measurement, "depth_divergence:ask", state.joint.Residual(1), data.UnitNat)

		putMetric(measurement, "relative_spread_baseline", state.joint.Baseline(2), data.UnitDimensionless)
		putMetric(measurement, "spread_ratio", state.joint.Ratio(2), data.UnitDimensionless)
		putMetric(measurement, "spread_divergence", state.joint.Residual(2), data.UnitNat)

		if noise, hasNoise := state.joint.Noise(0); hasNoise {
			putMetric(measurement, "depth_noise_scale:bid", noise, data.UnitNat)
		}

		if zscore, hasZ := state.joint.ZScore(0); hasZ {
			putMetric(measurement, "depth_zscore:bid", zscore, data.UnitDimensionless)
		}

		if noise, hasNoise := state.joint.Noise(1); hasNoise {
			putMetric(measurement, "depth_noise_scale:ask", noise, data.UnitNat)
		}

		if zscore, hasZ := state.joint.ZScore(1); hasZ {
			putMetric(measurement, "depth_zscore:ask", zscore, data.UnitDimensionless)
		}

		if noise, hasNoise := state.joint.Noise(2); hasNoise {
			putMetric(measurement, "spread_noise_scale", noise, data.UnitNat)
		}

		if zscore, hasZ := state.joint.ZScore(2); hasZ {
			putMetric(measurement, "spread_zscore", zscore, data.UnitDimensionless)
		}

		if snr, hasSNR := state.joint.SNR(); hasSNR {
			measurement.Metadata[data.MetadataMahalanobisSNR] = snr
		}

		// Step divergence velocity equations
		currentNano := trade.Timestamp.UnixNano()
		horizon := state.joint.Horizon()

		state.velBid.Step(state.joint.Residual(0), currentNano, horizon)
		state.velAsk.Step(state.joint.Residual(1), currentNano, horizon)
		state.velSpread.Step(state.joint.Residual(2), currentNano, horizon)

		if slope, hasSlope := state.velBid.Slope(); hasSlope {
			putMetric(measurement, "divergence_velocity:bid", slope, data.UnitPerSecond)
		}

		if snr, hasSNR := state.velBid.SNR(); hasSNR {
			putMetric(measurement, "divergence_velocity_snr:bid", snr, data.UnitDimensionless)
		}

		if slope, hasSlope := state.velAsk.Slope(); hasSlope {
			putMetric(measurement, "divergence_velocity:ask", slope, data.UnitPerSecond)
		}

		if snr, hasSNR := state.velAsk.SNR(); hasSNR {
			putMetric(measurement, "divergence_velocity_snr:ask", snr, data.UnitDimensionless)
		}

		if slope, hasSlope := state.velSpread.Slope(); hasSlope {
			putMetric(measurement, "spread_divergence_velocity", slope, data.UnitPerSecond)
		}

		if snr, hasSNR := state.velSpread.SNR(); hasSNR {
			putMetric(measurement, "spread_divergence_velocity_snr", snr, data.UnitDimensionless)
		}
	}

	measurement.Metadata[data.MetadataSupport] = state.joint.NEff()
	ticker.mu.Unlock()

	measurement.Finalize()

	return measurement
}

func putMetric(measurement *data.Measurement[float64], name string, value float64, unit data.Unit) {
	measurement.PutMetric(data.NewMetric(
		name, value, nil, nil, unit, data.TimescaleInstantaneous,
	))
}
