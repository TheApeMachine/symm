package broker

import (
	"context"
	"fmt"
	"math"
	"time"

	sdkdecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

type Direction string

const (
	BUY  Direction = "buy"
	SELL Direction = "sell"
)

var (
	decimalZero        = decimal.NewFromInt(0)
	decimalOne         = decimal.NewFromInt(1)
	decimalHundred     = decimal.NewFromInt(100)
	decimalNegativeOne = decimal.NewFromInt(-1)
	decimalPercent     = decimalOne.Div(decimalHundred)
	decimalTwo         = decimal.NewFromInt(2)
	decimalThree       = decimal.NewFromInt(3)
)

/*
Price answers book questions with fills. A last trade is not a price.
*/
type Price struct {
	*runtime.System
	Instrument *Instrument
	Books      *BookManager
	ticker     *market.Ticker
	venue      market.Venue
	public     network.Client
	balance    *market.Balance
	normalizer *spot.Normalizer
	tape       *Tape
}

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
		venue:      host,
		public:     public,
		normalizer: normalizer,
		tape:       NewTape(),
	}

	if !price.loadFees() {
		return nil
	}

	if host != nil {
		host.Ticker().OnChange(func(frame market.MarketType) {
			price.applyTicker(frame)
		})
	}

	if public != nil && host != nil {
		public.Watch(
			system.Cfg.WebSocket.Endpoints.Ticker,
			host.Ticker().Request(),
			host.Ticker(),
		)
	}

	price.Transition(runtime.READY)

	return price
}

func (price *Price) SetTicker(row *market.TickerData) {
	if row == nil {
		return
	}

	price.ticker.Set(row.Symbol, row)
}

func (price *Price) VolumeUSD(symbol string) float64 {
	return price.ticker.VolumeUSD(symbol)
}

func (price *Price) ChangePct(symbol string) float64 {
	return price.ticker.ChangePct(symbol)
}

/*
PeakFrac is last's position in the 24h high–low range (0 at low, 1 at high).
*/
func (price *Price) PeakFrac(symbol string) (float64, bool) {
	row := price.ticker.Of(symbol)

	if row == nil {
		return 0, false
	}

	span := row.High24h - row.Low24h

	if span <= 0 || row.Last <= 0 {
		return 0, false
	}

	return (row.Last - row.Low24h) / span, true
}

/*
BelowPeak is true when last sits under max_entry_peak_frac of the 24h range.
Disabled when the cap is <=0 or >=1. Missing range data vetoes while enabled.
*/
func (price *Price) BelowPeak(symbol string) bool {
	cap := system.Cfg.Risk.MaxEntryPeakFrac

	if cap <= 0 || cap >= 1 {
		return true
	}

	frac, ok := price.PeakFrac(symbol)

	if !ok {
		return false
	}

	return frac < cap
}

/*
NoteTrade records one print's quote dollars on the entry tape.
*/
func (price *Price) NoteTrade(trade *market.TradeData) {
	if trade == nil {
		return
	}

	at := trade.Timestamp

	if at.IsZero() {
		at = time.Now()
	}

	price.tape.Note(trade.Symbol, trade.Price.Mul(trade.Qty).InexactFloat64(), at)
}

/*
TapeUptake is recent vs prior window quote notional for the entry gate.
*/
func (price *Price) TapeUptake(symbol string, now time.Time) (recent, prior float64) {
	return price.tape.Uptake(symbol, now)
}

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

		symbol := pair

		if name := price.normalizer.Name(pair); name != "" {
			symbol = name
		}

		price.ticker.Set(symbol, row)
	}
}

func (price *Price) loadFees() bool {
	symbols := price.Instrument.Symbols()

	if len(symbols) == 0 {
		price.Error(errnie.Err(
			errnie.Internal, "trade volume fees need a non-empty symbol list", nil,
		))
		return false
	}

	if err := price.normalizer.Use(spot.NewREST()); err != nil {
		price.Error(errnie.Err(
			errnie.IO, "[price] asset pairs required before trade volume fees", err,
		))
		return false
	}

	response, err := network.NewREST().Post(
		"https://api.kraken.com"+system.Cfg.WebSocket.Endpoints.TradeVolume,
		kraken.NewTradeVolumeRequest(symbols),
	)

	if err != nil {
		price.Error(errnie.Err(errnie.IO, "failed to get trade volume fees", err))
		return false
	}

	volume, err := kraken.ParseTradeVolume(response.Body())

	if err != nil {
		price.Error(err)
		return false
	}

	ok := false

	for pair, fee := range volume.Fees {
		if fee.Fee == nil {
			continue
		}

		symbol := pair

		if name := price.normalizer.Name(pair); name != "" {
			symbol = name
		}

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

// Fee is the taker fee in percent (0.40 means 0.40%). Nil means unknown.
func (price *Price) Fee(symbol string) *decimal.Decimal {
	if fee := price.Instrument.TakerFee(symbol); fee != nil {
		return fee
	}

	if price.normalizer == nil {
		return nil
	}

	if name := price.normalizer.Name(symbol); name != "" && name != symbol {
		return price.Instrument.TakerFee(name)
	}

	return nil
}

/*
ExitRoom is best-bid size divided by quantity (1.0 = the touch absorbs the lot
once).
*/
func (price *Price) ExitRoom(symbol string, quantity *decimal.Decimal) *decimal.Decimal {
	if quantity == nil || quantity.Sign() <= 0 {
		return nil
	}

	var depth *decimal.Decimal

	price.Books.Book(symbol, func(managed *market.Book) {
		if managed == nil || managed.BestBid() == nil {
			return
		}

		depth = managed.BestBid().Quantity
	})

	if depth == nil || depth.Sign() <= 0 {
		return nil
	}

	r := depth.Div(*quantity)
	return &r
}

/*
TouchBidCash is quote notional on the top three bid levels.
*/
func (price *Price) TouchBidCash(symbol string) (*decimal.Decimal, bool) {
	if symbol == "" {
		return nil, false
	}

	var cash *decimal.Decimal

	price.Books.Book(symbol, func(managed *market.Book) {
		if managed == nil || managed.BestBid() == nil {
			return
		}

		total := decimalZero

		for n, level := 0, managed.BestBid(); n < 3 && level != nil; n, level = n+1, level.Lower {
			px, qty := level.Price, level.Quantity
			if px == nil || qty == nil || px.Sign() <= 0 || qty.Sign() <= 0 {
				continue
			}

			total = total.Add(px.Mul(*qty))
		}

		if total.Sign() > 0 {
			c := total
			cash = &c
		}
	})

	if cash == nil {
		return nil, false
	}

	return cash, true
}

// SpreadPct is the book's own relative bid-ask spread in percent.
func (price *Price) SpreadPct(symbol string) (*decimal.Decimal, bool) {
	var spread *decimal.Decimal

	price.Books.Book(symbol, func(managed *market.Book) {
		if !bookReady(managed) {
			return
		}

		spread = managed.Spread()
	})

	if spread == nil || spread.Sign() <= 0 {
		return nil, false
	}

	return spread, true
}

/*
Friction is relative spread plus both taker fee legs, as a fraction.
*/
func (price *Price) Friction(symbol string) (*decimal.Decimal, bool) {
	spreadPct, ok := price.SpreadPct(symbol)
	if !ok || spreadPct == nil {
		return nil, false
	}

	fee := price.Fee(symbol)

	if fee == nil {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"[price] friction failed",
			nil,
		).With("symbol", symbol, "why", "taker fee unknown"))
		return nil, false
	}

	if fee.Sign() < 0 {
		return nil, false
	}

	hurdle := spreadPct.Div(decimalHundred).Add(fee.Mul(decimalPercent).Mul(decimalTwo))

	if hurdle.Sign() <= 0 {
		return nil, false
	}

	return &hurdle, true
}

/*
OpenDragBudgetPct is three times Friction, in percent — the most negative
close-now PnLPct still allowed at entry.
*/
func (price *Price) OpenDragBudgetPct(symbol string) (*decimal.Decimal, bool) {
	friction, ok := price.Friction(symbol)
	if !ok || friction == nil {
		return nil, false
	}

	r := friction.Mul(decimalThree).Mul(decimalHundred)
	return &r, true
}

/*
OpenDragCap is MaxEntryOpenDragPct, tightened by OpenDragBudgetPct, floored at
two taker fee legs so fee-only drag cannot veto entry alone.
*/
func (price *Price) OpenDragCap(symbol string) float64 {
	cap := system.Cfg.Risk.MaxEntryOpenDragPct

	if budget, ok := price.OpenDragBudgetPct(symbol); ok && budget != nil {
		if b := budget.InexactFloat64(); b > 0 && b < cap {
			cap = b
		}
	}

	if fee := price.Fee(symbol); fee != nil {
		if rate := fee.InexactFloat64(); rate > 0 {
			if floor := 2 * rate; cap < floor {
				cap = floor
			}
		}
	}

	return cap
}

/*
EntryCloseNowPnLPct is fee-net percent PnL of opening the desk lot at the ask
walk and closing immediately into the bids.
*/
func (price *Price) EntryCloseNowPnLPct(symbol string) (*decimal.Decimal, bool) {
	qty, _ := price.Allocate(symbol, nil)

	if qty == nil || qty.Sign() <= 0 {
		return nil, false
	}

	entryPx, _, err := price.EntryCost(symbol, qty)

	if err != nil || entryPx == nil || entryPx.Sign() <= 0 {
		why := "entry cost unavailable"
		if err != nil {
			why = err.Error()
		}

		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[price] entry close-now failed",
			err,
		).With("symbol", symbol, "why", why))
		return nil, false
	}

	pct := price.PnLPct(symbol, qty, entryPx)

	if pct == nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[price] entry close-now failed",
			nil,
		).With("symbol", symbol, "why", "pnl percent unavailable"))
		return nil, false
	}

	return pct, true
}

// PnLPct is fee-net round-trip profit as a percent of entry cost.
func (price *Price) PnLPct(
	symbol string,
	quantity, entry *decimal.Decimal,
) *decimal.Decimal {
	if quantity == nil || entry == nil ||
		quantity.Sign() <= 0 || entry.Sign() <= 0 {
		return nil
	}

	_, _, _, pct := price.MarkClose(symbol, quantity, entry)
	return pct
}

func (price *Price) ExitVWAP(symbol string, quantity *decimal.Decimal) *decimal.Decimal {
	vwap, _, _, _ := price.MarkClose(symbol, quantity, nil)
	return vwap
}

/*
MarkClose walks the bid book once for exit VWAP, room, and fee-net PnL.
*/
func (price *Price) MarkClose(
	symbol string,
	quantity, entry *decimal.Decimal,
) (vwap, room, pnl, pnlPct *decimal.Decimal) {
	if quantity == nil || quantity.Sign() <= 0 {
		return
	}

	price.Books.Book(symbol, func(managed *market.Book) {
		if managed == nil {
			return
		}

		if bid := managed.BestBid(); bid != nil && bid.Quantity != nil && bid.Quantity.Sign() > 0 {
			r := bid.Quantity.Div(*quantity)
			room = &r
		}

		if !bookReady(managed) {
			return
		}

		// Partial depth still marks: unfilled remainder is worth zero so an
		// evaporating book cannot look better than a liquid walk.
		filled, gross, _ := price.consume(managed, quantity, nil, SELL)
		if filled == nil || gross == nil || filled.Sign() <= 0 {
			return
		}

		v := gross.Div(*filled)
		vwap = &v

		if entry == nil || entry.Sign() <= 0 {
			return
		}

		entryCost := entry.Mul(*quantity)
		cost := price.BuyCost(symbol, &entryCost)
		netExit := price.SellProceeds(symbol, gross)

		if cost == nil || netExit == nil || cost.Sign() <= 0 {
			return
		}

		p := netExit.Sub(*cost)
		pnl = &p
		pct := p.Div(*cost).Mul(decimalHundred)
		pnlPct = &pct
	})

	return
}

// TouchAsk is the best ask when the book is crossed-ready.
func (price *Price) TouchAsk(symbol string) *decimal.Decimal {
	var ask *decimal.Decimal

	price.Books.Book(symbol, func(managed *market.Book) {
		if !bookReady(managed) {
			return
		}

		best := managed.BestAsk()
		if best == nil || best.Price == nil || best.Price.Sign() <= 0 {
			return
		}

		ask = best.Price
	})

	return ask
}

// TouchBid is the best bid when the book is crossed-ready.
func (price *Price) TouchBid(symbol string) *decimal.Decimal {
	var bid *decimal.Decimal

	price.Books.Book(symbol, func(managed *market.Book) {
		if !bookReady(managed) {
			return
		}

		best := managed.BestBid()
		if best == nil || best.Price == nil || best.Price.Sign() <= 0 {
			return
		}

		bid = best.Price
	})

	return bid
}

// FillAsk is the ask-side VWAP for qty. Nil when the book cannot fill.
func (price *Price) FillAsk(symbol string, qty *decimal.Decimal) *decimal.Decimal {
	entryPx, _, err := price.EntryCost(symbol, qty)

	if err != nil || entryPx == nil || entryPx.Sign() <= 0 {
		return nil
	}

	return entryPx
}

// EntryCost walks the asks for quantity and returns book VWAP and fee amount.
func (price *Price) EntryCost(
	symbol string,
	quantity *decimal.Decimal,
) (*decimal.Decimal, *decimal.Decimal, error) {
	if quantity == nil || quantity.Sign() <= 0 {
		return nil, nil, errnie.Err(
			errnie.Validation, "entry cost: positive quantity required", nil,
		)
	}

	fee := price.Fee(symbol)

	var entryPx, entryFee *decimal.Decimal
	var err error

	price.Books.Book(symbol, func(managed *market.Book) {
		if !bookReady(managed) {
			err = errnie.Err(errnie.NotFound, "entry cost: book unavailable for "+symbol, nil)
			return
		}

		filled, gross, walkErr := price.consume(managed, quantity, nil, BUY)

		if walkErr != nil {
			err = errnie.Err(
				errnie.UnprocessableContent, "entry cost: book cannot fill "+symbol, walkErr,
			)
			return
		}

		total := price.afterFee(gross, fee, BUY)

		if total == nil {
			err = errnie.Err(errnie.NotFound, "entry cost: fee unavailable for "+symbol, nil)
			return
		}

		px := gross.Div(*filled)
		feeAmt := total.Sub(*gross)
		entryPx = &px
		entryFee = &feeAmt
	})

	if entryPx == nil && err == nil {
		err = errnie.Err(
			errnie.UnprocessableContent,
			"entry cost: complete executable book required for "+symbol,
			nil,
		)
	}

	return entryPx, entryFee, err
}

/*
Allocate sizes a venue-legal long from wallet (nil = wired desk balance).
Quantity is at most best-bid / TouchDivisor and at most risk.max_fraction of
cash after fees. Below QtyMin/CostMin it fails — those mins cannot be met by
"bumping" without breaking a cap that already bound the preferred size.
*/
func (price *Price) Allocate(
	symbol string,
	wallet *market.Balance,
) (*decimal.Decimal, error) {
	// Nil wallet uses the wired desk balance and skips venue mins (observe path).
	// Tier Enter passes the venue wallet so virtual/paper/real all enforce.
	enforceMins := wallet != nil

	if wallet == nil {
		wallet = price.balance
	}

	if wallet == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With(
			"symbol", symbol,
			"why", "balance is not wired",
		))
	}

	pair := price.Instrument.Pair(symbol)
	raw := wallet.Cash(pair.Quote)

	if raw == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With(
			"symbol", symbol,
			"quote", pair.Quote,
			"why", "no spendable quote cash in wallet",
		))
	}

	cash, err := decimal.NewFromString(raw.String())

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With(
			"symbol", symbol,
			"quote", pair.Quote,
			"why", "quote cash is not a decimal",
		))
	}

	if pair.QtyMin == nil || pair.CostMin == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With(
			"symbol", symbol,
			"quote", pair.Quote,
			"why", "instrument limits missing (QtyMin or CostMin unset)",
		))
	}

	fee := price.Fee(symbol)

	if fee == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With(
			"symbol", symbol,
			"quote", pair.Quote,
			"why", "taker fee unknown",
		))
	}

	budget := cash.Mul(
		decimal.NewFromFloat(system.Cfg.Risk.MaxFraction),
	)

	if budget.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With(
			"symbol", symbol,
			"quote", pair.Quote,
			"why", fmt.Sprintf(
				"risk budget is zero (cash=%s max_fraction=%v)",
				cash.String(),
				system.Cfg.Risk.MaxFraction,
			),
		))
	}

	feeRate := fee.Mul(decimalPercent)
	grossBudgetVal := budget.Div(decimalOne.Add(feeRate))
	budgetPtr, grossBudgetPtr := &budget, &grossBudgetVal

	var (
		quantity *decimal.Decimal
		reason   string
	)

	price.Books.Book(symbol, func(managed *market.Book) {
		if !bookReady(managed) {
			reason = "level3 book not ready (missing bid/ask or book crossed)"
			return
		}

		touchBid := managed.BestBid().Quantity
		if touchBid == nil || touchBid.Sign() <= 0 {
			reason = "best bid quantity missing or zero"
			return
		}

		touchCapQty := touchBid.Div(
			decimal.NewFromFloat(system.Cfg.Risk.TouchDivisor),
		)
		touchCap := &touchCapQty

		filled, gross, why := price.sizeBuy(
			managed, symbol, pair, fee, budgetPtr, grossBudgetPtr, touchCap,
		)
		if filled == nil {
			reason = why
			return
		}

		avgPx := gross.Div(*filled)
		cost := avgPx.Mul(*filled)
		underMin := filled.Cmp(*pair.QtyMin) < 0 || cost.Cmp(*pair.CostMin) < 0

		if underMin && enforceMins {
			reason = fmt.Sprintf(
				"size %s below venue minimum (qty_min=%s cost_min=%s touch_cap=%s)",
				filled.String(),
				pair.QtyMin.String(),
				pair.CostMin.String(),
				touchCap.String(),
			)
			return
		}

		quantity = filled
	})

	if quantity != nil {
		return quantity, nil
	}

	if reason == "" {
		if why := price.Books.BlockReason(symbol); why != "" {
			reason = why
		} else {
			reason = "book callback did not size"
		}
	}

	// Size denials are expected on thin books; return without ERROR spam.
	return nil, errnie.Err(
		errnie.UnprocessableContent,
		fmt.Sprintf(
			"[price] allocate failed symbol=%s quote=%s why=%s",
			symbol, pair.Quote, reason,
		),
		nil,
	)
}

// sizeBuy lifts asks under touchCap and grossBudget, then trims until fees fit budget.
func (price *Price) sizeBuy(
	managed *market.Book,
	symbol string,
	pair kraken.InstrumentPair,
	fee, budget, grossBudget, touchCap *decimal.Decimal,
) (filled, gross *decimal.Decimal, why string) {
	if touchCap == nil || touchCap.Sign() <= 0 {
		return nil, nil, "touch-bid carry cap is non-positive"
	}

	qty, err := price.formatSize(symbol, touchCap)

	if err != nil {
		return nil, nil, "touch cap format failed: " + err.Error()
	}

	// Venue step can round a dust touch-cap to zero; that book cannot be sized.
	if qty == nil || qty.Sign() <= 0 {
		return nil, nil, fmt.Sprintf(
			"touch cap below size step (gross_budget=%s touch_cap=%s)",
			grossBudget.String(), touchCap.String(),
		)
	}

	filled, gross, _ = price.consume(managed, qty, grossBudget, BUY)
	if filled == nil || filled.Sign() <= 0 {
		return nil, nil, fmt.Sprintf(
			"ask walk produced zero size (gross_budget=%s touch_cap=%s)",
			grossBudget.String(), touchCap.String(),
		)
	}

	askPx := managed.BestAsk().Price
	feeRate := fee.Mul(decimalPercent)
	fail := fmt.Sprintf(
		"cannot trim fill to fee-inclusive budget %s (filled=%s)",
		budget.String(), filled.String(),
	)

	for range 8 {
		total := price.afterFee(gross, fee, BUY)
		if total == nil {
			return nil, nil, "fee-inclusive cost unknown"
		}

		if total.Cmp(*budget) <= 0 {
			return price.venueFill(managed, symbol, filled, gross)
		}

		if askPx == nil || askPx.Sign() <= 0 {
			return nil, nil, fail
		}

		// Each base unit cut saves askPx*(1+feeRate) of fee-inclusive cash.
		cut := total.Sub(*budget).Div(askPx.Mul(decimalOne.Add(feeRate)))
		if step := pair.QtyIncrement; step != nil && step.Sign() > 0 && cut.Cmp(*step) < 0 {
			cut = *step
		}
		if cut.Sign() <= 0 || filled.Cmp(cut) <= 0 {
			return nil, nil, fail
		}

		next := filled.Sub(cut)
		nextPtr, fmtErr := price.formatSize(symbol, &next)

		if fmtErr != nil {
			return nil, nil, "budget trim format failed: " + fmtErr.Error()
		}

		if nextPtr == nil || nextPtr.Cmp(*filled) >= 0 {
			return nil, nil, fail
		}

		var walkErr error
		filled, gross, walkErr = price.consume(managed, nextPtr, nil, BUY)

		if walkErr != nil || filled == nil || filled.Sign() <= 0 {
			if walkErr != nil {
				return nil, nil, "budget trim walk failed: " + walkErr.Error()
			}

			return nil, nil, "budget trim walk failed"
		}
	}

	return nil, nil, fail
}

// venueFill snaps qty to the instrument step and re-walks so gross matches.
func (price *Price) venueFill(
	managed *market.Book,
	symbol string,
	filled, gross *decimal.Decimal,
) (*decimal.Decimal, *decimal.Decimal, string) {
	if filled == nil || filled.Sign() <= 0 {
		return nil, nil, "venue size is non-positive"
	}

	sized, err := price.formatSize(symbol, filled)

	if err != nil {
		return nil, nil, "venue size format failed: " + err.Error()
	}

	if sized == nil || sized.Sign() <= 0 {
		return nil, nil, "venue size rounded to zero"
	}

	if sized.Cmp(*filled) == 0 {
		return filled, gross, ""
	}

	next, nextGross, walkErr := price.consume(managed, sized, nil, BUY)

	if walkErr != nil || next == nil || next.Sign() <= 0 {
		if walkErr != nil {
			return nil, nil, "venue size format walk failed: " + walkErr.Error()
		}

		return nil, nil, "venue size format walk failed"
	}

	return next, nextGross, ""
}

// consume fills maxQty from the book; maxGross (if set) also caps quote spent.
func (price *Price) consume(
	managed *market.Book,
	maxQty, maxGross *decimal.Decimal,
	side Direction,
) (*decimal.Decimal, *decimal.Decimal, error) {
	if managed == nil || maxQty == nil || maxQty.Sign() <= 0 {
		return nil, nil, errnie.Err(
			errnie.Validation, "[price] valid book and positive quantity required", nil,
		)
	}

	need := *maxQty
	capped := maxGross != nil
	cash := decimalZero

	if capped {
		cash = *maxGross
	}

	level := managed.BestBid()

	if side == BUY {
		level = managed.BestAsk()
	}

	var filled, gross decimal.Decimal

	for level != nil && need.Sign() > 0 && (!capped || cash.Sign() > 0) {
		px, qty := level.Price, level.Quantity

		if side == BUY {
			level = level.Higher
		} else {
			level = level.Lower
		}

		if px == nil || qty == nil || px.Sign() <= 0 || qty.Sign() <= 0 {
			continue
		}

		take := decimal.Min(*qty, need)

		if capped {
			take = decimal.Min(take, cash.Div(*px))
		}

		if take.Sign() <= 0 {
			break
		}

		cost := px.Mul(take)
		filled, gross, need = filled.Add(take), gross.Add(cost), need.Sub(take)

		if capped {
			cash = cash.Sub(cost)
		}
	}

	if !capped && filled.Cmp(*maxQty) < 0 {
		return &filled, &gross, errnie.Err(
			errnie.UnprocessableContent, "[price] insufficient book depth", nil,
		)
	}

	return &filled, &gross, nil
}

/*
BuyCost is quote paid after taker fee on a buy notional.
*/
func (price *Price) BuyCost(symbol string, gross *decimal.Decimal) *decimal.Decimal {
	fee := price.Fee(symbol)

	if fee == nil {
		return nil
	}

	return price.afterFee(gross, fee, BUY)
}

/*
SellProceeds is quote received after taker fee on a sell notional.
*/
func (price *Price) SellProceeds(symbol string, gross *decimal.Decimal) *decimal.Decimal {
	fee := price.Fee(symbol)

	if fee == nil {
		return nil
	}

	return price.afterFee(gross, fee, SELL)
}

/*
FeeNetRoundTrip is fee-net PnL and percent from entry/exit prices at quantity.
*/
func (price *Price) FeeNetRoundTrip(
	symbol string,
	entry, qty, exit *decimal.Decimal,
) (pnl, pct *decimal.Decimal) {
	if entry == nil || entry.Sign() <= 0 ||
		qty == nil || qty.Sign() <= 0 ||
		exit == nil || exit.Sign() <= 0 {
		return nil, nil
	}

	entryCost := entry.Mul(*qty)
	netEntry := price.BuyCost(symbol, &entryCost)
	exitGross := exit.Mul(*qty)
	netExit := price.SellProceeds(symbol, &exitGross)

	if netEntry == nil || netExit == nil || netEntry.Sign() <= 0 {
		return nil, nil
	}

	p := netExit.Sub(*netEntry)
	pPct := p.Div(*netEntry).Mul(decimalHundred)

	return &p, &pPct
}

func (price *Price) afterFee(
	amount, fee *decimal.Decimal,
	direction Direction,
) *decimal.Decimal {
	if amount == nil || amount.Sign() <= 0 || fee == nil {
		return nil
	}

	rate := decimalPercent.Mul(*fee)

	if direction == SELL {
		rate = rate.Mul(decimalNegativeOne)
	}

	r := amount.Mul(decimalOne.Add(rate))
	return &r
}

/*
BackingRatio is bidCash/askCash inside the configured mid band.
*/
func (price *Price) BackingRatio(symbol string) (ratio float64, ok bool) {
	if symbol == "" {
		return 0, false
	}

	band := system.Cfg.Risk.BackingBandFrac

	price.Books.Book(symbol, func(managed *market.Book) {
		if !bookReady(managed) {
			return
		}

		mid := managed.Midpoint()
		if mid == nil || mid.Sign() <= 0 {
			return
		}

		loVal := mid.Mul(decimal.NewFromFloat(1 - band))
		hiVal := mid.Mul(decimal.NewFromFloat(1 + band))
		lo, hi := &loVal, &hiVal

		bidCash := sideCash(managed.BestBid(), true, lo, hi)
		askCash := sideCash(managed.BestAsk(), false, lo, hi)

		if askCash > 0 {
			ratio, ok = bidCash/askCash, true
		} else {
			// No ask notional in-band is an overhead vacuum, not a missing book.
			ratio, ok = math.Inf(1), true
		}
	})

	return ratio, ok
}

// sideCash sums quote notional along one book side inside [lo, hi].
func sideCash(
	start *market.Level,
	bids bool,
	lo, hi *decimal.Decimal,
) float64 {
	var total float64

	for level := start; level != nil; {
		px, qty := level.Price, level.Quantity

		if bids {
			level = level.Lower
		} else {
			level = level.Higher
		}

		if px == nil || qty == nil || px.Sign() <= 0 || qty.Sign() <= 0 {
			continue
		}

		if bids && px.Cmp(*lo) < 0 {
			break
		}

		if !bids && px.Cmp(*hi) > 0 {
			break
		}

		total += px.Mul(*qty).InexactFloat64()
	}

	return total
}

func bookReady(managed *market.Book) bool {
	if managed == nil {
		return false
	}

	bid, ask := managed.BestBid(), managed.BestAsk()
	if bid == nil || ask == nil || bid.Price == nil || ask.Price == nil {
		return false
	}

	return bid.Price.Cmp(*ask.Price) < 0
}

func toSymm(raw *sdkdecimal.Decimal) *decimal.Decimal {
	if raw == nil {
		return nil
	}

	parsed, err := decimal.NewFromString(raw.String())
	if err != nil {
		return nil
	}

	return &parsed
}

func toSDK(value *decimal.Decimal) (*sdkdecimal.Decimal, error) {
	if value == nil {
		return nil, nil
	}

	return sdkdecimal.NewFromString(value.String())
}

func (price *Price) formatSize(symbol string, value *decimal.Decimal) (*decimal.Decimal, error) {
	if price.normalizer == nil || value == nil {
		return value, nil
	}

	raw, err := toSDK(value)
	if err != nil {
		return nil, err
	}

	sized, err := price.normalizer.FormatSize(symbol, raw)
	if err != nil {
		return nil, err
	}

	return toSymm(sized), nil
}
