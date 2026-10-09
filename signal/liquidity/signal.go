package liquidity

import (
	"context"
	"fmt"
	"math"
	"sync"

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
	bidBaseline      welfordBaseline
	askBaseline      welfordBaseline
	spreadBaseline   welfordBaseline
	bidVel           velocityTracker
	askVel           velocityTracker
	spreadVel        velocityTracker
	historyPoints    [][2]float64
	historyDistances []float64
}

type Signal struct {
	*runtime.System
	books  broker.BookSource
	mu     sync.Mutex
	states map[string]*symbolState
}

func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books:  books,
		states: make(map[string]*symbolState),
	}

	signal.System = runtime.NewSystem(ctx, "liquidity", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[liquidity] book manager is required", err))
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
		signal.Error(errnie.Err(errnie.Internal, "[liquidity] book manager is required", nil))
		return nil
	}

	var bid, ask, bidQty, askQty float64
	var found bool

	signal.books.Book(prior.Label, func(book *spotbook.Book) {
		if book == nil {
			return
		}

		b := book.BestBid()
		a := book.BestAsk()

		if b == nil || a == nil || b.Price == nil || a.Price == nil ||
			b.Quantity == nil || a.Quantity == nil {
			return
		}

		bid = kraken.Float64(b.Price)
		bidQty = kraken.Float64(b.Quantity)
		ask = kraken.Float64(a.Price)
		askQty = kraken.Float64(a.Quantity)
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
			signal.Error(broker.InvalidTouch("liquidity", prior.Label, touch))
			return nil
		}
	}

	if ask <= bid {
		errnie.Warn(fmt.Sprintf("[liquidity] dropping frame for %s with crossed book: bid=%v ask=%v", prior.Label, bid, ask))
		return nil
	}

	if bidQty <= 0 || askQty <= 0 {
		return nil
	}

	atSec := float64(prior.At.UnixNano()) * 1e-9

	signal.mu.Lock()
	defer signal.mu.Unlock()

	state, exists := signal.states[prior.Label]
	if !exists {
		state = &symbolState{}
		signal.states[prior.Label] = state
	}

	bidNotional := bid * bidQty
	askNotional := ask * askQty
	midpoint := (bid + ask) / 2.0
	spread := ask - bid
	relativeSpread := spread / midpoint
	twoSidedNotional := math.Min(bidNotional, askNotional)
	total := bidNotional + askNotional
	imbalance := (bidNotional - askNotional) / total

	bidCenter, bidScale := state.bidBaseline.Step(bidNotional)
	askCenter, askScale := state.askBaseline.Step(askNotional)
	spreadCenter, spreadScale := state.spreadBaseline.Step(relativeSpread)

	bidDiv := bidNotional - bidCenter
	askDiv := askNotional - askCenter
	spreadDiv := relativeSpread - spreadCenter

	var bidRatio, askRatio, spreadRatio float64
	if bidCenter > 0 {
		bidRatio = bidNotional / bidCenter
	}

	if askCenter > 0 {
		askRatio = askNotional / askCenter
	}

	if spreadCenter > 0 {
		spreadRatio = relativeSpread / spreadCenter
	}

	var bidZ, askZ, spreadZ float64
	if bidScale > 0 {
		bidZ = bidDiv / bidScale
	}

	if askScale > 0 {
		askZ = askDiv / askScale
	}

	if spreadScale > 0 {
		spreadZ = spreadDiv / spreadScale
	}

	bidVelVal := state.bidVel.Step(bidDiv, atSec)
	askVelVal := state.askVel.Step(askDiv, atSec)
	spreadVelVal := state.spreadVel.Step(spreadDiv, atSec)

	target := [2]float64{spreadZ, imbalance}
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

	return prior.Next(signal.Name(), map[string]float64{
		"best_bid_price":              bid,
		"best_ask_price":              ask,
		"touch_quantity:bid":          bidQty,
		"touch_quantity:ask":          askQty,
		"touch_notional:bid":          bidNotional,
		"touch_notional:ask":          askNotional,
		"midpoint":                    midpoint,
		"spread":                      spread,
		"relative_spread":             relativeSpread,
		"two_sided_touch_notional":    twoSidedNotional,
		"touch_notional_imbalance":    imbalance,
		"touch_notional_baseline:bid": bidCenter,
		"touch_notional_baseline:ask": askCenter,
		"relative_spread_baseline":    spreadCenter,
		"depth_ratio:bid":             bidRatio,
		"depth_ratio:ask":             askRatio,
		"spread_ratio":                spreadRatio,
		"depth_divergence:bid":        bidDiv,
		"depth_divergence:ask":        askDiv,
		"spread_divergence":           spreadDiv,
		"depth_noise_scale:bid":       bidScale,
		"depth_noise_scale:ask":       askScale,
		"spread_noise_scale":          spreadScale,
		"depth_zscore:bid":            bidZ,
		"depth_zscore:ask":            askZ,
		"spread_zscore":               spreadZ,
		"divergence_velocity:bid":     bidVelVal,
		"divergence_velocity:ask":     askVelVal,
		"spread_divergence_velocity":  spreadVelVal,
		"historical_path_distance":    histDist,
		"historical_path_percentile":  histPerc,
	})
}
