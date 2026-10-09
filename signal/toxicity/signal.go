package toxicity

import (
	"context"
	"fmt"
	"math"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type welfordBaseline struct {
	count float64
	mean  float64
	m2    float64
}

func (wb *welfordBaseline) Step(value float64) (float64, float64) {
	priorCount := wb.count
	priorMean := wb.mean

	wb.count++
	delta := value - wb.mean
	wb.mean += delta / wb.count
	wb.m2 += delta * (value - wb.mean)

	center := value

	if priorCount > 0 {
		center = priorMean
	}

	var scale float64

	if wb.count > 1 {
		variance := wb.m2 / (wb.count - 1)
		if variance > 0 {
			scale = math.Sqrt(variance)
		}
	}

	return center, scale
}

type velocityTracker struct {
	hasPrev   bool
	prevVal   float64
	prevAtSec float64
}

func (vt *velocityTracker) Step(value float64, atSec float64) float64 {
	if !vt.hasPrev {
		vt.hasPrev = true
		vt.prevVal = value
		vt.prevAtSec = atSec
		return 0.0
	}

	dt := atSec - vt.prevAtSec
	vt.prevAtSec = atSec
	diff := value - vt.prevVal
	vt.prevVal = value

	if dt <= 0 {
		return 0.0
	}

	return diff / dt
}

type symbolState struct {
	hasPrev                      bool
	prevBid                      float64
	prevAsk                      float64
	prevBidQty                   float64
	prevAskQty                   float64
	prevAtNano                   float64
	cumBracketTradeQty           float64
	cumFillBid                   float64
	cumFillAsk                   float64
	bracketStartAtNano           float64
	fillFracBidBaseline          welfordBaseline
	fillFracAskBaseline          welfordBaseline
	withdrawalFracBidBaseline    welfordBaseline
	withdrawalFracAskBaseline    welfordBaseline
	retreatFracBidBaseline       welfordBaseline
	retreatFracAskBaseline       welfordBaseline
	replenishmentFracBidBaseline welfordBaseline
	replenishmentFracAskBaseline welfordBaseline
	fillFracBidVel               velocityTracker
	fillFracAskVel               velocityTracker
	withdrawalFracBidVel         velocityTracker
	withdrawalFracAskVel         velocityTracker
	historyPoints                [][6]float64
	historyDistances             []float64
}

type Signal struct {
	*runtime.System
	books  broker.BookSource
	states map[string]*symbolState
}

func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books:  books,
		states: make(map[string]*symbolState),
	}

	signal.System = runtime.NewSystem(ctx, "toxicity", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[toxicity] book manager is required", err))
	}

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

	if signal.books == nil {
		signal.Error(errnie.Err(errnie.Internal, "[toxicity] book manager is required", nil))
		return nil
	}

	price, err := tradeValue(prior, "price")
	if err != nil {
		signal.Error(err)
		return nil
	}

	qty, err := tradeValue(prior, "qty")
	if err != nil {
		signal.Error(err)
		return nil
	}

	if price <= 0 || qty <= 0 {
		return nil
	}

	var bid, ask, bidQty, askQty float64
	var found bool

	signal.books.Book(prior.Label, func(book *spotbook.Book) {
		if book == nil {
			return
		}

		bestBid := book.BestBid()
		bestAsk := book.BestAsk()

		if bestBid == nil || bestAsk == nil || bestBid.Price == nil || bestAsk.Price == nil ||
			bestBid.Quantity == nil || bestAsk.Quantity == nil {
			return
		}

		bid = kraken.Float64(bestBid.Price)
		bidQty = kraken.Float64(bestBid.Quantity)
		ask = kraken.Float64(bestAsk.Price)
		askQty = kraken.Float64(bestAsk.Quantity)
		found = true
	})

	if !found {
		return nil
	}

	touch := map[string]float64{
		"bid":     bid,
		"ask":     ask,
		"bid_qty": bidQty,
		"ask_qty": askQty,
	}

	for _, value := range touch {
		if !broker.ValidTouchValue(value) {
			signal.Error(broker.InvalidTouch("toxicity", prior.Label, touch))
			return nil
		}
	}

	if ask <= bid {
		errnie.Warn(fmt.Sprintf(
			"[toxicity] dropping frame for %s with crossed book: bid=%v ask=%v",
			prior.Label, bid, ask,
		))

		return nil
	}

	sideIndicator := 0.0
	side := prior.Meta("side")

	if side == "buy" {
		sideIndicator = 1.0
	}

	if side == "sell" {
		sideIndicator = -1.0
	}

	atNano := float64(prior.At.UnixNano())
	atSec := atNano / 1e9

	state, exists := signal.states[prior.Label]

	if !exists {
		state = &symbolState{}
		signal.states[prior.Label] = state
	}

	inBracket := price >= bid && price <= ask
	var matchedBid, matchedAsk float64

	if price == ask && sideIndicator > 0 {
		matchedAsk = qty
	}

	if price == bid && sideIndicator < 0 {
		matchedBid = qty
	}

	if inBracket {
		state.cumBracketTradeQty += qty
		state.cumFillBid += matchedBid
		state.cumFillAsk += matchedAsk

		if state.bracketStartAtNano == 0 {
			state.bracketStartAtNano = atNano
		}
	}

	var touchFillFracBid, touchFillFracAsk float64

	if inBracket {
		if bidQty > 0 {
			touchFillFracBid = state.cumFillBid / bidQty
		}

		if askQty > 0 {
			touchFillFracAsk = state.cumFillAsk / askQty
		}
	}

	var bracketTimeDeltaSec float64

	if state.bracketStartAtNano > 0 && atNano > state.bracketStartAtNano {
		bracketTimeDeltaSec = (atNano - state.bracketStartAtNano) / 1e9
	}

	var touchFillRateBid, touchFillRateAsk float64

	if bracketTimeDeltaSec > 0 {
		touchFillRateBid = state.cumFillBid / bracketTimeDeltaSec
		touchFillRateAsk = state.cumFillAsk / bracketTimeDeltaSec
	}

	unfilledBid := bidQty
	unfilledAsk := askQty

	if state.hasPrev {
		unfilledBid = math.Max(state.prevBidQty-matchedBid, 0.0)
		unfilledAsk = math.Max(state.prevAskQty-matchedAsk, 0.0)
	}

	var prevBid, prevAsk, prevBidQty, prevAskQty float64
	var retreatedBid, retreatedAsk, retreatFracBid, retreatFracAsk, retreatRateBid, retreatRateAsk float64
	var netWithdrawnBid, netWithdrawnAsk, netWithdrawalFracBid, netWithdrawalFracAsk, netWithdrawalRateBid, netWithdrawalRateAsk float64
	var netReplenishedBid, netReplenishedAsk, netReplenishmentFracBid, netReplenishmentFracAsk, netReplenishmentRateBid, netReplenishmentRateAsk float64
	var logChangeBid, logChangeAsk float64

	if state.hasPrev {
		prevBid = state.prevBid
		prevAsk = state.prevAsk
		prevBidQty = state.prevBidQty
		prevAskQty = state.prevAskQty

		var stepDeltaSec float64

		if atNano > state.prevAtNano {
			stepDeltaSec = (atNano - state.prevAtNano) / 1e9
		}

		if prevBid > 0 && bid > 0 {
			logChangeBid = math.Log(bid / prevBid)
		}

		if prevAsk > 0 && ask > 0 {
			logChangeAsk = math.Log(ask / prevAsk)
		}

		if bid < prevBid {
			retreatedBid = state.prevBidQty
			retreatFracBid = 1.0

			if stepDeltaSec > 0 {
				retreatRateBid = retreatedBid / stepDeltaSec
			}
		}

		if bid == prevBid {
			if bidQty < unfilledBid {
				netWithdrawnBid = unfilledBid - bidQty

				if state.prevBidQty > 0 {
					netWithdrawalFracBid = netWithdrawnBid / state.prevBidQty
				}

				if stepDeltaSec > 0 {
					netWithdrawalRateBid = netWithdrawnBid / stepDeltaSec
				}
			}

			if bidQty > unfilledBid {
				netReplenishedBid = bidQty - unfilledBid

				if state.prevBidQty > 0 {
					netReplenishmentFracBid = netReplenishedBid / state.prevBidQty
				}

				if stepDeltaSec > 0 {
					netReplenishmentRateBid = netReplenishedBid / stepDeltaSec
				}
			}
		}

		if ask > prevAsk {
			retreatedAsk = state.prevAskQty
			retreatFracAsk = 1.0

			if stepDeltaSec > 0 {
				retreatRateAsk = retreatedAsk / stepDeltaSec
			}
		}

		if ask == prevAsk {
			if askQty < unfilledAsk {
				netWithdrawnAsk = unfilledAsk - askQty

				if state.prevAskQty > 0 {
					netWithdrawalFracAsk = netWithdrawnAsk / state.prevAskQty
				}

				if stepDeltaSec > 0 {
					netWithdrawalRateAsk = netWithdrawnAsk / stepDeltaSec
				}
			}

			if askQty > unfilledAsk {
				netReplenishedAsk = askQty - unfilledAsk

				if state.prevAskQty > 0 {
					netReplenishmentFracAsk = netReplenishedAsk / state.prevAskQty
				}

				if stepDeltaSec > 0 {
					netReplenishmentRateAsk = netReplenishedAsk / stepDeltaSec
				}
			}
		}
	}

	state.hasPrev = true
	state.prevBid = bid
	state.prevAsk = ask
	state.prevBidQty = bidQty
	state.prevAskQty = askQty
	state.prevAtNano = atNano

	fillFracBidCenter, fillFracBidScale := state.fillFracBidBaseline.Step(touchFillFracBid)
	fillFracAskCenter, fillFracAskScale := state.fillFracAskBaseline.Step(touchFillFracAsk)
	withdrawalFracBidCenter, withdrawalFracBidScale := state.withdrawalFracBidBaseline.Step(netWithdrawalFracBid)
	withdrawalFracAskCenter, withdrawalFracAskScale := state.withdrawalFracAskBaseline.Step(netWithdrawalFracAsk)
	retreatFracBidCenter, retreatFracBidScale := state.retreatFracBidBaseline.Step(retreatFracBid)
	retreatFracAskCenter, retreatFracAskScale := state.retreatFracAskBaseline.Step(retreatFracAsk)
	replenishmentFracBidCenter, _ := state.replenishmentFracBidBaseline.Step(netReplenishmentFracBid)
	replenishmentFracAskCenter, _ := state.replenishmentFracAskBaseline.Step(netReplenishmentFracAsk)

	fillFracBidDiv := touchFillFracBid - fillFracBidCenter
	fillFracAskDiv := touchFillFracAsk - fillFracAskCenter
	withdrawalFracBidDiv := netWithdrawalFracBid - withdrawalFracBidCenter
	withdrawalFracAskDiv := netWithdrawalFracAsk - withdrawalFracAskCenter

	var fillFracBidZ, fillFracAskZ, withdrawalFracBidZ, withdrawalFracAskZ, retreatFracBidZ, retreatFracAskZ float64

	if fillFracBidScale > 0 {
		fillFracBidZ = fillFracBidDiv / fillFracBidScale
	}

	if fillFracAskScale > 0 {
		fillFracAskZ = fillFracAskDiv / fillFracAskScale
	}

	if withdrawalFracBidScale > 0 {
		withdrawalFracBidZ = withdrawalFracBidDiv / withdrawalFracBidScale
	}

	if withdrawalFracAskScale > 0 {
		withdrawalFracAskZ = withdrawalFracAskDiv / withdrawalFracAskScale
	}

	if retreatFracBidScale > 0 {
		retreatFracBidZ = retreatFracBid / retreatFracBidScale
	}

	if retreatFracAskScale > 0 {
		retreatFracAskZ = retreatFracAsk / retreatFracAskScale
	}

	fillFracBidVelVal := state.fillFracBidVel.Step(touchFillFracBid, atSec)
	fillFracAskVelVal := state.fillFracAskVel.Step(touchFillFracAsk, atSec)
	withdrawalFracBidVelVal := state.withdrawalFracBidVel.Step(netWithdrawalFracBid, atSec)
	withdrawalFracAskVelVal := state.withdrawalFracAskVel.Step(netWithdrawalFracAsk, atSec)

	target := [6]float64{
		fillFracBidZ,
		fillFracAskZ,
		withdrawalFracBidZ,
		withdrawalFracAskZ,
		retreatFracBidZ,
		retreatFracAskZ,
	}
	var histDist, histPerc float64

	if len(state.historyPoints) > 0 {
		var firstSumSq float64

		for dim := 0; dim < len(target); dim++ {
			diff := target[dim] - state.historyPoints[0][dim]
			firstSumSq += diff * diff
		}

		minDist := math.Sqrt(firstSumSq)

		for idx := 1; idx < len(state.historyPoints); idx++ {
			var sumSq float64

			for dim := 0; dim < len(target); dim++ {
				diff := target[dim] - state.historyPoints[idx][dim]
				sumSq += diff * diff
			}

			distance := math.Sqrt(sumSq)

			if distance < minDist {
				minDist = distance
			}
		}

		var belowCount int

		for _, pastDist := range state.historyDistances {
			if pastDist <= minDist {
				belowCount++
			}
		}

		if len(state.historyDistances) > 0 {
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

	bracketStart := state.bracketStartAtNano

	out := prior.Next(signal.Name(), map[string]float64{
		"best_price:bid":                      bid,
		"best_price:ask":                      ask,
		"touch_quantity:bid":                  bidQty,
		"touch_quantity:ask":                  askQty,
		"unfilled_residual_quantity:bid":      unfilledBid,
		"unfilled_residual_quantity:ask":      unfilledAsk,
		"bracket_trade_quantity":              state.cumBracketTradeQty,
		"matched_touch_trade_quantity:bid":    matchedBid,
		"matched_touch_trade_quantity:ask":    matchedAsk,
		"touch_fill_quantity:bid":             state.cumFillBid,
		"touch_fill_quantity:ask":             state.cumFillAsk,
		"touch_fill_fraction:bid":             touchFillFracBid,
		"touch_fill_fraction:ask":             touchFillFracAsk,
		"fill_fraction_baseline:bid":          fillFracBidCenter,
		"fill_fraction_baseline:ask":          fillFracAskCenter,
		"fill_fraction_divergence:bid":        fillFracBidDiv,
		"fill_fraction_divergence:ask":        fillFracAskDiv,
		"fill_fraction_zscore:bid":            fillFracBidZ,
		"fill_fraction_zscore:ask":            fillFracAskZ,
		"fill_fraction_velocity:bid":          fillFracBidVelVal,
		"fill_fraction_velocity:ask":          fillFracAskVelVal,
		"previous_best_price:bid":             prevBid,
		"previous_best_price:ask":             prevAsk,
		"previous_touch_quantity:bid":         prevBidQty,
		"previous_touch_quantity:ask":         prevAskQty,
		"touch_price_log_change:bid":          logChangeBid,
		"touch_price_log_change:ask":          logChangeAsk,
		"retreated_quantity:bid":              retreatedBid,
		"retreated_quantity:ask":              retreatedAsk,
		"retreat_fraction:bid":                retreatFracBid,
		"retreat_fraction:ask":                retreatFracAsk,
		"retreat_rate:bid":                    retreatRateBid,
		"retreat_rate:ask":                    retreatRateAsk,
		"net_withdrawn_quantity:bid":          netWithdrawnBid,
		"net_withdrawn_quantity:ask":          netWithdrawnAsk,
		"net_withdrawal_fraction:bid":         netWithdrawalFracBid,
		"net_withdrawal_fraction:ask":         netWithdrawalFracAsk,
		"net_withdrawal_rate:bid":             netWithdrawalRateBid,
		"net_withdrawal_rate:ask":             netWithdrawalRateAsk,
		"net_replenished_quantity:bid":        netReplenishedBid,
		"net_replenished_quantity:ask":        netReplenishedAsk,
		"net_replenishment_fraction:bid":      netReplenishmentFracBid,
		"net_replenishment_fraction:ask":      netReplenishmentFracAsk,
		"net_replenishment_rate:bid":          netReplenishmentRateBid,
		"net_replenishment_rate:ask":          netReplenishmentRateAsk,
		"touch_fill_rate:bid":                 touchFillRateBid,
		"touch_fill_rate:ask":                 touchFillRateAsk,
		"withdrawal_fraction_baseline:bid":    withdrawalFracBidCenter,
		"withdrawal_fraction_baseline:ask":    withdrawalFracAskCenter,
		"withdrawal_fraction_divergence:bid":  withdrawalFracBidDiv,
		"withdrawal_fraction_divergence:ask":  withdrawalFracAskDiv,
		"withdrawal_fraction_zscore:bid":      withdrawalFracBidZ,
		"withdrawal_fraction_zscore:ask":      withdrawalFracAskZ,
		"withdrawal_fraction_velocity:bid":    withdrawalFracBidVelVal,
		"withdrawal_fraction_velocity:ask":    withdrawalFracAskVelVal,
		"retreat_fraction_baseline:bid":       retreatFracBidCenter,
		"retreat_fraction_baseline:ask":       retreatFracAskCenter,
		"retreat_fraction_zscore:bid":         retreatFracBidZ,
		"retreat_fraction_zscore:ask":         retreatFracAskZ,
		"replenishment_fraction_baseline:bid": replenishmentFracBidCenter,
		"replenishment_fraction_baseline:ask": replenishmentFracAskCenter,
		"historical_path_distance":            histDist,
		"historical_path_percentile":          histPerc,
	})

	if bracketStart > 0 {
		out.From = time.Unix(0, int64(bracketStart))
	}

	return out
}

func tradeValue(prior *data.Measurement, key string) (float64, error) {
	entry := data.Pull(prior.Read(key))

	if entry != nil && entry.Err != nil {
		return 0, entry.Err
	}

	if entry == nil || entry.Metric == nil || entry.Metric.Label != key {
		return 0, errnie.Err(
			errnie.NotAcceptable, "[toxicity] trade frame is missing "+key, nil,
		)
	}

	return entry.Metric.Raw, nil
}
