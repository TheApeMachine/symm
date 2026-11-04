package broker

import (
	"context"
	"fmt"
	"strings"
	"sync"
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

const tickerRefresh = time.Minute

/*
EntryFill is a complete buy against the ask book at admission time.
*/
type EntryFill struct {
	EntryPrice *decimal.Decimal
	EntryFee   *decimal.Decimal
}

/*
Price answers book questions with fills. A last trade is not a price.
*/
type Price struct {
	*runtime.System
	Instrument *Instrument
	Books      *BookManager
	ticker     sync.Map
	balance    *market.Balance
	normalizer *spot.Normalizer
	tape       *Tape
}

type tickerSnap struct {
	volumeUSD float64
	changePct float64
	high24    float64
	low24     float64
	last      float64
}

func NewPrice(
	ctx context.Context,
	books *BookManager,
	instrument *Instrument,
	normalizer *spot.Normalizer,
) *Price {
	if normalizer == nil {
		normalizer = spot.NewNormalizer()
	}

	price := &Price{
		System:     runtime.NewSystem(ctx, "price"),
		Instrument: instrument,
		Books:      books,
		normalizer: normalizer,
		tape:       NewTape(),
	}

	if !price.loadFees() {
		return nil
	}

	price.refreshTicker()
	price.pollTicker()
	price.Transition(runtime.READY)

	return price
}

func (price *Price) SetTicker(
	symbol string,
	volumeUSD, changePct, high24, low24, last float64,
) {
	price.ticker.Store(symbol, tickerSnap{
		volumeUSD: volumeUSD,
		changePct: changePct,
		high24:    high24,
		low24:     low24,
		last:      last,
	})
}

func (price *Price) tickerOf(symbol string) tickerSnap {
	raw, ok := price.ticker.Load(symbol)
	if !ok {
		return tickerSnap{}
	}

	return raw.(tickerSnap)
}

func (price *Price) VolumeUSD(symbol string) float64 { return price.tickerOf(symbol).volumeUSD }
func (price *Price) ChangePct(symbol string) float64 { return price.tickerOf(symbol).changePct }

/*
PeakFrac is last's position in the 24h high–low range (0 at low, 1 at high).
*/
func (price *Price) PeakFrac(symbol string) (float64, bool) {
	snap := price.tickerOf(symbol)
	span := snap.high24 - snap.low24

	if span <= 0 || snap.last <= 0 {
		return 0, false
	}

	return (snap.last - snap.low24) / span, true
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

func (price *Price) refreshTicker() {
	response, err := network.NewREST().Get(
		"https://api.kraken.com"+system.Cfg.WebSocket.Endpoints.Ticker,
		map[string]string{"assetVersion": "1"},
	)
	if err != nil {
		errnie.Warn("[price] ticker poll failed", "err", err)
		return
	}

	rows, err := kraken.ParseTicker(response.Body())
	if err != nil {
		errnie.Warn("[price] ticker parse failed", "err", err)
		return
	}

	for pair, row := range rows {
		symbol := pair

		if name := price.normalizer.Name(pair); name != "" {
			symbol = name
		}

		price.SetTicker(
			symbol,
			row.QuoteVolume24h(),
			row.ChangePct(),
			row.High24h(),
			row.Low24h(),
			row.LastTrade(),
		)
	}
}

func (price *Price) pollTicker() {
	go func() {
		ticker := time.NewTicker(tickerRefresh)
		defer ticker.Stop()

		for {
			select {
			case <-price.Context().Done():
				return
			case <-ticker.C:
				price.refreshTicker()
			}
		}
	}()
}

/*
primeNormalizer loads public Assets + AssetPairs (legacy and assetVersion=1)
so TradeVolume keys like XXRPZUSD resolve to websocket symbols like XRP/USD.
*/
func (price *Price) primeNormalizer() bool {
	if err := price.normalizer.Use(spot.NewREST()); err != nil {
		price.Error(errnie.Err(
			errnie.IO,
			"[price] tradable asset pairs required before trade volume fees",
			err,
		))
		return false
	}

	return true
}

/*
feeSymbol maps a TradeVolume fee key (internal, altname, or ws name) onto the
instrument's websocket symbol. Matching starts from the desk universe so Kraken
concat keys like GALAUSD are not mis-split as GAL/AUSD by alias search.
*/
func (price *Price) feeSymbol(pair string) string {
	if pair == "" {
		return ""
	}

	if price.Instrument.Has(pair) {
		return pair
	}

	compact := strings.ToUpper(strings.ReplaceAll(pair, "/", ""))

	for _, symbol := range price.Instrument.Symbols() {
		if strings.EqualFold(symbol, pair) ||
			strings.ToUpper(strings.ReplaceAll(symbol, "/", "")) == compact {
			return symbol
		}

		inst := price.Instrument.Pair(symbol)
		if inst.Base != "" && strings.EqualFold(inst.Base+inst.Quote, pair) {
			return symbol
		}

		info, err := price.normalizer.PairInfo(symbol)
		if err != nil {
			continue
		}

		if strings.EqualFold(info.WSName, pair) || strings.EqualFold(info.AltName, pair) {
			return symbol
		}
	}

	// Internal keys (XXRPZUSD) resolve through the primed AssetPairs aliases.
	if named := price.normalizer.Name(pair); price.Instrument.Has(named) {
		return named
	}

	for _, symbol := range price.Instrument.Symbols() {
		if price.normalizer.Name(symbol) == price.normalizer.Name(pair) {
			return symbol
		}
	}

	return ""
}

func (price *Price) loadFees() bool {
	symbols := price.Instrument.Symbols()
	if len(symbols) == 0 {
		price.Error(errnie.Err(
			errnie.Internal, "trade volume fees need a non-empty symbol list", nil,
		))
		return false
	}

	if !price.primeNormalizer() {
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

	n := 0
	for pair, fee := range volume.Fees {
		if fee.Fee == nil {
			continue
		}

		symbol := price.feeSymbol(pair)
		if symbol == "" {
			continue
		}

		price.Instrument.RememberFee(symbol, fee.Fee)
		n++
	}

	if n == 0 {
		price.Error(errnie.Err(
			errnie.UnprocessableContent, "trade volume returned no usable taker fees", nil,
		))
		return false
	}

	missing := 0
	for _, symbol := range symbols {
		if price.Fee(symbol) == nil {
			missing++
		}
	}

	if missing > 0 {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"[price] trade volume fees incomplete for instrument universe",
			nil,
		).With("mapped", n, "missing", missing, "universe", len(symbols)))
	}

	return true
}

// Fee is the taker fee in percent (0.40 means 0.40%). Nil means unknown.
func (price *Price) Fee(symbol string) *decimal.Decimal {
	if fee := price.Instrument.TakerFee(symbol); fee != nil {
		return fee
	}

	return price.Instrument.TakerFee(price.normalizer.Name(symbol))
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
EntryCloseNowPnLPct is fee-net percent PnL of opening the desk lot at the ask
walk and closing immediately into the bids.
*/
func (price *Price) EntryCloseNowPnLPct(symbol string) (*decimal.Decimal, bool) {
	qty, _ := price.Allocate(symbol, nil)

	if qty == nil || qty.Sign() <= 0 {
		return nil, false
	}

	fill, err := price.EntryCost(symbol, qty)

	if err != nil || fill == nil || fill.EntryPrice == nil || fill.EntryPrice.Sign() <= 0 {
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

	pct := price.PnLPct(symbol, qty, fill.EntryPrice, price.Fee(symbol))

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
	quantity, entry, fee *decimal.Decimal,
) *decimal.Decimal {
	if fee == nil {
		fee = price.Fee(symbol)
	}

	return price.roundTripPct(symbol, quantity, entry, fee, fee)
}

func (price *Price) ExitVWAP(symbol string, quantity *decimal.Decimal) *decimal.Decimal {
	vwap, _, _, _ := price.closeNow(symbol, quantity, nil, nil)
	return vwap
}

/*
closeNow walks the bid book once for exit VWAP, room, and fee-net PnL.
*/
func (price *Price) closeNow(
	symbol string,
	quantity, entry, fee *decimal.Decimal,
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

		filled, gross, err := price.consume(managed, quantity, nil, SELL)
		if err != nil || filled == nil || gross == nil || filled.Sign() <= 0 {
			return
		}

		v := gross.Div(*filled)
		vwap = &v

		if entry == nil || entry.Sign() <= 0 || fee == nil {
			return
		}

		entryCost := entry.Mul(*quantity)
		cost := price.afterFee(&entryCost, fee, BUY)
		netExit := price.afterFee(gross, fee, SELL)
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

// FillAsk is the ask-side VWAP for qty, falling back to the touch ask.
func (price *Price) FillAsk(symbol string, qty *decimal.Decimal) *decimal.Decimal {
	if fill, err := price.EntryCost(symbol, qty); err == nil &&
		fill != nil && fill.EntryPrice != nil && fill.EntryPrice.Sign() > 0 {
		return fill.EntryPrice
	}

	return price.TouchAsk(symbol)
}

// FillBid is the bid-side VWAP for qty, falling back to the touch bid.
func (price *Price) FillBid(symbol string, qty *decimal.Decimal) *decimal.Decimal {
	if vwap := price.ExitVWAP(symbol, qty); vwap != nil && vwap.Sign() > 0 {
		return vwap
	}

	return price.TouchBid(symbol)
}

// EntryCost walks the asks for quantity and returns fill VWAP plus entry fee.
func (price *Price) EntryCost(
	symbol string,
	quantity *decimal.Decimal,
) (*EntryFill, error) {
	if quantity == nil || quantity.Sign() <= 0 {
		return nil, errnie.Err(
			errnie.Validation, "entry cost: positive quantity required", nil,
		)
	}

	fee := price.Fee(symbol)

	var fill *EntryFill
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

		entryPx := gross.Div(*filled)
		entryFee := total.Sub(*gross)

		fill = &EntryFill{
			EntryPrice: &entryPx,
			EntryFee:   &entryFee,
		}
	})

	if fill == nil && err == nil {
		err = errnie.Err(
			errnie.UnprocessableContent,
			"entry cost: complete executable book required for "+symbol,
			nil,
		)
	}

	return fill, err
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
	// A nil wallet falls back to the desk balance and skips venue mins.
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
		raw = wallet.Cash(wallet.Quote)
	}

	if raw == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With(
			"symbol", symbol,
			"quote", pair.Quote,
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
		why := "taker fee unknown"
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With(
			"symbol", symbol,
			"quote", pair.Quote,
			"why", why,
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

	qty := touchCap
	if sized, err := price.formatSize(symbol, qty); err == nil {
		qty = sized
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
			return filled, gross, ""
		}
		if askPx == nil || askPx.Sign() <= 0 {
			return nil, nil, fail
		}

		cut := total.Sub(*budget).Div(*askPx)
		if step := pair.QtyIncrement; step != nil && step.Sign() > 0 && cut.Cmp(*step) < 0 {
			cut = *step
		}
		if cut.Sign() <= 0 || filled.Cmp(cut) <= 0 {
			return nil, nil, fail
		}

		next := filled.Sub(cut)
		nextPtr := &next
		if sized, err := price.formatSize(symbol, nextPtr); err == nil {
			nextPtr = sized
		}
		if nextPtr.Cmp(*filled) >= 0 {
			return nil, nil, fail
		}

		var err error
		filled, gross, err = price.consume(managed, nextPtr, nil, BUY)
		if err != nil || filled == nil || filled.Sign() <= 0 {
			if err != nil {
				return nil, nil, "budget trim walk failed: " + err.Error()
			}
			return nil, nil, "budget trim walk failed"
		}
	}

	return nil, nil, fail
}

func (price *Price) roundTrip(
	symbol string,
	quantity, entry, entryFee, exitFee *decimal.Decimal,
) (profit, cost *decimal.Decimal) {
	if quantity == nil || entry == nil ||
		quantity.Sign() <= 0 || entry.Sign() <= 0 ||
		entryFee == nil || exitFee == nil {
		return nil, nil
	}

	_, _, profit, _ = price.closeNow(symbol, quantity, entry, entryFee)
	if profit == nil {
		return nil, nil
	}

	entryCost := entry.Mul(*quantity)
	cost = price.afterFee(&entryCost, entryFee, BUY)
	return profit, cost
}

func (price *Price) roundTripPct(
	symbol string,
	quantity, entry, entryFee, exitFee *decimal.Decimal,
) *decimal.Decimal {
	if quantity == nil || entry == nil ||
		quantity.Sign() <= 0 || entry.Sign() <= 0 ||
		entryFee == nil || exitFee == nil {
		return nil
	}

	_, _, _, pct := price.closeNow(symbol, quantity, entry, entryFee)
	return pct
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
