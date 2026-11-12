package broker

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
historyMinGap avoids hammering TradesHistory on every balance twitch while still
letting urgent syncs run after closes or fills.
*/
const historyMinGap = 45 * time.Second

/*
pullHistory is the REAL account ledger sync. Each run:

 1. POST /0/private/Balance and /0/private/TradeBalance (spot wallet checksum)
 2. POST /0/private/TradesHistory with cursor pagination (Kraken default limit 50, max 100)
 3. Rebuild exchange-path opens from fills; admit only when Balance matches net qty

symm-entered lots (non-exchange entry_path) are unchanged — stops and exits stay on live fills.
*/
func (tier *Tier) pullHistory(urgent bool) {
	if tier.Type != TierTypeReal {
		return
	}

	if tier.private == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[broker.Tier] trades history skipped: private transport is nil",
			nil,
		).With("urgent", urgent))

		return
	}

	if !urgent {
		tier.historyMu.Lock()
		if time.Since(tier.historySyncedAt) < historyMinGap {
			tier.historyMu.Unlock()
			return
		}
		tier.historyMu.Unlock()
	}

	if !tier.historyRunning.CompareAndSwap(false, true) {
		errnie.Error(errnie.Err(
			errnie.Conflict,
			"[broker.Tier] trades history skipped: sync already in flight",
			nil,
		).With("urgent", urgent))

		return
	}

	defer tier.historyRunning.Store(false)

	tier.syncExchangeBalances()

	url := system.Cfg.WebSocket.Endpoints.TradesHistory

	if url == "" {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[broker.Tier] trades history skipped: endpoint url is empty",
			nil,
		))

		return
	}

	delay := 2 * time.Second

	for attempt := 0; attempt < 8; attempt++ {
		if attempt > 0 {
			time.Sleep(delay)

			if delay < 30*time.Second {
				delay *= 2
			}
		}

		merged, ok, rateLimited := tier.fetchHistory(url)

		if ok {
			tier.foldHistory(merged)

			tier.historyMu.Lock()
			tier.historySyncedAt = time.Now()
			tier.historyMu.Unlock()

			if len(merged) == 0 {
				errnie.Error(errnie.Err(
					errnie.NotFound,
					"[broker.Tier] trades history returned no rows",
					nil,
				).With("url", url))
				tier.publishHistorySync("empty", map[string]any{"url": url})
			}

			return
		}

		if !rateLimited {
			errnie.Error(errnie.Err(
				errnie.IO,
				"[broker.Tier] trades history fetch failed",
				nil,
			).With("url", url, "attempt", attempt+1, "urgent", urgent))
			tier.publishHistorySync("fetch_failed", map[string]any{
				"url":     url,
				"attempt": attempt + 1,
			})

			return
		}
	}

	errnie.Error(errnie.Err(
		errnie.IO,
		"[broker.Tier] trades history still rate-limited after retries",
		nil,
	).With("url", url))
	tier.publishHistorySync("rate_limited", map[string]any{"url": url})
}

/*
publishHistorySync gives operators visibility when exchange admit fails — folded
lots may be empty without this signal on the History tab.
*/
func (tier *Tier) publishHistorySync(status string, detail map[string]any) {
	if tier == nil || status == "" || ui.UI == nil || !ui.UI.Active() {
		return
	}

	payload := map[string]any{
		"status": status,
		"at":     time.Now().UTC().Format(time.RFC3339Nano),
	}

	for key, value := range detail {
		payload[key] = value
	}

	ui.UI.WriteJSON(map[string]any{
		"history_sync": payload,
	})
}

/*
fetchHistory merges cursor pages because Kraken defaults to 50 rows — partial
fetch would under-count net qty and reject valid exchange holds.
*/
func (tier *Tier) fetchHistory(url string) (
	merged map[string]kraken.HistoryTrade,
	ok bool,
	rateLimited bool,
) {
	merged = make(map[string]kraken.HistoryTrade)
	cursor := ""

	for page := 0; page < 10_000; page++ {
		req := map[string]any{
			"type":        "all",
			"trades":      false,
			"with_cursor": true,
			"limit":       100,
		}

		if cursor != "" {
			req["cursor"] = cursor
		}

		raw, err := tier.private.Post(url, req)

		if err != nil || len(raw) == 0 {
			if err != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					"[broker.Tier] trades history request failed",
					err,
				).With("url", url, "page", page))
			} else {
				errnie.Error(errnie.Err(
					errnie.IO,
					"[broker.Tier] trades history empty response body",
					nil,
				).With("url", url, "page", page))
			}

			return nil, false, false
		}

		var page struct {
			Error  []any                      `json:"error"`
			Result kraken.TradesHistoryResult `json:"result"`
		}

		if err := sonic.Unmarshal(raw, &page); err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[broker.Tier] trades history decode failed",
				err,
			))

			return nil, false, false
		}

		if len(page.Error) > 0 {
			if kraken.RateLimited(page.Error...) {
				return nil, false, true
			}

			errnie.Error(errnie.Err(
				errnie.IO,
				"[broker.Tier] trades history venue error",
				nil,
			).With("error", page.Error))

			return nil, false, false
		}

		n := len(page.Result.Trades)

		if n == 0 {
			break
		}

		maps.Copy(merged, page.Result.Trades)

		next := page.Result.Cursor.Next

		if next == "" || n == 0 {
			break
		}

		cursor = next
	}

	return merged, true, false
}

/*
foldHistory is how REAL learns positions opened outside symm — admits only when
folded qty matches Balance hold, leaving symm_entered lots untouched.
*/
func (tier *Tier) foldHistory(trades map[string]kraken.HistoryTrade) {
	tier.clearExchangeClosed()
	tier.clearHistoryOpenLots()

	quote := system.Cfg.Market.QuoteCurrency
	bySymbol := make(map[string][]*market.ExecutionData)

	var skippedVolume int
	var skippedForeign int
	var unresolvedPairs []string

	for txid, trade := range trades {
		fill := historyFill(txid, trade)

		if fill == nil {
			skippedVolume++
			continue
		}

		symbol := tier.deskSymbol(trade.Pair)

		if symbol == "" || !strings.HasSuffix(symbol, "/"+quote) {
			if trade.Pair != "" && !historyPairForDesk(trade.Pair, quote) {
				skippedForeign++
			} else {
				unresolvedPairs = append(unresolvedPairs, trade.Pair)
			}

			continue
		}

		fill.Symbol = symbol
		bySymbol[symbol] = append(bySymbol[symbol], fill)
	}

	openLots := 0
	closedLegs := 0

	for symbol, fills := range bySymbol {
		for _, leg := range closedLegFills(fills) {
			tier.upsertClosedHistoryLot(symbol, leg)
			closedLegs++
		}
	}

	spotHolds := tier.spotUSDHolds()
	holdSpot := make(map[string]string, len(spotHolds))

	for symbol, hold := range spotHolds {
		if hold != nil {
			holdSpot[symbol] = hold.String()
		}
	}

	var holdNoFills []string
	holdNoLeg := make(map[string]any)
	holdRejected := make(map[string]string)
	var admitted []string

	for symbol, hold := range spotHolds {
		fills := bySymbol[symbol]

		if len(fills) == 0 {
			holdNoFills = append(holdNoFills, symbol)

			continue
		}

		leg := tier.openLegForHold(fills, hold, symbol)

		if len(leg) == 0 {
			trace := map[string]any{
				"hold":  hold.String(),
				"fills": len(fills),
				"net":   netFillQty(fills).String(),
			}

			if open := openLegFills(fills); len(open) > 0 {
				trace["open_leg_net"] = netFillQty(open).String()
			}

			holdNoLeg[symbol] = trace

			continue
		}

		if ok, reason := tier.upsertHistoryLot(symbol, leg); ok {
			openLots++
			admitted = append(admitted, symbol)
		} else {
			holdRejected[symbol] = reason
		}
	}

	skippedPair := len(unresolvedPairs)

	errnie.Info(fmt.Sprintf(
		"[broker.Tier] trades history fold: %d trades, %d skipped volume, %d skipped foreign, %d unresolved pair, %d history symbols, %d spot holds, %d admitted, %d closed legs",
		len(trades), skippedVolume, skippedForeign, skippedPair, len(bySymbol),
		len(spotHolds), openLots, closedLegs,
	))

	if len(holdNoFills) > 0 || len(holdNoLeg) > 0 || len(holdRejected) > 0 {
		errnie.Info(fmt.Sprintf(
			"[broker.Tier] history admit trace: admitted=%v hold_no_fills=%v hold_no_leg=%v hold_rejected=%v",
			admitted, holdNoFills, holdNoLeg, holdRejected,
		))
	}

	tier.publishHistorySync("ok", map[string]any{
		"trades":          len(trades),
		"skipped_volume":  skippedVolume,
		"skipped_foreign": skippedForeign,
		"skipped_pair":    skippedPair,
		"history_symbols": len(bySymbol),
		"open_lots":       openLots,
		"closed_legs":     closedLegs,
		"spot_holds":      holdSpot,
		"admitted":        admitted,
		"hold_no_fills":   holdNoFills,
		"hold_no_leg":     holdNoLeg,
		"hold_rejected":   holdRejected,
	})

	tier.purgeSyntheticBalanceLots()
	tier.Hello()
}

/*
historyFill omits CumQty intentionally — foldFills sums LastQty per print;
carrying venue cumulatives broke multi-buy exchange lots.
*/
func historyFill(txid string, trade kraken.HistoryTrade) *market.ExecutionData {
	if trade.Volume == nil || trade.Volume.Sign() <= 0 {
		return nil
	}

	side := trade.Type

	orderType := trade.OrderType

	if orderType == "" {
		orderType = "market"
	}

	var tradePx *decimal.Decimal

	if trade.Cost != nil && trade.Volume != nil && trade.Cost.Sign() > 0 {
		effective := trade.Cost.Div(*trade.Volume)
		tradePx = &effective
	} else {
		tradePx = trade.Price
	}

	fill := &market.ExecutionData{
		ExecID:      txid,
		ExecType:    "trade",
		OrderID:     trade.OrderID,
		Symbol:      trade.Pair,
		Side:        side,
		LastQty:     trade.Volume,
		LastPrice:   tradePx,
		Cost:        trade.Cost,
		FeeUsdEquiv: trade.Fee,
		OrderType:   orderType,
		Timestamp:   time.Unix(0, int64(trade.Time*float64(time.Second))).UTC(),
		OrderStatus: "filled",
	}

	// TradesHistory rows are one print each — not venue order cumulatives.
	// foldFills must sum LastQty; CumQty here overwrote multi-buy lots.
	return fill
}

/*
openLegFills isolates the current long leg so sells from a prior round trip do
not net into the open qty we compare against spot hold.
*/
func openLegFills(fills []*market.ExecutionData) []*market.ExecutionData {
	if len(fills) == 0 {
		return nil
	}

	sorted := slices.Clone(fills)

	slices.SortFunc(sorted, func(a, b *market.ExecutionData) int {
		if a == nil || b == nil {
			return 0
		}

		return a.Timestamp.Compare(b.Timestamp)
	})

	net := decimal.Zero
	start := 0

	for i, fill := range sorted {
		if fill == nil || fill.LastQty == nil || fill.LastQty.Sign() <= 0 {
			continue
		}

		switch strings.ToLower(fill.Side) {
		case "buy":
			net = net.Add(*fill.LastQty)
		case "sell":
			net = net.Sub(*fill.LastQty)
		}

		if net.Sign() <= 0 {
			start = i + 1
			net = decimal.Zero
		}
	}

	if net.Sign() <= 0 {
		return nil
	}

	return sorted[start:]
}

/*
historyPairForDesk filters foreign legs early so fold does not spend symbols on
cross pairs outside the configured quote book.
*/
func historyPairForDesk(pair, quote string) bool {
	if pair == "" {
		return false
	}

	if strings.Contains(pair, "/") {
		return strings.HasSuffix(pair, "/"+quote)
	}

	return strings.HasSuffix(pair, quote)
}

/*
netFillQty is the sanity check inside openLegForHold — hold must match folded
net, not individual buy rows.
*/
func netFillQty(fills []*market.ExecutionData) decimal.Decimal {
	net := decimal.Zero

	for _, fill := range fills {
		if fill == nil || fill.LastQty == nil || fill.LastQty.Sign() <= 0 {
			continue
		}

		switch strings.ToLower(fill.Side) {
		case "buy":
			net = net.Add(*fill.LastQty)
		case "sell":
			net = net.Sub(*fill.LastQty)
		}
	}

	return net
}

/*
openLegForHold disambiguates when multiple overlapping legs exist in history —
wrong subset is why admit logs hold_no_leg vs hold_rejected.
*/
func (tier *Tier) openLegForHold(
	all []*market.ExecutionData,
	spot *decimal.Decimal,
	symbol string,
) []*market.ExecutionData {
	if len(all) == 0 || spot == nil || spot.Sign() <= 0 {
		return nil
	}

	match := func(net *decimal.Decimal) bool {
		return tier.historyMatchesSpot(symbol, net, spot)
	}

	if leg := openLegFills(all); len(leg) > 0 {
		net := netFillQty(leg)

		if match(&net) {
			return leg
		}
	}

	netAll := netFillQty(all)

	if match(&netAll) {
		return all
	}

	sorted := slices.Clone(all)

	slices.SortFunc(sorted, func(a, b *market.ExecutionData) int {
		if a == nil || b == nil {
			return 0
		}

		return a.Timestamp.Compare(b.Timestamp)
	})

	need := *spot
	var picked []*market.ExecutionData

	for i := len(sorted) - 1; i >= 0; i-- {
		fill := sorted[i]

		if fill == nil || fill.LastQty == nil || fill.LastQty.Sign() <= 0 {
			continue
		}

		switch strings.ToLower(fill.Side) {
		case "sell":
			need = need.Add(*fill.LastQty)
			picked = append([]*market.ExecutionData{fill}, picked...)
		case "buy":
			if need.Sign() <= 0 {
				continue
			}

			picked = append([]*market.ExecutionData{fill}, picked...)
			need = need.Sub(*fill.LastQty)
		}

		if need.Sign() <= 0 {
			net := netFillQty(picked)

			if match(&net) {
				return picked
			}
		}
	}

	if open := openLegFills(all); len(open) > 0 {
		net := netFillQty(open)

		if match(&net) {
			return open
		}
	}

	return nil
}

/*
spotUSDHolds is the admit oracle: TradesHistory suggests legs, Balance decides
which symbols actually have spendable spot worth managing.
*/
func (tier *Tier) spotUSDHolds() map[string]*decimal.Decimal {
	out := make(map[string]*decimal.Decimal)

	if tier == nil || tier.price == nil || tier.venue == nil {
		return out
	}

	balance, ok := tier.venue.Balance().(*kraken.Balance)

	if !ok || balance == nil || balance.Result == nil {
		return out
	}

	quote := system.Cfg.Market.QuoteCurrency

	for asset, amount := range balance.Result {
		if amount == nil || amount.Sign() <= 0 {
			continue
		}

		if slices.Contains(system.Cfg.Market.Instrument.Excluded, kraken.SpotAsset(asset)) {
			continue
		}

		symbol := tier.price.DeskSymbolForLedgerAsset(asset)

		if symbol == "" || !strings.HasSuffix(symbol, "/"+quote) {
			continue
		}

		touch := *amount
		out[symbol] = &touch
	}

	return out
}

/*
closedLegFills feeds exchangeClosed hub rows without opening duplicate manage
paths — each flat-to-flat segment becomes one archived trip.
*/
func closedLegFills(fills []*market.ExecutionData) [][]*market.ExecutionData {
	if len(fills) == 0 {
		return nil
	}

	sorted := slices.Clone(fills)

	slices.SortFunc(sorted, func(a, b *market.ExecutionData) int {
		if a == nil || b == nil {
			return 0
		}

		return a.Timestamp.Compare(b.Timestamp)
	})

	var closed [][]*market.ExecutionData
	var leg []*market.ExecutionData
	net := decimal.Zero
	wasLong := false

	for _, fill := range sorted {
		if fill == nil || fill.LastQty == nil || fill.LastQty.Sign() <= 0 {
			continue
		}

		leg = append(leg, fill)

		switch strings.ToLower(fill.Side) {
		case "buy":
			net = net.Add(*fill.LastQty)
		case "sell":
			net = net.Sub(*fill.LastQty)
		}

		if net.Sign() > 0 {
			wasLong = true
		}

		if wasLong && net.Sign() <= 0 {
			closed = append(closed, slices.Clone(leg))
			leg = nil
			net = decimal.Zero
			wasLong = false
		}
	}

	return closed
}

/*
exchangeLotViable stops history from re-opening lots the desk could not exit on
REAL because the pair left spot or qty fell below min.
*/
func (tier *Tier) exchangeLotViable(symbol string, qty *decimal.Decimal) bool {
	if tier == nil || symbol == "" || qty == nil || qty.Sign() <= 0 {
		return false
	}

	if tier.price == nil || tier.price.Instrument == nil {
		return false
	}

	if !tier.price.Instrument.SpotOpen(symbol) {
		return false
	}

	if tier.Type != TierTypeReal {
		return true
	}

	pair := tier.price.Instrument.Pair(symbol)

	if pair.QtyMin != nil && qty.LessThan(*pair.QtyMin) {
		return false
	}

	return true
}

/*
clearHistoryOpenLots prevents stale exchange IDs surviving a refold — otherwise
two open rows for one symbol fight Apply matching.
*/
func (tier *Tier) clearHistoryOpenLots() {
	if tier == nil || tier.positions == nil {
		return
	}

	for _, position := range tier.positions.Open() {
		if position == nil || position.Symbol == "" || position.EntryPath != "exchange" {
			continue
		}

		tier.positions.Drop(position.Symbol)
	}
}

/*
spotHoldQty is the per-symbol check inside upsertHistoryLot after fold — ledger
keys and instrument base names both must resolve to the same hold.
*/
func (tier *Tier) spotHoldQty(symbol string) (*decimal.Decimal, bool) {
	if tier == nil || tier.venue == nil || symbol == "" || tier.price == nil {
		return nil, false
	}

	balance, ok := tier.venue.Balance().(*kraken.Balance)

	if !ok || balance == nil || balance.Result == nil {
		return nil, false
	}

	if tier.price.Instrument != nil && tier.price.Instrument.Has(symbol) {
		base := tier.price.Instrument.Pair(symbol).Base

		if amount, ok := balance.AssetAmount(base); ok && amount != nil && amount.Sign() > 0 {
			return amount, true
		}
	}

	for asset, amount := range balance.Result {
		if amount == nil || amount.Sign() <= 0 {
			continue
		}

		if tier.price.DeskSymbolForLedgerAsset(asset) != symbol {
			continue
		}

		touch := *amount

		return &touch, true
	}

	return nil, false
}

/*
balanceQtyScale widens admit tolerance when Balance reports more decimals than
TradesHistory lot precision allows.
*/
func balanceQtyScale(hold decimal.Decimal) int32 {
	s := hold.String()

	dot := strings.IndexByte(s, '.')

	if dot < 0 {
		return 0
	}

	return int32(len(s) - dot - 1)
}

/*
historyMatchesSpot rejects invented lots when history net and wallet disagree —
symm-managed symbols should not get phantom exchange lots from rounding drift.
*/
func (tier *Tier) historyMatchesSpot(symbol string, lotQty, hold *decimal.Decimal) bool {
	if lotQty == nil || hold == nil || lotQty.Sign() <= 0 || hold.Sign() <= 0 {
		return false
	}

	if lotQty.Equal(*hold) {
		return true
	}

	precision := 8

	if tier.price != nil && tier.price.Instrument != nil && tier.price.Instrument.Has(symbol) {
		precision = tier.price.Instrument.Pair(symbol).QtyPrecision
	}

	if precision < 0 {
		precision = 0
	}

	coarse := balanceQtyScale(*hold)

	if coarse > int32(precision) {
		coarse = int32(precision)
	}

	if lotQty.Round(coarse).Equal(hold.Round(coarse)) {
		return true
	}

	step := decimal.New(1, -int32(precision))
	tolerance := step.Mul(decimal.NewFromInt(2))

	if coarse > 0 {
		balanceStep := decimal.New(1, -coarse)
		balanceTol := balanceStep.Mul(decimal.NewFromInt(2))

		if balanceTol.GreaterThan(tolerance) {
			tolerance = balanceTol
		}
	}

	return hold.Sub(*lotQty).Abs().LessThanOrEqual(tolerance)
}

/*
historyLotRejectReason surfaces in admit trace JSON so operators see below_qty_min
vs not_spot_online without reading fold logs.
*/
func (tier *Tier) historyLotRejectReason(symbol string, qty *decimal.Decimal) string {
	if tier == nil || symbol == "" || qty == nil || qty.Sign() <= 0 {
		return "zero_qty"
	}

	if tier.price == nil || tier.price.Instrument == nil {
		return "no_instrument"
	}

	if !tier.price.Instrument.Has(symbol) {
		return "not_in_instrument_cache"
	}

	if !tier.price.Instrument.SpotOpen(symbol) {
		return "not_spot_online"
	}

	pair := tier.price.Instrument.Pair(symbol)

	if pair.QtyMin != nil && qty.LessThan(*pair.QtyMin) {
		return "below_qty_min"
	}

	return "not_viable"
}

/*
upsertHistoryLot refuses to clobber symm_entered opens — exchange sync is read-only
for third-party fills, not a takeover of bot-managed risk.
*/
func (tier *Tier) upsertHistoryLot(key string, fills []*market.ExecutionData) (bool, string) {
	if tier == nil || key == "" || len(fills) == 0 {
		return false, "no_fills"
	}

	symbol := tier.deskSymbol(fills[0].Symbol)

	if symbol == "" {
		return false, "desk_symbol_unresolved"
	}

	for _, position := range tier.positions.Open(symbol) {
		if position != nil && position.EntryPath != "exchange" {
			return false, "symm_lot_open"
		}
	}

	quote := system.Cfg.Market.QuoteCurrency

	if !strings.HasSuffix(symbol, "/"+quote) {
		return false, "not_quote_pair"
	}

	var net decimal.Decimal

	for _, fill := range fills {
		if fill == nil || fill.LastQty == nil {
			continue
		}

		if tier.price != nil {
			fill.Symbol = tier.deskSymbol(fill.Symbol)
		}

		switch strings.ToLower(fill.Side) {
		case "buy":
			net = net.Add(*fill.LastQty)
		case "sell":
			net = net.Sub(*fill.LastQty)
		}
	}

	lot := tier.lotByID(key)

	if lot == nil {
		for _, position := range tier.positions.Open(symbol) {
			if position != nil && position.Symbol == symbol &&
				position.EntryPath == "exchange" {
				lot = position
				break
			}
		}
	}

	if net.Sign() <= 0 {
		if lot != nil && lot.EntryPath != "exchange" {
			return false, fmt.Sprintf("symm_lot_skip net=%s", net.String())
		}

		tier.positions.Drop(symbol)

		return false, fmt.Sprintf("leg_net_nonpos net=%s", net.String())
	}

	if lot == nil {
		lot = NewPosition(
			tier.ctx,
			symbol,
			tier.Type.String(),
			"exchange",
			tier.price,
			tier.private,
			tier.settle,
		)
		lot.ID = key
	}

	lot.mu.Lock()
	lot.Fills = make([]*market.ExecutionData, 0, len(fills))
	lot.ClosedAt = nil
	lot.Closing = false
	lot.Qty = nil
	lot.Sold = nil
	lot.mu.Unlock()

	for _, fill := range fills {
		if fill == nil {
			continue
		}

		lot.Apply(*fill)
	}

	hold, holdOK := tier.spotHoldQty(symbol)

	if !holdOK || hold == nil {
		tier.positions.Drop(symbol)

		return false, "balance_hold_missing"
	}

	if lot.Qty == nil || !tier.historyMatchesSpot(symbol, lot.Qty, hold) {
		if lot.EntryPath != "exchange" {
			return false, "symm_lot_qty_mismatch"
		}

		tier.positions.Drop(symbol)

		lotQty := "nil"

		if lot.Qty != nil {
			lotQty = lot.Qty.String()
		}

		return false, fmt.Sprintf(
			"qty_mismatch lot=%s hold=%s leg_net=%s",
			lotQty, hold.String(), net.String(),
		)
	}

	if !tier.exchangeLotViable(symbol, lot.Qty) {
		tier.positions.Drop(symbol)

		return false, tier.historyLotRejectReason(symbol, lot.Qty)
	}

	tier.wireExchangeLot(lot)
	tier.positions.SetOpen(symbol, lot)
	tier.markHistoryLot(lot)

	return true, ""
}

/*
upsertClosedHistoryLot does not enter the open cache — it only populates
exchangeClosed so Hello shows off-bot round trips without armable stoploss.
*/
func (tier *Tier) upsertClosedHistoryLot(key string, fills []*market.ExecutionData) {
	if tier == nil || key == "" || len(fills) == 0 {
		return
	}

	symbol := tier.deskSymbol(fills[0].Symbol)

	if symbol == "" {
		return
	}

	quote := system.Cfg.Market.QuoteCurrency

	if !strings.HasSuffix(symbol, "/"+quote) {
		return
	}

	if tier.price == nil || tier.price.Instrument == nil ||
		!tier.price.Instrument.SpotOpen(symbol) {
		return
	}

	id := closedLotID(key, fills)

	if _, loaded := tier.exchangeClosed.Load(id); loaded {
		return
	}

	closedAt := fills[len(fills)-1].Timestamp

	if closedAt.IsZero() {
		closedAt = time.Now().UTC()
	}

	lot := NewPosition(
		tier.ctx,
		symbol,
		tier.Type.String(),
		"exchange",
		tier.price,
		tier.private,
		nil,
	)
	lot.ID = id

	for _, fill := range fills {
		if fill == nil {
			continue
		}

		if tier.price != nil {
			fill.Symbol = tier.deskSymbol(fill.Symbol)
		}

		lot.Apply(*fill)
	}

	if lot.ClosedAt == nil {
		lot.ClosedAt = &closedAt
	}

	tier.finalizeClosedLot(lot)
	tier.recordExchangeClosed(lot)
}

/*
clearExchangeClosed prevents duplicate closed ids when history sync re-runs — the
map is hub overlay, not the tier closed lane.
*/
func (tier *Tier) clearExchangeClosed() {
	tier.exchangeClosed.Range(func(key, _ any) bool {
		tier.exchangeClosed.Delete(key)
		return true
	})
}

/*
recordExchangeClosed dedupes via Load before insert so repeated fold of the same
closed segment does not flicker UI positions.
*/
func (tier *Tier) recordExchangeClosed(lot *Position) {
	if tier == nil || lot == nil || lot.ID == "" || lot.ClosedAt == nil {
		return
	}

	tier.exchangeClosed.Store(lot.ID, lot)
}

/*
markHistoryLot triggers L3 resubscribe when mark fails — exchange lots still need
live books for hub PnL even though stoploss does not manage them.
*/
func (tier *Tier) markHistoryLot(lot *Position) {
	if lot == nil || lot.ClosedAt != nil || lot.EntryPath != "exchange" {
		return
	}

	mark := tier.exchangeMark(lot)

	if mark == nil && tier.price != nil && tier.price.Instrument != nil {
		_ = tier.price.Instrument.ResubscribeLevel3(lot.Symbol)
		mark = tier.exchangeMark(lot)
	}

	if mark == nil {
		return
	}

	_ = lot.Update(mark)

	if tier.positions != nil {
		if err := tier.positions.Save(lot); err != nil {
			errnie.Error(err)
		}
	}
}

/*
exchangeMark avoids freezing nil marks on thin books while not re-marking closed
snapshots that Hello already froze.
*/
func (tier *Tier) exchangeMark(lot *Position) *decimal.Decimal {
	if tier.price == nil || lot == nil {
		return nil
	}

	if mark := tier.price.CloseMark(lot.Symbol); mark != nil {
		return mark
	}

	lot.mu.Lock()
	mark := lot.Mark
	lot.mu.Unlock()

	return mark
}

/*
lotByID reunites history txid keys with in-flight submits when Kraken reuses ids
across open and archived rows during upsert.
*/
func (tier *Tier) lotByID(id string) *Position {
	if id == "" {
		return nil
	}

	for _, position := range tier.positions.Submitted() {
		if position != nil && position.ID == id {
			return position
		}
	}

	for _, position := range tier.positions.Closed() {
		if position != nil && position.ID == id {
			return position
		}
	}

	return nil
}

/*
deskSymbol centralizes pair normalization inside ledger fold so every fill row
lands on the same cache key as live executions.
*/
func (tier *Tier) deskSymbol(pair string) string {
	if tier.price == nil {
		return ""
	}

	return tier.price.ResolveDeskSymbol(pair)
}

/*
purgeSyntheticBalanceLots drops interim balance: rows once TradesHistory provides
the real fill-backed lot — avoids duplicate manage on the same hold.
*/
func (tier *Tier) purgeSyntheticBalanceLots() {
	if tier == nil || tier.Type != TierTypeReal || tier.positions == nil {
		return
	}

	for _, position := range tier.positions.Submitted() {
		if position == nil || position.Symbol == "" {
			continue
		}

		if !strings.HasPrefix(position.ID, "balance:") {
			continue
		}

		tier.positions.Drop(position.Symbol)
	}
}

/*
wireExchangeLot rebuilds EntryOrder/ExitOrder so Owns and hub JSON show plausible
cl_ord_id/venue ids even though symm did not send those trades.
*/
func (tier *Tier) wireExchangeLot(lot *Position) {
	if lot == nil {
		return
	}

	lot.mu.Lock()
	defer lot.mu.Unlock()

	vol := entryVolume(lot)

	var opened time.Time
	entryTx := ""
	entryKind := ""

	var exitTx string
	exitKind := ""

	for _, fill := range lot.Fills {
		if fill == nil {
			continue
		}

		if strings.EqualFold(fill.Side, "buy") {
			if opened.IsZero() || fill.Timestamp.Before(opened) {
				opened = fill.Timestamp
				entryTx = fill.OrderID
				entryKind = fill.OrderType
			}

			if entryTx == "" && fill.OrderID != "" {
				entryTx = fill.OrderID
				entryKind = fill.OrderType
			}

			continue
		}

		if strings.EqualFold(fill.Side, "sell") {
			exitTx = fill.OrderID
			exitKind = fill.OrderType
		}
	}

	if !opened.IsZero() {
		lot.OpenedAt = opened
	}

	if vol == "" {
		return
	}

	if entryKind == "" {
		entryKind = "market"
	}

	if lot.EntryOrder == nil {
		clOrd := entryTx

		if clOrd == "" {
			clOrd = "exchange:" + lot.Symbol
		}

		lot.EntryOrder = &spot.OrderRequest{
			ClOrdId:   clOrd,
			OrderType: entryKind,
			Type:      "buy",
			Volume:    vol,
		}
	}

	lot.ensureExitOrder()

	if exitTx != "" && lot.ExitOrder != nil {
		if exitKind == "" {
			exitKind = "market"
		}

		lot.ExitOrder.ClOrdId = exitTx
		lot.ExitOrder.OrderType = exitKind
	}
}

/*
closedLotID prefers first: last exec ids so refold does not mint new ids for the
same round trip when timestamps shift.
*/
func closedLotID(key string, fills []*market.ExecutionData) string {
	if len(fills) == 0 {
		return key
	}

	first := fills[0]

	last := fills[len(fills)-1]

	if first != nil && last != nil &&
		first.ExecID != "" && last.ExecID != "" {
		return first.ExecID + ":" + last.ExecID
	}

	closedAt := last.Timestamp

	if closedAt.IsZero() {
		closedAt = time.Now().UTC()
	}

	if key == "" {
		key = "closed"
	}

	return fmt.Sprintf("%s-%d", key, closedAt.Unix())
}

/*
finalizeClosedLot applies FeeNet when possible so exchangeClosed rows match paper
tier economics on the hub without running stoploss.
*/
func (tier *Tier) finalizeClosedLot(lot *Position) {
	if lot == nil || lot.ClosedAt == nil {
		return
	}

	tier.wireExchangeLot(lot)

	if lot.OpenedAt.IsZero() {
		for _, fill := range lot.Fills {
			if fill == nil {
				continue
			}

			if strings.EqualFold(fill.Side, "buy") {
				lot.OpenedAt = fill.Timestamp
				break
			}
		}
	}

	if lot.PnL != nil && lot.PnLPct != nil {
		return
	}

	if tier.price == nil || lot.Entry == nil || lot.Sold == nil {
		return
	}

	exit := lot.ExitVWAP

	if exit == nil {
		exit = lot.Mark
	}

	if exit == nil || exit.Sign() <= 0 {
		return
	}

	lot.PnL, lot.PnLPct = tier.price.FeeNetRoundTrip(
		lot.Symbol, lot.Entry, lot.Sold, exit,
	)

	if lot.PnL == nil && lot.Entry != nil && lot.Sold != nil {
		cost := lot.Entry.Mul(*lot.Sold)
		proceeds := exit.Mul(*lot.Sold)
		gross := proceeds.Sub(cost)
		hundred := decimal.NewFromInt(100)
		pct := gross.Div(cost).Mul(hundred)
		lot.PnL = &gross
		lot.PnLPct = &pct
	}

	if lot.ExitTrigger == "" {
		lot.ExitTrigger = "exchange history"
	}
}

/*
entryVolume chooses qty vs sold for wireExchangeLot when a leg closed before wire
— volume must match what Kraken history claims was traded.
*/
func entryVolume(lot *Position) string {
	if lot.Qty != nil && lot.Qty.Sign() > 0 {
		return lot.Qty.String()
	}

	if lot.Sold != nil && lot.Sold.Sign() > 0 {
		return lot.Sold.String()
	}

	return ""
}
