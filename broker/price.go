package broker

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	sdkdecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

var (
	decimalHundred = decimal.NewFromInt(100)
	exitRoomLevels = 5
)

/*
Price answers book questions with fills. A last trade is not a price.
*/
type Price struct {
	*runtime.System
	Instrument *Instrument
	Books      *BookManager
	ticker     *market.Ticker
	tape       *Tape
	venue      market.Venue
	public     network.Client
	balance    *market.Balance
	normalizer *spot.Normalizer
	exhaustion sync.Map // symbol → float64 pumpdump reading
	lastPrint  sync.Map // symbol → time.Time latest aggressive print
}

/*
NewPrice loads taker fees before READY. Fee load failure leaves ERROR status
so callers can check Status without a nil Price.
*/
func NewPrice(
	ctx context.Context,
	books *BookManager,
	instrument *Instrument,
	normalizer *spot.Normalizer,
	host market.Venue,
	public network.Client,
) *Price {
	if normalizer == nil {
		normalizer = spot.NewNormalizer()
	}

	price := &Price{
		System:     runtime.NewSystem(ctx, "price"),
		Instrument: instrument,
		Books:      books,
		ticker:     market.NewTicker(nil),
		tape:       NewTape(),
		venue:      host,
		public:     public,
		normalizer: normalizer,
	}

	if !price.loadFees() {
		return price
	}

	if public != nil && host != nil {
		batch := system.Cfg.WebSocket.TickerBatchSize

		if batch <= 0 {
			batch = 32
		}

		cursor := 0
		ticker := host.Ticker()

		public.Watch(
			system.Cfg.WebSocket.Endpoints.Ticker,
			func() map[string]any {
				symbols, next := price.PulseSymbols(batch, cursor)
				cursor = next

				if len(symbols) == 0 {
					return nil
				}

				return map[string]any{
					"pair":         strings.Join(symbols, ","),
					"assetVersion": "1",
				}
			},
			ticker,
			func() { price.applyTicker(ticker) },
		)
	}

	price.Transition(runtime.READY)

	return price
}

/*
applyTicker folds a decoded venue ticker frame into the live reference book.
*/
func (price *Price) applyTicker(frame market.MarketType) {
	if frame == nil {
		return
	}

	batch := market.NewTicker(nil)
	frame.ToMarket(batch)

	for pair, row := range batch.Rows {
		if row == nil {
			continue
		}

		price.ticker.Set(price.normalizer.Name(pair), row)
	}
}

/*
VolumeUSD is the latest ticker 24h quote turnover for mover floors.
*/
func (price *Price) VolumeUSD(symbol string) float64 {
	if price == nil || price.ticker == nil {
		return 0
	}

	if price.normalizer != nil {
		symbol = price.normalizer.Name(symbol)
	}

	return price.ticker.VolumeUSD(symbol)
}

/*
ChangePct is session open → live mark when both exist, else the REST open→last
reading. Mark keeps day-move oracles current between ticker batches.
*/
func (price *Price) ChangePct(symbol string) float64 {
	name := price.normalizer.Name(symbol)
	row := price.ticker.Of(name)

	if row == nil {
		return 0
	}

	open := row.Open

	if open <= 0 {
		return row.ChangePct
	}

	last := row.Last

	if mark := price.Mark(name); mark != nil && mark.Sign() > 0 {
		last = mark.InexactFloat64()
	}

	if last <= 0 {
		return row.ChangePct
	}

	return (last - open) / open * 100
}

/*
Mark is the best bid — what a long can hit now.
*/
func (price *Price) Mark(symbol string) (bid *decimal.Decimal) {
	if price == nil || price.Books == nil {
		return nil
	}

	if price.normalizer != nil {
		symbol = price.normalizer.Name(symbol)
	}

	price.Books.Book(symbol, func(book *market.Book) {
		bid = book.BidPrice()
	})

	return bid
}

/*
Walk fills qty against the book. buy walks asks; sell walks bids.
Returns filled base size and gross quote notional.
*/
func (price *Price) Walk(
	symbol string, qty decimal.Decimal, buy bool,
) (filled, gross *decimal.Decimal) {
	if price == nil || qty.Sign() <= 0 {
		return nil, nil
	}

	symbol = price.normalizer.Name(symbol)

	price.Books.Book(symbol, func(book *market.Book) {
		if book == nil {
			return
		}

		var got, cost decimal.Decimal

		level := book.BestBid()

		if buy {
			level = book.BestAsk()
		}

		for level != nil && qty.Sign() > 0 {
			cur := level

			if buy {
				level = level.Higher
			} else {
				level = level.Lower
			}

			if !cur.Live() {
				continue
			}

			take := decimal.Min(*cur.Quantity, qty)
			got = got.Add(take)
			cost = cost.Add(cur.Price.Mul(take))
			qty = qty.Sub(take)
		}

		if got.Sign() <= 0 {
			return
		}

		filled = &got
		gross = &cost
	})

	return filled, gross
}

/*
Fee is taker cut on amount (rate percent of notional).
*/
func (price *Price) Fee(symbol string, amount *decimal.Decimal) *decimal.Decimal {
	if amount == nil || amount.Sign() <= 0 {
		return nil
	}

	rate := price.Instrument.TakerFee(price.normalizer.Name(symbol))

	if rate == nil {
		return nil
	}

	out := amount.Mul(rate.Div(decimalHundred))
	return &out
}

/*
ExitRoom is cumulative bid depth over the top levels / quantity.
Touch-only depth flickers; summing a few levels is the absorbable room.
*/
func (price *Price) ExitRoom(symbol string, quantity *decimal.Decimal) *decimal.Decimal {
	if quantity == nil || quantity.Sign() <= 0 {
		return nil
	}

	symbol = price.normalizer.Name(symbol)

	var depth decimal.Decimal

	price.Books.Book(symbol, func(book *market.Book) {
		count := 0

		for level := book.BestBid(); level != nil && count < exitRoomLevels; level = level.Lower {
			if !level.Live() {
				continue
			}

			depth = depth.Add(*level.Quantity)
			count++
		}
	})

	if depth.Sign() <= 0 {
		return nil
	}

	r := depth.Div(*quantity)
	return &r
}

/*
NoteExhaustion caches pumpdump exhaustion for stoploss friction.
*/
func (price *Price) NoteExhaustion(symbol string, value float64) {
	if price.normalizer != nil {
		symbol = price.normalizer.Name(symbol)
	}

	if symbol == "" || value < 0 {
		return
	}

	price.exhaustion.Store(symbol, value)
}

/*
ExhaustionReading is the last NoteExhaustion for symbol, or 0 when never seen.
*/
func (price *Price) ExhaustionReading(symbol string) float64 {
	if price.normalizer != nil {
		symbol = price.normalizer.Name(symbol)
	}

	if symbol == "" {
		return 0
	}

	raw, ok := price.exhaustion.Load(symbol)

	if !ok {
		return 0
	}

	return raw.(float64)
}

/*
NoteTrade feeds tape uptake from aggressive buy notional only. Sell prints on a
quiet pair must not keep BookIntact true for the rest of the tape window.
Every print (buy or sell) still stamps LastPrintAt for correlation heartbeats.
*/
func (price *Price) NoteTrade(trade *market.TradeData) {
	if trade == nil || trade.Price.Sign() <= 0 || trade.Qty.Sign() <= 0 {
		return
	}

	name := price.normalizer.Name(trade.Symbol)

	// Same-timestamp sweep prints must all land on the tape. Only drop
	// strictly older stamps; lastPrint and LastPrintAt share `name`.
	if raw, ok := price.lastPrint.Load(name); ok {
		if prev, ok := raw.(time.Time); ok && trade.Timestamp.Before(prev) {
			return
		}
	}

	price.lastPrint.Store(name, trade.Timestamp)

	if trade.Side != market.SideBuy {
		return
	}

	price.tape.Note(
		name,
		trade.Price.Mul(trade.Qty).InexactFloat64(),
		trade.Timestamp,
	)
}

/*
LastPrintAt is the newest trade timestamp for a symbol — correlation heartbeats
prefer this over a frozen resting touch order time.
*/
func (price *Price) LastPrintAt(symbol string) time.Time {
	if price == nil {
		return time.Time{}
	}

	raw, ok := price.lastPrint.Load(price.normalizer.Name(symbol))

	if !ok {
		return time.Time{}
	}

	at, _ := raw.(time.Time)
	return at
}

/*
TapeUptake is recent vs prior quote-flow windows for board and BookIntact.
*/
func (price *Price) TapeUptake(symbol string, now time.Time) (recent, prior float64) {
	return price.tape.Uptake(price.normalizer.Name(symbol), now)
}

/*
SpreadPct is touch spread percent from the live book.
*/
func (price *Price) SpreadPct(symbol string) (*decimal.Decimal, bool) {
	var spread *decimal.Decimal

	price.Books.Book(price.normalizer.Name(symbol), func(book *market.Book) {
		if !book.Tradable() {
			return
		}

		bid, ask := *book.BidPrice(), *book.AskPrice()
		// Ask-relative percent — mid-based book.Spread() is not the desk gate.
		pct := ask.Sub(bid).Div(ask).Mul(decimal.NewFromInt(100))
		spread = &pct
	})

	if spread == nil {
		return nil, false
	}

	return spread, true
}

/*
PeakFrac is mark's place in the 24h high–low range (0 at low, 1 at high).
Live bid marks beat stale REST Last so peak vetoes track the tape, not the
last ticker batch from ~20 minutes ago.
*/
func (price *Price) PeakFrac(symbol string) (float64, bool) {
	name := price.normalizer.Name(symbol)
	row := price.ticker.Of(name)

	if row == nil {
		return 0, false
	}

	span := row.High24h - row.Low24h

	if span <= 0 {
		return 0, false
	}

	last := row.Last

	if mark := price.Mark(name); mark != nil && mark.Sign() > 0 {
		last = mark.InexactFloat64()
	}

	if last <= 0 {
		return 0, false
	}

	return (last - row.Low24h) / span, true
}

/*
EntryPeakOK is true when PeakFrac is under the accum/ignition selection cap.
*/
func (price *Price) EntryPeakOK(symbol string, ignition bool) bool {
	cap := system.Cfg.Selection.MaxAccumPeakFrac

	if ignition {
		cap = system.Cfg.Selection.MaxIgnitionPeakFrac
	}

	if cap <= 0 || cap >= 1 {
		return false
	}

	frac, ok := price.PeakFrac(symbol)

	return ok && frac < cap
}

/*
TapeAccelOK is rising buy-tape vs the prior window against configured floors.
*/
func (price *Price) TapeAccelOK(symbol string) bool {
	risk := system.Cfg.Risk
	recent, prior := price.TapeUptake(symbol, time.Now())

	if risk.MinTapeNotionalUSD <= 0 || risk.MinTapeAccelRatio <= 0 {
		return false
	}

	if recent < risk.MinTapeNotionalUSD {
		return false
	}

	if prior <= 0 {
		return recent >= risk.MinTapeNotionalUSD*risk.MinTapeAccelRatio
	}

	return recent/prior >= risk.MinTapeAccelRatio
}

/*
SessionMoveOK is session open→mark plus live tape and volume-clock pumpdump impulse.
*/
func (price *Price) SessionMoveOK(symbol string, pump *equation.IgnitionOutput) bool {
	risk := system.Cfg.Risk

	if price.ChangePct(symbol) < risk.MinMoverChangePct {
		return false
	}

	if !price.TapeAccelOK(symbol) {
		return false
	}

	return PumpMoveOK(pump)
}

/*
TouchBidCash is quote notional on the top three bids.
*/
func (price *Price) TouchBidCash(symbol string) (*decimal.Decimal, bool) {
	var cash decimal.Decimal

	price.Books.Book(price.normalizer.Name(symbol), func(book *market.Book) {
		for n, level := 0, book.BestBid(); n < 3 && level != nil; n, level = n+1, level.Lower {
			cash = cash.Add(level.Notional())
		}
	})

	if cash.Sign() <= 0 {
		return nil, false
	}

	return &cash, true
}

/*
BackingRatio is near-mid bid cash over ask cash.
*/
func (price *Price) BackingRatio(symbol string) (ratio float64, ok bool) {
	band := system.Cfg.Risk.BackingBandFrac

	price.Books.Book(price.normalizer.Name(symbol), func(book *market.Book) {
		if !book.Tradable() {
			return
		}

		mid := book.Midpoint()

		if mid == nil || mid.Sign() <= 0 {
			return
		}

		lo := mid.Mul(decimal.NewFromFloat(1 - band))
		hi := mid.Mul(decimal.NewFromFloat(1 + band))
		bidCash, askCash := 0.0, 0.0

		for level := book.BestBid(); level != nil; level = level.Lower {
			if !level.Live() {
				continue
			}

			if level.Price.Cmp(lo) < 0 {
				break
			}

			bidCash += level.Notional().InexactFloat64()
		}

		for level := book.BestAsk(); level != nil; level = level.Higher {
			if !level.Live() {
				continue
			}

			if level.Price.Cmp(hi) > 0 {
				break
			}

			askCash += level.Notional().InexactFloat64()
		}

		if askCash > 0 {
			ratio, ok = bidCash/askCash, true
			return
		}

		if bidCash > 0 {
			ratio, ok = math.Inf(1), true
			return
		}
	})

	return ratio, ok
}

/*
BookIntact is cash + backing + accelerating tape + open-drag — Observe's book oracle.
*/
func (price *Price) BookIntact(symbol string) bool {
	risk := system.Cfg.Risk
	cash, ok := price.TouchBidCash(symbol)

	if !ok || cash.InexactFloat64() < risk.MinTouchBidCash {
		return false
	}

	ratio, ok := price.BackingRatio(symbol)

	if !ok || ratio < risk.MinBackingRatio {
		return false
	}

	if !price.EntryOpenDragOK(symbol) {
		return false
	}

	return price.TapeAccelOK(symbol)
}

/*
PulseSymbols rotates through the instrument universe for quiet-symbol heartbeats.
*/
func (price *Price) PulseSymbols(batch, start int) ([]string, int) {
	if price.Instrument == nil {
		return nil, start
	}

	all := price.Instrument.Symbols()

	if len(all) == 0 || batch <= 0 {
		return nil, start
	}

	if batch > len(all) {
		batch = len(all)
	}

	start = max(0, start) % len(all)
	out := make([]string, 0, batch)

	for i := range batch {
		out = append(out, all[(start+i)%len(all)])
	}

	return out, (start + batch) % len(all)
}

/*
PulseQuote is last plus touch bid/ask for heartbeat ignition inputs.
*/
func (price *Price) PulseQuote(symbol string) (last, bid, ask float64, ok bool) {
	symbol = price.normalizer.Name(symbol)

	price.Books.Book(symbol, func(book *market.Book) {
		if !book.Tradable() {
			return
		}

		bid = book.BidPrice().InexactFloat64()
		ask = book.AskPrice().InexactFloat64()
		ok = bid > 0 && ask > bid
	})

	if !ok {
		return 0, 0, 0, false
	}

	return bid, bid, ask, true
}

/*
PnL is bid-walk proceeds, minus the exit fee and the entry fee, minus
Entry times Qty. Incomplete walks return nil.
*/
func (price *Price) PnL(position *Position) *decimal.Decimal {
	pnl, _ := price.markPnL(position)
	return pnl
}

/*
PnLPct is PnL as a percent of Entry times Qty.
*/
func (price *Price) PnLPct(position *Position) *decimal.Decimal {
	_, pct := price.markPnL(position)
	return pct
}

/*
markPnL snapshots Entry/Qty under position.mu then walks the book unlocked so
ProfitableExit/Model cannot race Apply while Update holds the same mutex.
*/
func (price *Price) markPnL(
	position *Position,
) (pnl, pct *decimal.Decimal) {
	if position == nil {
		return nil, nil
	}

	position.mu.Lock()
	pnl, pct = price.markPnLLocked(position)
	position.mu.Unlock()
	return pnl, pct
}

/*
markPnLLocked walks the bid book once and returns fee-net PnL with PnLPct.
Caller must hold position.mu (Position.Update). One Book read so Update cannot
pair pnl from T with pct from T+1.
*/
func (price *Price) markPnLLocked(
	position *Position,
) (pnl, pct *decimal.Decimal) {
	if position == nil || position.validateLocked() != nil {
		return nil, nil
	}

	entry := *position.Entry
	qty := *position.Qty
	symbol := position.Symbol
	cost := entry.Mul(qty)

	if cost.Sign() <= 0 {
		return nil, nil
	}

	name := price.normalizer.Name(symbol)
	filled, gross := price.Walk(name, qty, false)

	if filled == nil || gross == nil || filled.Cmp(qty) < 0 || gross.Sign() <= 0 {
		return nil, nil
	}

	feeIn := price.Fee(name, &cost)
	feeOut := price.Fee(name, gross)

	if feeIn == nil || feeOut == nil {
		return nil, nil
	}

	p := gross.Sub(*feeOut).Sub(cost).Sub(*feeIn)
	q := p.Div(cost).Mul(decimalHundred)
	return &p, &q
}

/*
Allocate sizes a venue-legal long from wallet (nil = wired desk balance).
Working enters spend MaxFraction of (1−ReserveFraction)×cash × conviction.
ShortHold hint draws from the reserved slice instead — no forced rotation.
Size starts at touch ask / TouchDivisor and lifts to QtyMin/CostMin when spend fits.
*/
func (price *Price) Allocate(
	symbol string, wallet *market.Balance, hint SizeHint,
) (*decimal.Decimal, error) {
	symbol = price.normalizer.Name(symbol)

	// Nil wallet → desk balance, skip venue mins (observe). Callers pass a wallet to enforce.
	enforceMins := wallet != nil

	if wallet == nil {
		wallet = price.balance
	}

	if price.Instrument == nil || !price.Instrument.Has(symbol) {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "symbol not in instrument universe")
	}

	if wallet == nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "balance is not wired")
	}

	pair := price.Instrument.Pair(symbol)
	raw := wallet.Cash(pair.Quote)

	if raw == nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "no spendable quote cash")
	}

	cash, err := decimal.NewFromString(raw.String())

	if err != nil || cash.Sign() <= 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "quote cash unusable")
	}

	if pair.QtyMin == nil || pair.CostMin == nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "instrument limits missing")
	}

	lane := price.cashLane(cash, hint.ShortHold)
	budget := lane.Mul(decimal.NewFromFloat(system.Cfg.Risk.MaxFraction)).
		Mul(decimal.NewFromFloat(hint.Conviction()))

	if budget.Sign() <= 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "risk budget is zero")
	}

	var touch *decimal.Decimal

	price.Books.Book(symbol, func(book *market.Book) {
		if !book.Tradable() {
			return
		}

		touch = book.AskSize()
	})

	if touch == nil || touch.Sign() <= 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "book not ready")
	}

	divisor := system.Cfg.Risk.TouchDivisor

	if divisor <= 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "touch divisor invalid")
	}

	cap := touch.Div(decimal.NewFromFloat(divisor))
	sized, err := price.formatQty(symbol, cap)

	if err != nil || sized == nil || sized.Sign() <= 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "touch cap below size step")
	}

	if enforceMins {
		minQty, err := price.venueMinQty(symbol, pair)

		if err != nil {
			return nil, err
		}

		if sized.Cmp(*minQty) < 0 {
			sized = minQty
		}
	}

	for range 8 {
		filled, gross := price.Walk(symbol, *sized, true)

		if filled == nil || gross == nil || filled.Sign() <= 0 {
			return nil, errnie.Err(
				errnie.UnprocessableContent,
				"[price] allocate failed",
				nil,
			).With("symbol", symbol, "why", "ask walk produced zero size")
		}

		fee := price.Fee(symbol, gross)

		if fee == nil {
			return nil, errnie.Err(
				errnie.UnprocessableContent,
				"[price] allocate failed",
				nil,
			).With("symbol", symbol, "why", "fee unknown")
		}

		spend := gross.Add(*fee)

		if spend.Cmp(budget) <= 0 {
			if !enforceMins ||
				(filled.Cmp(*pair.QtyMin) >= 0 && gross.Cmp(*pair.CostMin) >= 0) {
				return filled, nil
			}

			// Touch/trim undershot venue mins but still had budget headroom —
			// lift to QtyMin/CostMin when that spend still fits MaxFraction.
			return price.allocateVenueMin(symbol, pair, budget)
		}

		avg := gross.Div(*filled)
		unitFee := price.Fee(symbol, &avg)

		if unitFee == nil || avg.Sign() <= 0 {
			return nil, errnie.Err(
				errnie.UnprocessableContent,
				"[price] allocate failed",
				nil,
			).With("symbol", symbol, "why", "cannot trim to budget")
		}

		unit := avg.Add(*unitFee)
		cut := spend.Sub(budget).Div(unit)

		if pair.QtyIncrement != nil && pair.QtyIncrement.Sign() > 0 &&
			cut.Cmp(*pair.QtyIncrement) < 0 {
			cut = *pair.QtyIncrement
		}

		if cut.Sign() <= 0 || filled.Cmp(cut) <= 0 {
			return nil, errnie.Err(
				errnie.UnprocessableContent,
				"[price] allocate failed",
				nil,
			).With("symbol", symbol, "why", "cannot trim to budget")
		}

		next := filled.Sub(cut)
		formatted, err := price.formatQty(symbol, next)

		if err != nil || formatted == nil || formatted.Sign() <= 0 || formatted.Cmp(*filled) >= 0 {
			return nil, errnie.Err(
				errnie.UnprocessableContent,
				"[price] allocate failed",
				nil,
			).With("symbol", symbol, "why", "cannot trim to budget")
		}

		sized = formatted
	}

	return nil, errnie.Err(
		errnie.UnprocessableContent,
		"[price] allocate failed",
		nil,
	).With("symbol", symbol, "why", "cannot trim to budget")
}

/*
cashLane splits quote cash into working vs reserved pools. Short-hold draws
use the reserved slice; normal enters never spend it.
*/
func (price *Price) cashLane(cash decimal.Decimal, reserve bool) decimal.Decimal {
	frac := system.Cfg.Risk.ReserveFraction
	if frac <= 0 {
		return cash
	}
	if frac >= 1 {
		if reserve {
			return cash
		}
		return decimal.Zero
	}

	reserved := cash.Mul(decimal.NewFromFloat(frac))
	if reserve {
		return reserved
	}
	return cash.Sub(reserved)
}

/*
OpenDragPct is the fee-net close-now loss (%) of buying one touch/÷N clip and
immediately selling it — spread + fees as positive drag. False when the book
cannot size the probe.
*/
func (price *Price) OpenDragPct(symbol string) (float64, bool) {
	symbol = price.normalizer.Name(symbol)

	var touch *decimal.Decimal
	price.Books.Book(symbol, func(book *market.Book) {
		if !book.Tradable() {
			return
		}
		touch = book.AskSize()
	})

	if touch == nil || touch.Sign() <= 0 {
		return 0, false
	}

	divisor := system.Cfg.Risk.TouchDivisor
	if divisor <= 0 {
		return 0, false
	}

	cap := touch.Div(decimal.NewFromFloat(divisor))
	sized, err := price.formatQty(symbol, cap)
	if err != nil || sized == nil || sized.Sign() <= 0 {
		return 0, false
	}

	bought, buyGross := price.Walk(symbol, *sized, true)
	if bought == nil || buyGross == nil || bought.Sign() <= 0 || buyGross.Sign() <= 0 {
		return 0, false
	}

	buyFee := price.Fee(symbol, buyGross)
	if buyFee == nil {
		return 0, false
	}

	sold, sellGross := price.Walk(symbol, *bought, false)
	if sold == nil || sellGross == nil || sold.Sign() <= 0 {
		return 0, false
	}

	sellFee := price.Fee(symbol, sellGross)
	if sellFee == nil {
		return 0, false
	}

	cost := buyGross.Add(*buyFee)
	proceeds := sellGross.Sub(*sellFee)
	pnl := proceeds.Sub(cost)
	drag := pnl.Neg().Div(cost).Mul(decimalHundred).InexactFloat64()
	if drag < 0 {
		drag = 0
	}
	return drag, true
}

/*
EntryOpenDragOK is true when close-now round-trip drag is under MaxEntryOpenDragPct.
*/
func (price *Price) EntryOpenDragOK(symbol string) bool {
	cap := system.Cfg.Risk.MaxEntryOpenDragPct
	if cap <= 0 {
		return true
	}

	drag, ok := price.OpenDragPct(symbol)
	return ok && drag <= cap
}

/*
formatQty snaps qty onto the venue size grid. Nil normalizer keeps qty as-is.
*/
func (price *Price) formatQty(symbol string, qty decimal.Decimal) (*decimal.Decimal, error) {
	if price.normalizer == nil {
		out := qty
		return &out, nil
	}

	raw, err := sdkdecimal.NewFromString(qty.String())

	if err != nil {
		return nil, errnie.Error(err)
	}

	formatted, err := price.normalizer.FormatSize(symbol, raw)

	if err != nil || formatted == nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] format qty failed",
			err,
		).With("symbol", symbol)
	}

	parsed, err := decimal.NewFromString(formatted.String())

	if err != nil {
		return nil, errnie.Error(err)
	}

	return &parsed, nil
}

/*
venueMinQty is the smallest venue-legal size: at least QtyMin, and large enough
that an ask walk clears CostMin. Walks ask levels until both mins clear — no
linear Level-1 price extrapolation (steep books overshoot CostMin badly).
*/
func (price *Price) venueMinQty(
	symbol string, pair kraken.InstrumentPair,
) (*decimal.Decimal, error) {
	symbol = price.normalizer.Name(symbol)

	var raw decimal.Decimal
	met := false

	price.Books.Book(symbol, func(book *market.Book) {
		if book == nil {
			return
		}

		var got, cost decimal.Decimal

		for level := book.BestAsk(); level != nil; level = level.Higher {
			if !level.Live() || level.Price == nil || level.Quantity == nil {
				continue
			}

			needQty := got.Cmp(*pair.QtyMin) < 0
			needCost := cost.Cmp(*pair.CostMin) < 0

			if !needQty && !needCost {
				break
			}

			want := decimal.Zero

			if needQty {
				want = pair.QtyMin.Sub(got)
			}

			if needCost && level.Price.Sign() > 0 {
				byCost := pair.CostMin.Sub(cost).Div(*level.Price)

				if byCost.Cmp(want) > 0 {
					want = byCost
				}
			}

			if want.Sign() <= 0 {
				want = *level.Quantity
			}

			take := decimal.Min(*level.Quantity, want)
			got = got.Add(take)
			cost = cost.Add(level.Price.Mul(take))
		}

		if got.Cmp(*pair.QtyMin) >= 0 && cost.Cmp(*pair.CostMin) >= 0 {
			raw = got
			met = true
		}
	})

	if !met || raw.Sign() <= 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "ask walk cannot clear venue mins")
	}

	if pair.QtyIncrement != nil && pair.QtyIncrement.Sign() > 0 {
		raw = raw.Div(*pair.QtyIncrement).Ceil().Mul(*pair.QtyIncrement)
	}

	formatted, err := price.formatQty(symbol, raw)

	if err != nil || formatted == nil || formatted.Sign() <= 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "venue minimum below size step")
	}

	if formatted.Cmp(*pair.QtyMin) < 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "venue minimum below size step")
	}

	filled, gross := price.Walk(symbol, *formatted, true)

	if filled == nil || gross == nil ||
		filled.Cmp(*pair.QtyMin) < 0 || gross.Cmp(*pair.CostMin) < 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "size below venue minimum")
	}

	return formatted, nil
}

/*
allocateVenueMin walks the QtyMin/CostMin size and accepts it only when fee-gross
spend still fits the MaxFraction cash budget.
*/
func (price *Price) allocateVenueMin(
	symbol string, pair kraken.InstrumentPair, budget decimal.Decimal,
) (*decimal.Decimal, error) {
	minQty, err := price.venueMinQty(symbol, pair)

	if err != nil {
		return nil, err
	}

	filled, gross := price.Walk(symbol, *minQty, true)

	if filled == nil || gross == nil || filled.Sign() <= 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "ask walk produced zero size")
	}

	if filled.Cmp(*pair.QtyMin) < 0 || gross.Cmp(*pair.CostMin) < 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "size below venue minimum")
	}

	fee := price.Fee(symbol, gross)

	if fee == nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "fee unknown")
	}

	if gross.Add(*fee).Cmp(budget) > 0 {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With("symbol", symbol, "why", "venue minimum exceeds risk budget")
	}

	return filled, nil
}

func (price *Price) loadFees() bool {
	symbols := price.Instrument.Symbols()

	if len(symbols) == 0 {
		price.Error(errnie.Err(
			errnie.Internal, "trade volume fees need a non-empty symbol list", nil,
		))
		return false
	}

	raw, err := price.public.Post(
		system.Cfg.WebSocket.Endpoints.TradeVolume,
		kraken.NewTradeVolumeRequest(symbols),
	)

	if err != nil {
		price.Error(errnie.Err(errnie.IO, "failed to get trade volume fees", err))
		return false
	}

	volume, err := kraken.ParseTradeVolume(raw)

	if err != nil {
		price.Error(err)
		return false
	}

	ok := false

	for pair, fee := range volume.Fees {
		if fee.Fee == nil {
			continue
		}

		symbol := price.normalizer.Name(pair)

		if !price.Instrument.Has(symbol) {
			continue
		}

		price.Instrument.takerFees.Store(symbol, fee.Fee)
		ok = true
	}

	if !ok {
		price.Error(errnie.Err(
			errnie.UnprocessableContent, "trade volume returned no usable taker fees", nil,
		))
	}

	return ok
}
