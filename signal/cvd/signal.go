package cvd

import (
	"context"
	"math"
	"sync"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	parent "github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/signal/volumeclock"
)

type window struct {
	buyNotional  float64
	sellNotional float64
}

// initiativeDeadband matches pumpdump quiet-band: flow without repricing.
const initiativeDeadband = 5e-4

type symbolState struct {
	clock              volumeclock.Clock
	window             window
	prevMidpoint       float64
	cumulativeCVD      float64
	peakMid            float64
	peakCVD            float64
	lastDivergence     float64
	prevSignedFraction float64
	lastInitiativeLock float64
	lastFlowSignFlip   float64
	hasPrevSigned      bool
}

type Signal struct {
	*runtime.System
	books  *broker.Book
	states *sync.Map
	keys   []string
	emit   *parent.Emit
}

func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	signal := &Signal{
		books:  books,
		states: &sync.Map{},
		emit:   parent.NewEmit(),
		keys: []string{
			"aggressor_backing",
			"cvd_divergence",
			"initiative_lock",
			"flow_sign_flip",
		},
	}

	signal.System = runtime.NewSystem(ctx, "cvd", signal)
	return signal
}

/*
Step processes every trade entry of the tick's frame concurrently by symbol
print. Per-symbol state and the volume clock are guarded; layer appends are
already synchronized on the shared value.
*/
func (signal *Signal) Step(value *data.Value) *data.Value {
	var trades []kraken.TradeData
	value.Lookup(&trades, "spot:trade")

	if err := parent.RunTrades(signal.Context(), trades, func(trade kraken.TradeData) {
		signal.step(value, trade)
	}); err != nil {
		signal.Error(err)
	}

	return value
}

func (signal *Signal) step(value *data.Value, trade kraken.TradeData) *data.Value {
	if signal.Status() != runtime.READY {
		return value
	}

	side := trade.Side

	if side != "buy" && side != "sell" {
		return value
	}

	price := trade.Price.Float64()
	qty := trade.Qty

	if price <= 0 || qty <= 0 {
		return value
	}

	tradeNotional := qty * price

	var (
		signedFraction float64
		cvdDivergence  float64
		initiativeLock float64
		flowSignFlip   float64
		appendMetrics  bool
	)

	// The `books.Book` callback is synchronous; the book is present only if
	// the callback ran. Pending (no snapshot yet), stale (L3 socket down) and
	// invalid (venue checksum mismatch / absent level) symbols skip the
	// callback and the tick carries book_error/<symbol> instead.
	signal.books.Book(trade.Symbol, func(b *spotbook.Book) {
		bid := b.BestBid()
		ask := b.BestAsk()
		if bid == nil || ask == nil || bid.Price == nil || ask.Price == nil {
			return
		}

		midpoint := (bid.Price.Float64() + ask.Price.Float64()) / 2.0

		untypedState, _ := signal.states.LoadOrStore(trade.Symbol, &symbolState{})
		state := untypedState.(*symbolState)

		tick := state.clock.Step(price, qty, float64(value.Timestamp), midpoint)

		if tick.Counted {
			switch side {
			case "buy":
				state.window.buyNotional += tradeNotional
			case "sell":
				state.window.sellNotional += tradeNotional
			}
		}

		if tick.Closed {
			flow := state.window
			state.window = window{}
			grossNotional := flow.buyNotional + flow.sellNotional
			netNotional := flow.buyNotional - flow.sellNotional
			if grossNotional > 0 {
				signedFraction = netNotional / grossNotional
			}

			if state.hasPrevSigned &&
				signedFraction*state.prevSignedFraction < 0 &&
				math.Abs(signedFraction) > 0 &&
				math.Abs(state.prevSignedFraction) > 0 {
				flowSignFlip = math.Min(
					1,
					math.Abs(signedFraction-state.prevSignedFraction)/2,
				)
			}

			state.prevSignedFraction = signedFraction
			state.hasPrevSigned = true

			barMid := tick.Bar.AtMid
			if barMid <= 0 {
				barMid = midpoint
			}

			state.cumulativeCVD += netNotional
			cvdDivergence = state.lastDivergence

			if barMid > state.peakMid {
				priceScale := math.Max(math.Abs(state.peakMid), 1e-12)
				cvdScale := math.Max(math.Abs(state.peakCVD), 1e-12)
				priceLeg := (barMid - state.peakMid) / priceScale
				cvdLeg := (state.cumulativeCVD - state.peakCVD) / cvdScale
				if priceLeg > 0 && priceLeg > cvdLeg {
					state.lastDivergence = priceLeg - cvdLeg
					cvdDivergence = state.lastDivergence
				}

				state.peakMid = barMid
				state.peakCVD = state.cumulativeCVD
			}

			bar := tick.Bar
			if bar.FromMid > 0 && bar.AtMid > 0 {
				midMove := math.Abs(math.Log(bar.AtMid / bar.FromMid))
				quiet := 1 - math.Min(1, midMove/initiativeDeadband)
				initiativeLock = math.Abs(signedFraction) * quiet
			}

			state.lastDivergence = cvdDivergence
			state.lastInitiativeLock = initiativeLock
			state.lastFlowSignFlip = flowSignFlip
			appendMetrics = true
		} else if state.hasPrevSigned {
			signedFraction = state.prevSignedFraction
			cvdDivergence = state.lastDivergence
			initiativeLock = state.lastInitiativeLock
			flowSignFlip = state.lastFlowSignFlip
			appendMetrics = true
		}

		state.prevMidpoint = midpoint
	})

	if !appendMetrics {
		return value
	}

	keys := []string{
		signal.Name(),
		trade.Symbol,
	}

	signal.emit.Metric(value, []float64{
		signedFraction,
		cvdDivergence,
		initiativeLock,
		flowSignFlip,
	}, append(keys, signal.keys...)...)

	audit.Record.Write(map[string]any{
		"signal":         signal.Name(),
		"trade":          trade,
		"signedFraction": signedFraction,
	})

	return value
}
