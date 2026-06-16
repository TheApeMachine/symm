package signal

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/symm/kraken/market"
)

/*
TradeSnapshot holds input facts for a symbol: data the feed already knows.
*/
type TradeSnapshot struct {
	Price    float64
	Volume   float64
	Elapsed  float64
	Observed time.Time
}

/*
Trade stores scoped trade updates on a click clock.
OnUpdate runs after each accepted trade update.
*/
type Trade struct {
	ctx      context.Context
	cancel   context.CancelFunc
	Scope    string
	OnUpdate func(*market.TradeUpdate)
	symbols  *sync.Map
}

/*
NewTrade returns a trade feed backed by a per-symbol click clock.
*/
func NewTrade(ctx context.Context) *Trade {
	ctx, cancel := context.WithCancel(ctx)

	return &Trade{
		ctx:     ctx,
		cancel:  cancel,
		symbols: &sync.Map{},
	}
}

/*
TradeWindow holds one symbol's recent trade click-clock window.
*/
type TradeWindow struct {
	First   *market.TradeUpdate
	Latest  *market.TradeUpdate
	Prices  []float64
	Volume  float64
	Elapsed float64
}

/*
Window returns the scoped symbol's trade window.
*/
func (trade *Trade) Window(symbol string) (TradeWindow, bool) {
	value, ok := trade.symbols.Load(symbol)

	if !ok {
		return TradeWindow{}, false
	}

	ring := value.(*structure.ClockRing[*market.TradeUpdate])

	var window TradeWindow

	ring.Do(func(slot structure.ClockSlot[*market.TradeUpdate]) {
		update := slot.Payload

		if update == nil || update.Price <= 0 || update.Qty <= 0 {
			return
		}

		if window.First == nil {
			window.First = update
		}

		window.Latest = update
		window.Volume += update.Qty
		window.Prices = append(window.Prices, update.Price)
	})

	if window.Latest == nil || len(window.Prices) < 2 {
		return TradeWindow{}, false
	}

	if window.First != nil {
		window.Elapsed = window.Latest.Timestamp.Sub(window.First.Timestamp).Seconds()
	}

	return window, true
}

/*
Scan visits each trade update in the scoped symbol window.
*/
func (trade *Trade) Scan(symbol string, visit func(*market.TradeUpdate)) bool {
	value, ok := trade.symbols.Load(symbol)

	if !ok {
		return false
	}

	ring := value.(*structure.ClockRing[*market.TradeUpdate])

	ring.Do(func(slot structure.ClockSlot[*market.TradeUpdate]) {
		update := slot.Payload

		if update == nil {
			return
		}

		visit(update)
	})

	return true
}

func (trade *Trade) Update(update market.TradeUpdates) {
	for _, tradeUpdate := range update {
		if tradeUpdate == nil || tradeUpdate.Symbol == "" {
			continue
		}

		ring, _ := trade.symbols.LoadOrStore(
			tradeUpdate.Symbol, structure.NewClockRing[*market.TradeUpdate](
				10, 100, 1000,
				datura.Acquire(
					"trade", datura.Artifact_Type_json,
				).WithRole("trade"),
			),
		)

		ring.(*structure.ClockRing[*market.TradeUpdate]).ObserveSecond(
			tradeUpdate.Timestamp, tradeUpdate,
		)

		if trade.OnUpdate != nil {
			trade.OnUpdate(tradeUpdate)
		}
	}
}

func (trade *Trade) Read(buffer []byte) (int, error) {
	value, ok := trade.symbols.Load(trade.Scope)

	if !ok {
		return 0, io.EOF
	}

	ring := value.(*structure.ClockRing[*market.TradeUpdate])

	var (
		first  *market.TradeUpdate
		latest *market.TradeUpdate
		volume float64
	)

	ring.Do(func(slot structure.ClockSlot[*market.TradeUpdate]) {
		update := slot.Payload

		if update == nil {
			return
		}

		if first == nil {
			first = update
		}

		latest = update
		volume += update.Price * update.Qty
	})

	if latest == nil {
		return 0, io.EOF
	}

	return ring.Read(buffer)
}

/*
Snapshot returns the scoped symbol's latest input facts.
*/
func (trade *Trade) Snapshot(symbol string) TradeSnapshot {
	value, ok := trade.symbols.Load(symbol)

	if !ok {
		return TradeSnapshot{}
	}

	ring := value.(*structure.ClockRing[*market.TradeUpdate])

	var (
		first  *market.TradeUpdate
		latest *market.TradeUpdate
		volume float64
	)

	ring.Do(func(slot structure.ClockSlot[*market.TradeUpdate]) {
		update := slot.Payload

		if update == nil {
			return
		}

		if first == nil {
			first = update
		}

		latest = update
		volume += update.Price * update.Qty
	})

	if latest == nil {
		return TradeSnapshot{}
	}

	elapsed := 0.0

	if first != nil {
		elapsed = latest.Timestamp.Sub(first.Timestamp).Seconds()
	}

	return TradeSnapshot{
		Price:    latest.Price,
		Volume:   volume,
		Elapsed:  elapsed,
		Observed: latest.Timestamp,
	}
}

func (trade *Trade) Close() error {
	trade.cancel()

	return nil
}
