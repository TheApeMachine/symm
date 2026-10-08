package cvd

import (
	"context"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

var outputKeys = []string{
	"trade_count:buy",
	"trade_count:sell",
	"trade_count",
	"signed_count_fraction",
	"executed_quantity:buy",
	"executed_quantity:sell",
	"gross_executed_quantity",
	"net_executed_quantity",
	"cumulative_volume_delta",
	"aggressive_notional:buy",
	"aggressive_notional:sell",
	"gross_notional",
	"net_notional",
	"cumulative_notional_delta",
	"signed_net_fraction",
	"mean_trade_notional",
	"trade_rate",
	"gross_notional_rate",
	"net_notional_rate",
	"buy_notional_rate",
	"sell_notional_rate",
	"cvd_epoch_from",
	"response_midpoint:from",
	"response_midpoint:at",
	"midpoint_log_return",
	"midpoint_return_rate",
	"flow_aligned_midpoint_return",
	"midpoint_response_per_net_notional",
	"gross_notional_rate_baseline",
	"gross_notional_rate_ratio",
	"gross_notional_rate_divergence",
	"gross_notional_rate_zscore",
	"signed_net_fraction_baseline",
	"signed_net_fraction_divergence",
	"signed_net_fraction_zscore",
	"midpoint_return_rate_baseline",
	"midpoint_return_rate_divergence",
	"midpoint_return_rate_zscore",
	"net_notional_rate_velocity",
	"gross_notional_rate_velocity",
	"historical_path_distance",
	"historical_path_percentile",
}

type onlineTracker struct {
	count float64
	mean  float64
	m2    float64
	std   float64
}

func (tracker *onlineTracker) Step(value float64) {
	tracker.count++
	delta := value - tracker.mean
	tracker.mean += delta / tracker.count
	delta2 := value - tracker.mean
	tracker.m2 += delta * delta2

	if tracker.count > 1 {
		variance := tracker.m2 / (tracker.count - 1)
		if variance > 0 {
			tracker.std = math.Sqrt(variance)
		}
	}
}

func (tracker *onlineTracker) Baseline() float64 {
	return tracker.mean
}

func (tracker *onlineTracker) Divergence(value float64) float64 {
	return value - tracker.mean
}

func (tracker *onlineTracker) Ratio(value float64) float64 {
	if tracker.mean == 0 {
		return 1.0
	}

	return value / tracker.mean
}

func (tracker *onlineTracker) ZScore(value float64) float64 {
	if tracker.std <= 0 {
		return 0.0
	}

	return (value - tracker.mean) / tracker.std
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
	grossRateTracker      onlineTracker
	signedNetTracker      onlineTracker
	midpointReturnTracker onlineTracker
	historyPoints         [][2]float64
	historyDistances      []float64
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

	signal.mu.Lock()
	defer signal.mu.Unlock()

	key := prior.Label + "@" + strconv.FormatInt(prior.Epoch, 10)
	state, found := signal.states[key]

	if !found {
		state = &symbolState{
			firstAtNano: float64(prior.At.UnixNano()),
		}
		signal.states[key] = state
	}

	var timeDelta float64

	if !prior.At.Equal(prior.From) && prior.At.After(prior.From) {
		timeDelta = prior.At.Sub(prior.From).Seconds()
	}

	if timeDelta <= 0 && !state.lastAt.IsZero() && prior.At.After(state.lastAt) {
		timeDelta = prior.At.Sub(state.lastAt).Seconds()
	}

	if timeDelta <= 0 {
		timeDelta = 1.0
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

	var signedCountFraction float64
	
	if state.totalCount > 0 {
		signedCountFraction = (state.buyCount - state.sellCount) / state.totalCount
	}

	var signedNetFraction float64
	
	if grossNotional > 0 {
		signedNetFraction = netNotional / grossNotional
	}

	var meanTradeNotional float64
	
	if state.totalCount > 0 {
		meanTradeNotional = grossNotional / state.totalCount
	}

	tradeRate := 1.0 / timeDelta
	grossNotionalRate := tradeGrossNotional / timeDelta
	netNotionalRate := tradeNetNotional / timeDelta
	buyNotionalRate := tradeBuyNotional / timeDelta
	sellNotionalRate := tradeSellNotional / timeDelta

	midpointFrom := price
	
	if state.prevPrice > 0 {
		midpointFrom = state.prevPrice
	}
	
	midpointAt := price

	var midpointLogReturn float64
	
	if state.prevPrice > 0 && price > 0 {
		midpointLogReturn = math.Log(price / state.prevPrice)
	}
	
	midpointReturnRate := midpointLogReturn / timeDelta

	flowAlignedMidpointReturn := midpointLogReturn
	
	if side == "sell" {
		flowAlignedMidpointReturn = -midpointLogReturn
	}

	var midpointResponsePerNetNotional float64
	
	if tradeNetNotional != 0 {
		midpointResponsePerNetNotional = midpointLogReturn / tradeNetNotional
	}

	state.grossRateTracker.Step(grossNotionalRate)
	grossNotionalRateBaseline := state.grossRateTracker.Baseline()
	grossNotionalRateRatio := state.grossRateTracker.Ratio(grossNotionalRate)
	grossNotionalRateDivergence := state.grossRateTracker.Divergence(grossNotionalRate)
	grossNotionalRateZScore := state.grossRateTracker.ZScore(grossNotionalRate)

	state.signedNetTracker.Step(signedNetFraction)
	signedNetFractionBaseline := state.signedNetTracker.Baseline()
	signedNetFractionDivergence := state.signedNetTracker.Divergence(signedNetFraction)
	signedNetFractionZScore := state.signedNetTracker.ZScore(signedNetFraction)

	state.midpointReturnTracker.Step(midpointReturnRate)
	midpointReturnRateBaseline := state.midpointReturnTracker.Baseline()
	midpointReturnRateDivergence := state.midpointReturnTracker.Divergence(midpointReturnRate)
	midpointReturnRateZScore := state.midpointReturnTracker.ZScore(midpointReturnRate)

	var netNotionalRateVelocity, grossNotionalRateVelocity float64
	
	if !state.lastAt.IsZero() && timeDelta > 0 {
		netNotionalRateVelocity = (netNotionalRate - state.prevNetNotionalRate) / timeDelta
		grossNotionalRateVelocity = (grossNotionalRate - state.prevGrossNotionalRate) / timeDelta
	}

	target := [2]float64{signedNetFractionZScore, grossNotionalRateZScore}
	histDist := 0.0
	histPerc := 0.0

	if len(state.historyPoints) > 0 {
		diffX := target[0] - state.historyPoints[0][0]
		diffY := target[1] - state.historyPoints[0][1]
		minDist := math.Sqrt(diffX*diffX + diffY*diffY)

		for idx := 1; idx < len(state.historyPoints); idx++ {
			dx := target[0] - state.historyPoints[idx][0]
			dy := target[1] - state.historyPoints[idx][1]
			distance := math.Sqrt(dx*dx + dy*dy)
			
			if distance < minDist {
				minDist = distance
			}
		}

		if len(state.historyDistances) > 0 {
			belowCount := 0
			
			for _, pastDist := range state.historyDistances {
				if pastDist <= minDist {
					belowCount++
				}
			}

			histPerc = float64(belowCount) / float64(len(state.historyDistances))
		}

		histDist = minDist
		state.historyDistances = append(state.historyDistances, minDist)
	
		if len(state.historyDistances) > 256 {
			state.historyDistances = state.historyDistances[len(state.historyDistances)-256:]
		}
	}

	state.historyPoints = append(state.historyPoints, target)

	if len(state.historyPoints) > 256 {
		state.historyPoints = state.historyPoints[len(state.historyPoints)-256:]
	}

	state.lastAt = prior.At
	state.prevPrice = price
	state.prevNetNotionalRate = netNotionalRate
	state.prevGrossNotionalRate = grossNotionalRate

	return prior.Next(signal.Name(), map[string]float64{
		"trade_count:buy":                    state.buyCount,
		"trade_count:sell":                   state.sellCount,
		"trade_count":                        state.totalCount,
		"signed_count_fraction":              signedCountFraction,
		"executed_quantity:buy":              state.buyQty,
		"executed_quantity:sell":             state.sellQty,
		"gross_executed_quantity":            grossExecutedQty,
		"net_executed_quantity":              netExecutedQty,
		"cumulative_volume_delta":            cumulativeVolumeDelta,
		"aggressive_notional:buy":            state.buyNotional,
		"aggressive_notional:sell":           state.sellNotional,
		"gross_notional":                     grossNotional,
		"net_notional":                       netNotional,
		"cumulative_notional_delta":          cumulativeNotionalDelta,
		"signed_net_fraction":                signedNetFraction,
		"mean_trade_notional":                meanTradeNotional,
		"trade_rate":                         tradeRate,
		"gross_notional_rate":                grossNotionalRate,
		"net_notional_rate":                  netNotionalRate,
		"buy_notional_rate":                  buyNotionalRate,
		"sell_notional_rate":                 sellNotionalRate,
		"cvd_epoch_from":                     state.firstAtNano,
		"response_midpoint:from":             midpointFrom,
		"response_midpoint:at":               midpointAt,
		"midpoint_log_return":                midpointLogReturn,
		"midpoint_return_rate":               midpointReturnRate,
		"flow_aligned_midpoint_return":       flowAlignedMidpointReturn,
		"midpoint_response_per_net_notional": midpointResponsePerNetNotional,
		"gross_notional_rate_baseline":       grossNotionalRateBaseline,
		"gross_notional_rate_ratio":          grossNotionalRateRatio,
		"gross_notional_rate_divergence":     grossNotionalRateDivergence,
		"gross_notional_rate_zscore":         grossNotionalRateZScore,
		"signed_net_fraction_baseline":       signedNetFractionBaseline,
		"signed_net_fraction_divergence":     signedNetFractionDivergence,
		"signed_net_fraction_zscore":         signedNetFractionZScore,
		"midpoint_return_rate_baseline":      midpointReturnRateBaseline,
		"midpoint_return_rate_divergence":    midpointReturnRateDivergence,
		"midpoint_return_rate_zscore":        midpointReturnRateZScore,
		"net_notional_rate_velocity":         netNotionalRateVelocity,
		"gross_notional_rate_velocity":       grossNotionalRateVelocity,
		"historical_path_distance":           histDist,
		"historical_path_percentile":         histPerc,
	})
}
