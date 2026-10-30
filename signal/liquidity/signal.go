package liquidity

import (
	"context"
	"math"
	"slices"
	"sync"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	parent "github.com/theapemachine/symm/signal"
)

type symbolState struct {
	spreadEMA             float64
	peakMid               float64
	prevMid               float64
	hasMid                bool
	midReturns            []float64
	lastSpreadExpansion   float64
	lastLiquidationImpact float64
}

type Signal struct {
	*runtime.System
	books  *broker.Book
	price  *broker.Price
	states *sync.Map
}

func NewSignal(ctx context.Context, books *broker.Book, price *broker.Price) *Signal {
	signal := &Signal{
		books:  books,
		price:  price,
		states: &sync.Map{},
	}

	signal.System = runtime.NewSystem(ctx, "liquidity", signal)
	return signal
}

/*
Step publishes per-symbol spread expansion and liquidation impact, then
board-wide friction, drag ratio, and capacity oracles.
*/
func (signal *Signal) Step(value *data.Value) *data.Value {
	if signal.Status() != runtime.READY {
		return value
	}

	var trades []kraken.TradeData
	value.Lookup(&trades, "spot:trade")

	symbols := parent.DistinctSymbols(trades)
	spreads := make([]float64, 0, len(symbols))
	dragRatios := make([]float64, 0, len(symbols))

	for _, symbol := range symbols {
		if spreadExp, ok := signal.spreadExpansion(symbol); ok {
			spreads = append(spreads, spreadExp.spread)
			signal.appendSymbol(value, symbol, "spread_expansion", spreadExp.expansion)
		}

		if impact, ok := signal.liquidationImpact(symbol); ok {
			signal.appendSymbol(value, symbol, "liquidation_impact", impact)
		}

		if drag, ok := signal.frictionDragRatio(symbol); ok {
			dragRatios = append(dragRatios, drag)
		}
	}

	if len(spreads) == 0 {
		return value
	}

	slices.Sort(spreads)
	median := spreads[len(spreads)/2]
	if len(spreads)%2 == 0 {
		median = (spreads[len(spreads)/2-1] + spreads[len(spreads)/2]) / 2
	}

	worst := spreads[len(spreads)-1]

	value.Append([]float64{median}, append([]string{
		signal.Name(),
		"aggregate",
	}, "market_friction")...)

	value.Append([]float64{worst}, append([]string{
		signal.Name(),
		"aggregate",
	}, "capacity_stress")...)

	if drag, ok := parent.Median(dragRatios); ok {
		value.Append([]float64{drag}, append([]string{
			signal.Name(),
			"aggregate",
		}, "friction_drag_ratio")...)
	}

	audit.Record.Write(map[string]any{
		"signal": signal.Name(),
		"median": median,
		"worst":  worst,
	})

	return value
}

type spreadSample struct {
	spread     float64
	expansion  float64
}

func (signal *Signal) spreadExpansion(symbol string) (spreadSample, bool) {
	var sample spreadSample
	var ok bool

	signal.books.Book(symbol, func(b *spotbook.Book) {
		bestBid := b.BestBid()
		bestAsk := b.BestAsk()
		if bestBid == nil || bestAsk == nil || bestBid.Price == nil || bestAsk.Price == nil {
			return
		}

		bid := bestBid.Price.Float64()
		ask := bestAsk.Price.Float64()
		mid := (bid + ask) / 2
		if mid <= 0 || ask <= bid {
			return
		}

		spread := (ask - bid) / mid
		untypedState, _ := signal.states.LoadOrStore(symbol, &symbolState{})
		state := untypedState.(*symbolState)

		if state.hasMid && state.prevMid > 0 && mid > 0 {
			ret := math.Log(mid / state.prevMid)
			state.midReturns = append(state.midReturns, ret)
			if len(state.midReturns) > 64 {
				state.midReturns = state.midReturns[len(state.midReturns)-64:]
			}
		}
		state.prevMid = mid
		state.hasMid = true

		if state.spreadEMA <= 0 {
			state.spreadEMA = spread
		} else {
			state.spreadEMA = 0.94*state.spreadEMA + 0.06*spread
		}

		if mid > state.peakMid {
			state.peakMid = mid
		}

		expansion := 0.0
		if state.spreadEMA > 0 && mid >= 0.995*state.peakMid {
			expansion = math.Max(0, (spread-state.spreadEMA)/state.spreadEMA)
		}

		state.lastSpreadExpansion = expansion
		sample = spreadSample{spread: spread, expansion: expansion}
		ok = true
	})

	return sample, ok
}

func (signal *Signal) frictionDragRatio(symbol string) (float64, bool) {
	if signal.price == nil {
		return 0, false
	}

	friction, ok := signal.price.Friction(symbol)
	if !ok || friction == nil {
		return 0, false
	}

	vol := signal.realizedVol(symbol)
	if vol <= 0 {
		return 0, false
	}

	return friction.Float64() / vol, true
}

func (signal *Signal) realizedVol(symbol string) float64 {
	untypedState, ok := signal.states.Load(symbol)
	if !ok {
		return 0
	}

	state := untypedState.(*symbolState)
	if len(state.midReturns) < 2 {
		return 0
	}

	mean := 0.0
	for _, ret := range state.midReturns {
		mean += ret
	}
	mean /= float64(len(state.midReturns))

	variance := 0.0
	for _, ret := range state.midReturns {
		delta := ret - mean
		variance += delta * delta
	}
	variance /= float64(len(state.midReturns) - 1)

	return math.Sqrt(variance)
}

func (signal *Signal) liquidationImpact(symbol string) (float64, bool) {
	if signal.price == nil {
		return 0, false
	}

	qty, _ := signal.price.Allocate(symbol)
	if qty == nil || qty.Sign() <= 0 {
		return 0, false
	}

	exitVWAP := signal.price.ExitVWAP(symbol, qty)
	if exitVWAP == nil || exitVWAP.Sign() <= 0 {
		return 0, false
	}

	var impact float64
	var ok bool

	signal.books.Book(symbol, func(b *spotbook.Book) {
		bestBid := b.BestBid()
		if bestBid == nil || bestBid.Price == nil {
			return
		}

		bid := bestBid.Price.Float64()
		if bid <= 0 {
			return
		}

		impact = math.Abs(bid-exitVWAP.Float64()) / bid
		ok = true
	})

	if ok {
		untypedState, _ := signal.states.LoadOrStore(symbol, &symbolState{})
		untypedState.(*symbolState).lastLiquidationImpact = impact
	}

	return impact, ok
}

func (signal *Signal) appendSymbol(value *data.Value, symbol, label string, reading float64) {
	value.Append([]float64{reading}, append([]string{
		signal.Name(),
		symbol,
	}, label)...)
}
