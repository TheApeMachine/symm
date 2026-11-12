package broker

import (
	"context"
	"fmt"
	"math"
	"strings"
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

/*
Direction selects which side consume walks and whether afterFee adds or subtracts
taker — BUY lifts asks, SELL hits bids.
*/
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
	Instrument      *Instrument
	Books           *BookManager
	ticker          *market.Ticker
	venue           market.Venue
	public          network.Client
	balance         *market.Balance
	normalizer      *spot.Normalizer
	tape            *Tape
	peakFrac        float64
	backingBand     float64
	minTouchBidCash float64
	minBackingRatio float64
	minTapeNotional float64
}

/*
queryTradeVolume is swappable so price tests avoid live REST while production
still uses the shared network client for fee bootstrap.
*/
var queryTradeVolume = func(url string, body any) ([]byte, error) {
	resp, err := network.NewREST().Post(url, body)

	if err != nil {
		return nil, err
	}

	return resp.Body(), nil
}

/*
NewPrice refuses to construct if TradeVolume fails — downstream FeeNet and
Allocate would silently use wrong economics without account taker rates.
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
		venue:      host,
		public:     public,
		normalizer: normalizer,
		tape:       NewTape(),
	}
	price.bindRisk()

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

/*
SetTicker bypasses ws ticker when tests need BelowPeak/CloseMark fallback without
running the full public Watch loop.
*/
func (price *Price) SetTicker(row *market.TickerData) {
	if row == nil {
		return
	}

	price.ticker.Set(row.Symbol, row)
}

/*
ResolveDeskSymbol bridges TradesHistory and Balance compact names to ws symbols
used as cache keys — mismatches here invent ghost or missing exchange lots.
*/
func (price *Price) ResolveDeskSymbol(pair string) string {
	if pair == "" {
		return ""
	}

	quote := system.Cfg.Market.QuoteCurrency

	if strings.Contains(pair, "/") {
		if !strings.HasSuffix(pair, "/"+quote) {
			return ""
		}

		base, found := strings.CutSuffix(pair, "/"+quote)
		if !found || base == "" {
			return ""
		}

		if mapped := price.SymbolForBase(base, quote); mapped != "" {
			return mapped
		}

		return pair
	}

	if price.normalizer != nil {
		if desk := price.normalizer.Name(pair); desk != "" && strings.Contains(desk, "/") {
			if strings.HasSuffix(desk, "/"+quote) {
				if mapped := price.SymbolForBase(strings.TrimSuffix(desk, "/"+quote), quote); mapped != "" {
					return mapped
				}

				return desk
			}
		}

		if info, err := price.normalizer.PairInfo(pair); err == nil &&
			info != nil && info.WSName != "" {
			if strings.HasSuffix(info.WSName, "/"+quote) {
				return info.WSName
			}
		}
	}

	if price.Instrument == nil {
		return ""
	}

	for _, row := range price.Instrument.Pairs() {
		if pair == row.Base+row.Quote {
			return row.Symbol
		}

		compact := strings.ReplaceAll(row.Symbol, "/", "")

		if strings.EqualFold(pair, compact) {
			return row.Symbol
		}

		if alt := krakenAltName(row.Base, row.Quote); strings.EqualFold(pair, alt) {
			return row.Symbol
		}
	}

	return ""
}

/*
SymbolForBase turns ledger asset codes into pairs for history admit when REST
names omit the slash or use alternate base spellings.
*/
func (price *Price) SymbolForBase(base, quote string) string {
	if base == "" || quote == "" || price.Instrument == nil {
		return ""
	}

	for _, row := range price.Instrument.Pairs() {
		if !assetMatches(row.Base, base) {
			continue
		}

		if assetMatches(row.Quote, quote) {
			return row.Symbol
		}
	}

	return ""
}

/*
DeskSymbolForLedgerAsset skips quote cash and maps hold assets to pairs so
spotUSDHolds can line up TradesHistory legs with Balance rows.
*/
func (price *Price) DeskSymbolForLedgerAsset(asset string) string {
	if price == nil || asset == "" {
		return ""
	}

	quote := system.Cfg.Market.QuoteCurrency

	if asset == quote || kraken.SpotAsset(asset) == quote {
		return ""
	}

	if sym := price.SymbolForBase(kraken.SpotAsset(asset), quote); sym != "" {
		return sym
	}

	if price.normalizer != nil {
		base := price.normalizer.Name(asset)

		if base != "" && !strings.Contains(base, "/") {
			if sym := price.SymbolForBase(base, quote); sym != "" {
				return sym
			}
		}
	}

	return price.ResolveDeskSymbol(strings.ToUpper(asset + quote))
}

/*
assetMatches equates XXBT with XBT because instrument cache and Balance use
different Kraken spellings for the same spendable asset.
*/
func assetMatches(code, want string) bool {
	if code == want {
		return true
	}

	return strings.EqualFold(krakenCompactAsset(code), want)
}

/*
krakenAltName is the REST pair fallback when normalizer.PairInfo misses during
ResolveDeskSymbol scans over instrument pairs.
*/
func krakenAltName(base, quote string) string {
	return krakenCompactAsset(base) + krakenCompactAsset(quote)
}

/*
krakenCompactAsset aligns websocket and REST asset spellings before suffix
comparisons in historyPairForDesk and assetMatches.
*/
func krakenCompactAsset(code string) string {
	if strings.HasPrefix(code, "XX") {
		return code[1:]
	}

	if strings.HasPrefix(code, "X") || strings.HasPrefix(code, "Z") {
		return code[1:]
	}

	return code
}

/*
VolumeUSD defers to ticker not tape because signals want venue-published 24h
stats, not the desk's rolling minute buckets.
*/
func (price *Price) VolumeUSD(symbol string) float64 {
	return price.ticker.VolumeUSD(symbol)
}

/*
ChangePct is ticker-sourced for signal features; trade prints would double-count
intra-day moves the ticker already aggregates.
*/
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
bindRisk snapshots system.Cfg risk knobs once so BookIntact and Allocate do not
hit the global config struct on every symbol tick.
*/
func (price *Price) bindRisk() {
	price.peakFrac = system.Cfg.Risk.MaxEntryPeakFrac
	price.backingBand = system.Cfg.Risk.BackingBandFrac
	price.minTouchBidCash = system.Cfg.Risk.MinTouchBidCash
	price.minBackingRatio = system.Cfg.Risk.MinBackingRatio
	price.minTapeNotional = system.Cfg.Risk.MinTapeNotionalUSD
}

/*
BelowPeak is true when last sits under max_entry_peak_frac of the 24h range.
Disabled when the cap is <=0 or >=1. Missing range data vetoes while enabled.
*/
func (price *Price) BelowPeak(symbol string) bool {
	cap := price.peakFrac

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
MoverVolumeOK accepts either venue 24h quote turnover or accelerating desk tape.
*/
func (price *Price) MoverVolumeOK(symbol string) bool {
	risk := system.Cfg.Risk

	if price.VolumeUSD(symbol) >= risk.MinMoverVolumeUSD {
		return true
	}

	recent, prior := price.TapeUptake(symbol, time.Now())
	floor := price.minTapeNotional

	if risk.MinTapeNotionalUSD > 0 {
		floor = risk.MinTapeNotionalUSD
	}

	return recent >= floor && recent > prior
}

/*
EntryPeakOK applies accumulation vs ignition range caps from selection config.
*/
func (price *Price) EntryPeakOK(symbol string, ignition bool) bool {
	sel := system.Cfg.Selection
	cap := sel.MaxAccumPeakFrac

	if ignition {
		cap = sel.MaxIgnitionPeakFrac
	}

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
PulseSymbols returns the next heartbeat slice over the online L3 universe.
*/
func (price *Price) PulseSymbols(batch, start int) ([]string, int) {
	if price.Instrument == nil || batch <= 0 {
		return nil, start
	}

	all := price.Instrument.Symbols()

	if len(all) == 0 {
		return nil, start
	}

	if batch > len(all) {
		batch = len(all)
	}

	if start < 0 {
		start = 0
	}

	start %= len(all)
	out := make([]string, 0, batch)

	for i := range batch {
		out = append(out, all[(start+i)%len(all)])
	}

	next := (start + batch) % len(all)

	return out, next
}

/*
PulseQuote is last mark plus touch bid/ask for book-heartbeat ignition inputs.
*/
func (price *Price) PulseQuote(symbol string) (last, bid, ask float64, ok bool) {
	if symbol == "" {
		return 0, 0, 0, false
	}

	mark := price.CloseMark(symbol)

	if mark == nil || mark.Sign() <= 0 {
		row := price.ticker.Of(symbol)

		if row == nil || row.Last <= 0 {
			return 0, 0, 0, false
		}

		last = row.Last
	} else {
		last = mark.InexactFloat64()
	}

	price.Books.Book(symbol, func(managed *market.Book) {
		if !bookReady(managed) {
			return
		}

		bestBid := managed.BestBid()
		bestAsk := managed.BestAsk()

		if bestBid == nil || bestAsk == nil ||
			bestBid.Price == nil || bestAsk.Price == nil {
			return
		}

		bid = bestBid.Price.InexactFloat64()
		ask = bestAsk.Price.InexactFloat64()

		if bid > 0 && ask > bid {
			ok = true
		}
	})

	if !ok {
		return 0, 0, 0, false
	}

	return last, bid, ask, true
}

/*
NoteTrade feeds BookIntact tape uptake — prints alone are too noisy per tick, so
we aggregate quote dollars per minute before comparing recent vs prior windows.
*/
func (price *Price) NoteTrade(trade *market.TradeData) {
	if trade == nil {
		return
	}

	at := trade.Timestamp

	if at.IsZero() {
		at = time.Now()
	}

	if price.tape == nil {
		return
	}

	price.tape.Note(trade.Symbol, trade.Price.Mul(trade.Qty).InexactFloat64(), at)
}

/*
TapeUptake is the strategy-facing export of tape windows — BookIntact wants both
windows, not the raw bucket ring.
*/
func (price *Price) TapeUptake(symbol string, now time.Time) (recent, prior float64) {
	return price.tape.Uptake(symbol, now)
}

/*
applyTicker rekeys ws pair ids through normalizer.Name so ticker rows align with
instrument ws symbols used everywhere else on the desk.
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

		symbol := pair

		if name := price.normalizer.Name(pair); name != "" {
			symbol = name
		}

		price.ticker.Set(symbol, row)
	}
}

/*
loadFees must run before READY because default fee tables would bias BookIntact
open-drag and every FeeNet settle on this API key's actual taker tier.
*/
func (price *Price) loadFees() bool {
	symbols := price.Instrument.Symbols()

	if len(symbols) == 0 {
		price.Error(errnie.Err(
			errnie.Internal, "trade volume fees need a non-empty symbol list", nil,
		))
		return false
	}

	raw, err := queryTradeVolume(
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

/*
Fee returns nil when unknown so callers fail closed — sizing and friction must
not assume a default taker when TradeVolume missed a symbol.
*/
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
TouchBidCash approximates near-touch exit liquidity — top three levels bound how
fast a desk lot could be monetized without walking deep bids.
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

/*
SpreadPct uses the managed book spread, not ticker last — Observe wants
microstructure at decision time, not delayed public stats.
*/
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

/*
PnLPct is the close-now probe EntryCloseNowPnLPct uses — same MarkClose fee
netting as open marks without duplicating the walk logic at call sites.
*/
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

/*
ExitVWAP exposes only the walk price for callers that compute PnL elsewhere and
must not double-run MarkClose's fee leg math.
*/
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

/*
TouchAsk refuses crossed or empty books so signals do not treat a stale ask as
executable when the touch is invalid.
*/
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

/*
MarkPaused mirrors BookManager during level3 gaps so positions and desk can
degrade marks without treating disconnect as an application fault.
*/
func (price *Price) MarkPaused(symbol string) bool {
	if price == nil || price.Books == nil {
		return false
	}

	return price.Books.MarkPaused(symbol)
}

/*
CloseMark is the bid used to fee-net an open long: level3 touch, else public last.
*/
func (price *Price) CloseMark(symbol string) *decimal.Decimal {
	if mark := price.TouchBid(symbol); mark != nil {
		return mark
	}

	row := price.ticker.Of(symbol)

	if row != nil && row.Last > 0 {
		m := decimal.NewFromFloat(row.Last)
		return &m
	}

	return nil
}

/*
TouchBid is the mark for longs — trade prints are not exit prices; marks and
stoploss must use the bid the desk could actually hit.
*/
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

/*
FillAsk wraps EntryCost for local tier fillsEnter — nil means the book cannot
honor the Enter volume before we fabricate an Apply print.
*/
func (price *Price) FillAsk(symbol string, qty *decimal.Decimal) *decimal.Decimal {
	entryPx, _, err := price.EntryCost(symbol, qty)

	if err != nil || entryPx == nil || entryPx.Sign() <= 0 {
		return nil
	}

	return entryPx
}

/*
EntryCost separates executable VWAP from fee leg for probes that need both
open-drag (close-now) and cash budgeting without a full Allocate pass.
*/
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

	if price.Instrument != nil && !price.Instrument.Has(symbol) {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With(
			"symbol", symbol,
			"why", "symbol not in instrument universe",
		)
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
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With(
			"symbol", symbol,
			"quote", pair.Quote,
			"why", "no spendable quote cash in wallet",
		)
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
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[price] allocate failed",
			nil,
		).With(
			"symbol", symbol,
			"quote", pair.Quote,
			"why", "taker fee unknown",
		)
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

/*
sizeBuy implements the risk caps inside Allocate: touch-divisor limits impact,
then fee-inclusive trim so Send never exceeds spendable quote after taker.
*/
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

/*
venueFill re-walks after lot rounding because Kraken rejects off-step qty — the
post-format book walk must match the volume we will actually send.
*/
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

/*
consume is the shared depth walk for buy and sell; maxGross bounds quote spend
during sizing, while exit walks require full depth to mark honestly.
*/
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
BuyCost grosses up buys for MarkClose fee-net PnL — entry spend includes taker
on the entry leg only.
*/
func (price *Price) BuyCost(symbol string, gross *decimal.Decimal) *decimal.Decimal {
	fee := price.Fee(symbol)

	if fee == nil {
		return nil
	}

	return price.afterFee(gross, fee, BUY)
}

/*
SellProceeds nets down sells for MarkClose — exit proceeds use taker on the
sell leg, paired with BuyCost for round-trip PnL.
*/
func (price *Price) SellProceeds(symbol string, gross *decimal.Decimal) *decimal.Decimal {
	fee := price.Fee(symbol)

	if fee == nil {
		return nil
	}

	return price.afterFee(gross, fee, SELL)
}

/*
FeeNetRoundTrip is how Apply stamps closed lots and how exchange history
finalizeClosedLot aligns with symm-managed settle economics.
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

/*
afterFee keeps fee direction in one place so BuyCost/SellProceeds and sizing
trim share the same sign convention as Kraken's percent fee display.
*/
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

	band := price.backingBand

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

/*
sideCash measures liquidity near mid for BackingRatio — only notional inside
the band counts, not the full depth subscription.
*/
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

/*
bookReady vetoes spread, touch, and walk helpers when checksum passed but the
touch is crossed — otherwise Friction and marks lie.
*/
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

/*
toSymm bridges normalizer output into desk decimals without letting SDK mutability
leak into Position JSON and cache blobs.
*/
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

/*
toSDK is the inverse bridge for FormatSize input — shopspring remains canonical
in broker, SDK only at the formatting boundary.
*/
func toSDK(value *decimal.Decimal) (*sdkdecimal.Decimal, error) {
	if value == nil {
		return nil, nil
	}

	return sdkdecimal.NewFromString(value.String())
}

/*
formatSize prevents add_order reject on qty_increment — Allocate calls this on
touch cap and every budget trim step.
*/
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

/*
BookIntact is the book-oracle input for Observe: cash, backing, tape uptake,
and close-now vs open-drag. Spread and peak are separate Observe args.
*/
func (price *Price) BookIntact(symbol string) bool {
	minCash := price.minTouchBidCash
	cash, ok := price.TouchBidCash(symbol)

	if !ok || cash == nil || cash.InexactFloat64() < minCash {
		return false
	}

	ratio, ok := price.BackingRatio(symbol)

	if !ok || ratio < price.minBackingRatio {
		return false
	}

	recent, prior := price.TapeUptake(symbol, time.Now())
	floor := price.minTapeNotional

	if recent < floor || recent <= prior {
		return false
	}

	closeNow, ok := price.EntryCloseNowPnLPct(symbol)

	if !ok || closeNow == nil {
		return false
	}

	return closeNow.InexactFloat64() >= -price.OpenDragCap(symbol)
}
