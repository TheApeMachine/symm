package broker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	sdkdecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
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

// defaultTakerFeePercent is used when the account schedule has no fee for a pair.
const defaultTakerFeePercent = 0.8

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
	Books      *Book
	ticker     sync.Map
	balance    *market.Balance
	normalizer *spot.Normalizer
}

type tickerSnap struct {
	volumeUSD float64
	changePct float64
}

func NewPrice(
	ctx context.Context,
	books *Book,
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
	}

	if !price.loadFees() {
		return nil
	}

	price.refreshTicker()
	price.pollTicker()
	price.Transition(runtime.READY)

	return price
}

func (price *Price) SetTicker(symbol string, volumeUSD, changePct float64) {
	price.ticker.Store(symbol, tickerSnap{volumeUSD, changePct})
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

		price.SetTicker(symbol, row.QuoteVolume24h(), row.ChangePct())
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

func (price *Price) loadFees() bool {
	symbols := price.Instrument.Symbols()
	if len(symbols) == 0 {
		price.Error(errnie.Err(
			errnie.Internal, "trade volume fees need a non-empty symbol list", nil,
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

	n := 0
	for pair, fee := range volume.Fees {
		if fee.Fee == nil {
			continue
		}

		price.Instrument.RememberFee(price.normalizer.Name(pair), fee.Fee)
		n++
	}

	if n == 0 {
		price.Error(errnie.Err(
			errnie.UnprocessableContent, "trade volume returned no usable taker fees", nil,
		))
		return false
	}

	return true
}

// Fee is the taker fee in percent (0.40 means 0.40%). Nil means unknown.
func (price *Price) Fee(symbol string) *decimal.Decimal {
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

	price.Books.Book(symbol, func(managed *book.Book) {
		if managed == nil || managed.BestBid() == nil {
			return
		}

		depth = toSymm(managed.BestBid().Quantity)
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

	price.Books.Book(symbol, func(managed *book.Book) {
		if managed == nil || managed.BestBid() == nil {
			return
		}

		total := decimalZero

		for n, level := 0, managed.BestBid(); n < 3 && level != nil; n, level = n+1, level.Lower {
			px, qty := toSymm(level.Price), toSymm(level.Quantity)
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

	price.Books.Book(symbol, func(managed *book.Book) {
		if !bookReady(managed) {
			return
		}

		spread = toSymm(managed.Spread())
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
		return nil, false
	}

	pct := price.PnLPct(symbol, qty, fill.EntryPrice, price.Fee(symbol))

	if pct == nil {
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
	if quantity == nil || quantity.Sign() <= 0 {
		return nil
	}

	var vwap *decimal.Decimal

	price.Books.Book(symbol, func(managed *book.Book) {
		if !bookReady(managed) {
			return
		}

		filled, gross, err := price.consume(managed, quantity, nil, SELL)

		if err != nil || filled == nil || gross == nil || filled.Sign() <= 0 {
			return
		}

		v := gross.Div(*filled)
		vwap = &v
	})

	return vwap
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

	price.Books.Book(symbol, func(managed *book.Book) {
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
) (*decimal.Decimal, string) {
	if wallet == nil {
		wallet = price.balance
	}

	if wallet == nil {
		return nil, "balance is not wired"
	}

	pair := price.Instrument.Pair(symbol)
	raw := wallet.Cash(pair.Quote)

	if raw == nil {
		raw = wallet.Cash(wallet.Quote)
	}

	if raw == nil {
		return nil, fmt.Sprintf(
			"quote cash unknown (pair quote %q, desk quote %q)",
			pair.Quote,
			wallet.Quote,
		)
	}

	cash, err := decimal.NewFromString(raw.String())
	if err != nil {
		return nil, "quote cash is not a decimal"
	}

	if pair.QtyMin == nil || pair.CostMin == nil {
		return nil, "instrument limits missing (QtyMin or CostMin unset)"
	}

	fee := price.Fee(symbol)

	budget := cash.Mul(
		decimal.NewFromFloat(system.Cfg.Risk.MaxFraction),
	)

	if budget.Sign() <= 0 {
		return nil, fmt.Sprintf(
			"risk budget is zero (cash=%s max_fraction=%v)",
			cash.String(),
			system.Cfg.Risk.MaxFraction,
		)
	}

	feeRate := fee.Mul(decimalPercent)
	grossBudgetVal := budget.Div(decimalOne.Add(feeRate))
	budgetPtr, grossBudgetPtr := &budget, &grossBudgetVal

	var (
		quantity *decimal.Decimal
		reason   string
	)

	price.Books.Book(symbol, func(managed *book.Book) {
		if !bookReady(managed) {
			reason = "level3 book not ready (missing bid/ask or book crossed)"
			return
		}

		touchBid := toSymm(managed.BestBid().Quantity)
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

		if underMin && wallet != nil {
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
		return quantity, ""
	}

	if reason == "" {
		if why := price.Books.BlockReason(symbol); why != "" {
			reason = why
		} else {
			reason = "book callback did not size"
		}
	}

	return nil, reason
}

// sizeBuy lifts asks under touchCap and grossBudget, then trims until fees fit budget.
func (price *Price) sizeBuy(
	managed *book.Book,
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

	filled, gross, _ = price.consume(managed, qty, grossBudget, BUY)
	if filled == nil || filled.Sign() <= 0 {
		return nil, nil, fmt.Sprintf(
			"ask walk produced zero size (gross_budget=%s touch_cap=%s)",
			grossBudget.String(), touchCap.String(),
		)
	}

	askPx := toSymm(managed.BestAsk().Price)
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

	var grossExit *decimal.Decimal

	price.Books.Book(symbol, func(managed *book.Book) {
		if !bookReady(managed) {
			return
		}

		filled, gross, err := price.consume(managed, quantity, nil, SELL)
		if err != nil || filled == nil || gross == nil {
			return
		}

		grossExit = gross
	})

	if grossExit == nil {
		return nil, nil
	}

	entryCost := entry.Mul(*quantity)
	cost = price.afterFee(&entryCost, entryFee, BUY)
	netExit := price.afterFee(grossExit, exitFee, SELL)
	if cost == nil || netExit == nil {
		return nil, nil
	}

	p := netExit.Sub(*cost)
	return &p, cost
}

func (price *Price) roundTripPct(
	symbol string,
	quantity, entry, entryFee, exitFee *decimal.Decimal,
) *decimal.Decimal {
	profit, cost := price.roundTrip(symbol, quantity, entry, entryFee, exitFee)
	if profit == nil || cost == nil || cost.Sign() <= 0 {
		return nil
	}

	pct := profit.Div(*cost).Mul(decimalHundred)
	return &pct
}

// consume fills maxQty from the book; maxGross (if set) also caps quote spent.
func (price *Price) consume(
	managed *book.Book,
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
		px, qty := toSymm(level.Price), toSymm(level.Quantity)

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

	price.Books.Book(symbol, func(managed *book.Book) {
		if !bookReady(managed) {
			return
		}

		mid := toSymm(managed.Midpoint())
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
	start *book.Level,
	bids bool,
	lo, hi *decimal.Decimal,
) float64 {
	var total float64

	for level := start; level != nil; {
		px, qty := toSymm(level.Price), toSymm(level.Quantity)

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

func bookReady(managed *book.Book) bool {
	if managed == nil {
		return false
	}

	bid, ask := managed.BestBid(), managed.BestAsk()
	if bid == nil || ask == nil || bid.Price == nil || ask.Price == nil {
		return false
	}

	return bid.Price.Cmp(ask.Price) < 0
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
