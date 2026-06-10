package exhaust

import (
	"context"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric/adaptive"
	floatring "github.com/theapemachine/symm/ring"
	"github.com/theapemachine/symm/signal"
)

type System struct {
	base *signal.System
}

var exhaustSection *crossSection

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	capacity := viper.GetInt("signals.exhaust.history_capacity")

	if capacity <= 0 {
		capacity = 24
	}

	exhaustSection = newCrossSection(capacity)

	base := signal.NewSystem(
		ctx,
		pool,
		logic.SourceExhaustion,
		func(symbol string, entity *logic.Entity) market.Signal {
			return NewSignal(symbol, entity)
		},
	)

	if base == nil {
		return nil
	}

	return &System{base: base}
}

func (system *System) Tick() error {
	return system.base.Tick()
}

func (system *System) Close() error {
	return system.base.Close()
}

type crossSection struct {
	universe sync.Map
	capacity int
}

type featureState struct {
	snapshot    *krakenmarket.BookUpdate
	bidDepths   floatring.FloatRing
	askDepths   floatring.FloatRing
	densities   floatring.FloatRing
	spreads     floatring.FloatRing
	pressures   floatring.FloatRing
	imbalances  floatring.FloatRing
	pressureEMA *adaptive.EMA
	lastPrice   float64
}

func newCrossSection(capacity int) *crossSection {
	return &crossSection{capacity: capacity}
}

func (crossSection *crossSection) ensure(symbol string) *featureState {
	raw, _ := crossSection.universe.LoadOrStore(symbol, &featureState{
		bidDepths:   floatring.NewFloatRing(crossSection.capacity),
		askDepths:   floatring.NewFloatRing(crossSection.capacity),
		densities:   floatring.NewFloatRing(crossSection.capacity),
		spreads:     floatring.NewFloatRing(crossSection.capacity),
		pressures:   floatring.NewFloatRing(crossSection.capacity),
		imbalances:  floatring.NewFloatRing(crossSection.capacity),
		pressureEMA: adaptive.NewEMA(0),
	})

	state, ok := raw.(*featureState)

	if !ok {
		return nil
	}

	return state
}

func (crossSection *crossSection) observeBook(symbol string, book *krakenmarket.BookUpdate) {
	if book == nil {
		return
	}

	state := crossSection.ensure(symbol)

	if state == nil {
		return
	}

	if book.Type == "snapshot" {
		state.snapshot = book
	}

	if state.snapshot == nil {
		return
	}

	bids := book.Bids
	asks := book.Asks

	if len(bids) == 0 {
		bids = state.snapshot.Bids
	}

	if len(asks) == 0 {
		asks = state.snapshot.Asks
	}

	if len(bids) == 0 || len(asks) == 0 {
		return
	}

	bidDepth := crossSection.sideDepth(bids)
	askDepth := crossSection.sideDepth(asks)

	midPrice := (bids[0].Price + asks[0].Price) / 2
	touchSpread := asks[0].Price - bids[0].Price
	depth := bids[0].Qty + asks[0].Qty

	if bidDepth > 0 {
		state.bidDepths.Push(bidDepth)
	}

	if askDepth > 0 {
		state.askDepths.Push(askDepth)
	}

	if depth > 0 {
		state.densities.Push(depth)
	}

	spreadBPS := (touchSpread / midPrice) * 10000

	if spreadBPS > 0 {
		state.spreads.Push(spreadBPS)
	}

	imbalance, imbalanceOK := crossSection.level1Imbalance(bids, asks)

	if imbalanceOK {
		state.imbalances.Push(imbalance)
	}

	state.lastPrice = midPrice
}

func (crossSection *crossSection) observeTrade(symbol string, trade *krakenmarket.TradeUpdate) {
	if trade == nil {
		return
	}

	state := crossSection.ensure(symbol)

	if state == nil {
		return
	}

	sign := 0.0

	if trade.Side == "buy" {
		sign = 1
	}

	if trade.Side == "sell" {
		sign = -1
	}

	if sign == 0 {
		return
	}

	smoothed, err := state.pressureEMA.Next(0, sign)

	if errnie.Error(err) != nil {
		return
	}

	state.pressures.Push(smoothed)

	if trade.Price > 0 {
		state.lastPrice = trade.Price
	}
}

func (crossSection *crossSection) observeTick(symbol string, ticker *krakenmarket.TickerUpdate) {
	if ticker == nil {
		return
	}

	state := crossSection.ensure(symbol)

	if state == nil {
		return
	}

	mid := ticker.Last

	if mid <= 0 {
		mid = (ticker.Ask + ticker.Bid) / 2
	}

	spread := ticker.Ask - ticker.Bid

	if mid > 0 && spread > 0 {
		state.spreads.Push((spread / mid) * 10000)
	}

	if mid > 0 {
		state.lastPrice = mid
	}
}

func (crossSection *crossSection) snapshot(symbol string) (featureState, bool) {
	raw, ok := crossSection.universe.Load(symbol)

	if !ok {
		return featureState{}, false
	}

	state, ok := raw.(*featureState)

	if !ok || state.lastPrice <= 0 {
		return featureState{}, false
	}

	return *state, true
}

func (crossSection *crossSection) sideDepth(levels []krakenmarket.BookLevel) float64 {
	depth := 0.0

	for _, level := range levels {
		depth += level.Qty
	}

	return depth
}

func (crossSection *crossSection) level1Imbalance(
	bids, asks []krakenmarket.BookLevel,
) (float64, bool) {
	if len(bids) == 0 || len(asks) == 0 {
		return 0, false
	}

	total := bids[0].Qty + asks[0].Qty

	if total <= 0 {
		return 0, false
	}

	return (bids[0].Qty - asks[0].Qty) / total, true
}
