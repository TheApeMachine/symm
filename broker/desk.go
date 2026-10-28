package broker

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/ui"
)

/*
Desk owns open positions: it sizes entries from the book (cash budget,
touch-bid carry divisor, and ask walk), works them in child market
orders, takes the learned exit, runs the capacity monitor, accumulates venue
fills beside their shadow fills, and reports each completed round trip.
*/
type Desk struct {
	*runtime.System
	transport      Transport
	price          *Price
	balance        *Balance
	history        *History
	positions      *Cache
	virtuals       *Cache
	virtualArchive *sync.Map
	pending        *sync.Map
	policies       *sync.Map
}

func NewDesk(
	ctx context.Context,
	transport Transport,
	price *Price,
	balance *Balance,
) *Desk {
	desk := &Desk{
		transport:      transport,
		balance:        balance,
		history:        NewHistory(ctx),
		price:          price,
		positions:      NewCache(ctx),
		virtuals:       NewCache(ctx),
		virtualArchive: &sync.Map{},
		pending:        &sync.Map{},
		policies:       &sync.Map{},
	}

	desk.System = runtime.NewSystem(ctx, "desk", desk)

	for key, cache := range map[string]*Cache{
		"position": desk.positions,
		"virtual":  desk.virtuals,
	} {
		desk.restoreCache(key, cache)
	}

	desk.resumePaperOrders()
	desk.reconcilePaperFills()
	desk.retireFlatPaper()
	desk.Transition(runtime.READY)
	return desk
}

type historyHit struct {
	role     string
	position *Position
	value    *data.Value
}

/*
readHistory decodes one stored lot. The layer name is always "position";
the book (paper or virtual) is the object prefix, not the layer.
*/
func readHistory(value *data.Value) (historyHit, bool) {
	if value == nil {
		return historyHit{}, false
	}

	fields := strings.Split(value.Prefix, "/")
	if len(fields) < 5 {
		return historyHit{}, false
	}

	role := fields[1]
	if role != "open" && role != "closed" {
		return historyHit{}, false
	}

	position := &Position{}
	value.Lookup(position, "position")

	if value.Err != nil || position.Symbol == "" {
		return historyHit{}, false
	}

	return historyHit{role: role, position: position, value: value}, true
}

func keepHistory(into map[string]historyHit, key string, hit historyHit) {
	if into == nil || key == "" {
		return
	}

	prev, exists := into[key]
	if exists && !laterHistory(hit.value, prev.value) {
		return
	}

	into[key] = hit
}

func prepareRestored(position *Position) {
	if position == nil || position.ExitPath != "" {
		return
	}

	position.ExitPath = position.EntryPath
}

func laterHistory(next, prev *data.Value) bool {
	if next == nil {
		return false
	}

	if prev == nil {
		return true
	}

	if next.Epoch != prev.Epoch {
		return next.Epoch > prev.Epoch
	}

	if next.Tick != prev.Tick {
		return next.Tick > prev.Tick
	}

	return next.Timestamp > prev.Timestamp
}

func (desk *Desk) restoreCache(cacheKey string, cache *Cache) {
	if desk == nil || desk.history == nil || cache == nil {
		return
	}

	open := map[string]historyHit{}
	closed := map[string]historyHit{}
	settled := map[string]historyHit{}

	for value := range desk.history.Restore(cacheKey) {
		hit, ok := readHistory(value)
		if !ok {
			if value != nil && value.Err != nil {
				desk.Error(value.Err)
			}

			continue
		}

		// A closed paper lot is a round trip this market actually paid for,
		// whether or not its open record survived the restart. Fold it in
		// before the settled map can drop it.
		if hit.role == "closed" && cacheKey == "position" {
			desk.observePolicy(hit.position)
		}

		switch hit.role {
		case "open":
			prev, existed := open[hit.position.Symbol]
			keepHistory(open, hit.position.Symbol, hit)
			if existed {
				desk.dropStaleOpen(prev, hit, open[hit.position.Symbol])
			}
		case "closed":
			if hit.position.ID == "" {
				continue
			}

			if cacheKey == "virtual" {
				keepHistory(closed, hit.position.ID, hit)
				continue
			}

			keepHistory(settled, hit.position.ID, hit)
		}
	}

	for _, hit := range open {
		if !restoreOpen(hit, settled) {
			if hit.value != nil {
				_ = desk.history.Drop(hit.value.Prefix)
			}
			continue
		}

		desk.bind(hit.position)
		prepareRestored(hit.position)
		desk.teachPolicy(hit.position)
		cache.Restore(hit.position)
	}

	for _, hit := range closed {
		desk.bind(hit.position)
		desk.virtualArchive.Store(hit.position.ID, hit.position)
		desk.observePolicy(hit.position)
	}

	// Every symbol that settled something gets a policy before its next lot
	// opens, so the first entry after a restart is already taught.
	desk.refreshPolicies()
}

func (desk *Desk) dropStaleOpen(prev, next, kept historyHit) {
	if desk == nil || desk.history == nil || prev.value == nil || next.value == nil || kept.value == nil {
		return
	}

	if prev.value.Prefix == "" || prev.value.Prefix == kept.value.Prefix {
		if next.value.Prefix != kept.value.Prefix {
			_ = desk.history.Drop(next.value.Prefix)
		}
		return
	}

	_ = desk.history.Drop(prev.value.Prefix)
}

func restoreOpen(hit historyHit, settled map[string]historyHit) bool {
	if hit.position == nil {
		return false
	}

	if hit.position.ClosedAt != nil && !hit.position.ClosedAt.IsZero() {
		return false
	}

	if hit.position.ID == "" {
		return true
	}

	_, done := settled[hit.position.ID]
	return !done
}

func (desk *Desk) bind(position *Position) {
	if desk == nil || position == nil {
		return
	}

	// History JSON does not carry the live price book. Without it,
	// later marks never reprice and a lot in profit never exits.
	position.price = desk.price
	position.transport = desk.transport
	position.onSettled = desk.onPositionSettled
	if position.stopLoss == nil {
		position.stopLoss = NewStopLoss()
	}
}

func (desk *Desk) snapshotOpen(position *Position) error {
	if desk == nil || desk.history == nil || position == nil || position.Symbol == "" {
		return nil
	}

	if position.ClosedAt != nil && !position.ClosedAt.IsZero() {
		return nil
	}

	_, key := desk.cacheFor(position.Virtual)

	return desk.history.ReplaceOpen(data.NewValue(
		key, "open", position.Symbol,
	).Append(
		position, "position",
	))
}

/*
Hello is a callback for the ui.Hub that fires when a new WebSocket
connection is established, and we want to update the client with
some initial data.
*/
func (desk *Desk) Hello() {
	// The snapshot replaces the book. A mark tick publishes one lot at a
	// time and must not reuse this frame, or the page drops every lot that
	// did not move.
	io.Copy(ui.UI, data.NewValue("positions", "snapshot", "all").Append(
		append(
			desk.positions.AllPositions(),
			desk.virtuals.AllPositions()...,
		), "positions",
	))

	if desk.balance.value != nil {
		io.Copy(ui.UI, desk.balance.value)
	}
}

func (desk *Desk) cacheFor(virtual bool) (*Cache, string) {
	if virtual {
		return desk.virtuals, "virtual"
	}

	return desk.positions, "position"
}

/*
OpenPositions returns the open positions in the desk.
*/
func (desk *Desk) OpenPositions(virtual bool) []*Position {
	cache, _ := desk.cacheFor(virtual)
	return cache.OpenPositions()
}

func (desk *Desk) Position(symbol string, virtual bool) *Position {
	cache, _ := desk.cacheFor(virtual)
	return cache.Get(symbol, true)
}

/*
ClosedVirtuals returns virtual lots settled this tick and still on the closed
side of the live cache, awaiting promotion scoring.
*/
func (desk *Desk) ClosedVirtuals() []*Position {
	if desk == nil {
		return nil
	}

	return desk.virtuals.Closed()
}

/*
Takepending returns a virtual lot closed on a mark exit since the last
take. Evaluate consumes it for path threshold updates.
*/
func (desk *Desk) Takepending(symbol string) (*Position, bool) {
	if desk == nil || symbol == "" {
		return nil, false
	}

	val, ok := desk.pending.LoadAndDelete(symbol)
	if !ok {
		return nil, false
	}

	position, ok := val.(*Position)
	return position, ok && position != nil
}

/*
pendingSymbols lists pairs waiting for Evaluate to score a mark exit.
*/
func (desk *Desk) pendingSymbols() []string {
	if desk == nil {
		return nil
	}

	out := make([]string, 0)

	desk.pending.Range(func(key, _ any) bool {
		if symbol, ok := key.(string); ok && symbol != "" {
			out = append(out, symbol)
		}

		return true
	})

	slices.Sort(out)
	return out
}

func (desk *Desk) onPositionSettled(position *Position) {
	if desk == nil || position == nil {
		return
	}

	if err := desk.releaseLocked(position); err != nil {
		errnie.Error(err)
	}
}

/*
Update marks open positions against the trade price. Called from the workspace
tick handler — the desk sits off the trade ingress path, so this is where
marks become live.

The trade batch is the changed set. Those lots are the ones published. Open lots
whose book mark actually moved are also updated so exits see a live mark.
Unchanged, non-traded lots are skipped to prevent false momentum stalls.
*/
func (desk *Desk) Update(trades []kraken.TradeData) {
	if len(trades) == 0 {
		return
	}

	traded := make(map[string]struct{}, len(trades))
	for _, t := range trades {
		if t.Symbol != "" {
			traded[t.Symbol] = struct{}{}
		}
	}

	changed := make([]*Position, 0)

	for _, cache := range []*Cache{desk.positions, desk.virtuals} {
		for _, position := range cache.OpenPositions() {
			if position == nil || position.Symbol == "" {
				continue
			}

			mark := desk.price.CurrentMark(position.Symbol)
			if mark == nil {
				continue
			}

			_, isTraded := traded[position.Symbol]
			markUnchanged := position.Mark != nil && position.Mark.Cmp(mark) == 0
			watchProfit := profitLockArmed(position) || watchGreenBook(position)

			// Skip idle lots. A green lot is still repriced when the touch
			// price is unchanged: ExitRoom is size, and a size pull is an
			// exit input. Identical replays are not this symbol's tape.
			if !isTraded && markUnchanged && !watchProfit {
				continue
			}

			position.Update(mark)
			changed = append(changed, position)
		}
	}

	if len(changed) > 0 {
		// One frame per lot. Publishing this sparse set as scope "all"
		// replaces the connect snapshot, so the page drops every lot that
		// did not move on this tick and the open count jumps with it.
		for _, frame := range lotUpdates(changed) {
			io.Copy(ui.UI, frame)
		}
	}
}

/*
lotUpdates addresses each lot on its own scope. The connect snapshot stays
put under "all"; a later tick only replaces the lots it names.
*/
func lotUpdates(lots []*Position) []*data.Value {
	ordered := slices.Clone(lots)
	sortPositions(ordered)

	frames := make([]*data.Value, 0, len(ordered))

	for _, position := range ordered {
		if position == nil || position.Symbol == "" {
			continue
		}

		scope := position.ID
		if scope == "" {
			scope = position.Symbol
			if position.Virtual {
				scope = "virtual:" + scope
			}
		}

		frames = append(frames, data.NewValue("positions", "update", scope).Append(
			position, "position",
		))
	}

	return frames
}

/*
Has reports whether the desk holds an open position (paper or virtual)
for the symbol.
*/
func (desk *Desk) Has(symbol string, virtual bool) bool {
	cache, _ := desk.cacheFor(virtual)
	return cache.Has(symbol)
}

/*
CanSize reports whether the book and budget can allocate a venue-valid lot.
*/
func (desk *Desk) CanSize(symbol string) bool {
	if desk == nil || desk.price == nil || symbol == "" {
		return false
	}

	qty, _ := desk.price.Allocate(symbol)
	return qty != nil && qty.Sign() > 0
}

/*
Apply an execution to the desk.
*/
func (desk *Desk) Apply(executions *kraken.Execution) {
	for _, execution := range executions.Data {
		position := desk.positions.Get(execution.Symbol, true)

		if position == nil {
			position = desk.positions.Get(execution.Symbol, false)
		}

		if position == nil {
			position = desk.virtuals.Get(execution.Symbol, true)
		}

		if position == nil {
			continue
		}

		position.Apply(execution)

		// A filled sell is closed. releaseLocked retires the open objects.
		// Removing the lot here skips that and a restart opens it again.
		if !position.Virtual && position.ClosedAt != nil {
			continue
		}

		// The entry snapshot is taken before a paper fill arrives. Rewrite
		// the open record so a restart comes back with the fill and order id.
		if err := desk.snapshotOpen(position); err != nil {
			errnie.Error(err)
		}
	}
}

type resumedOrder struct {
	orderID       string
	clientOrderID string
	pair          string
	side          string
}

type paperOrderBook interface {
	OpenOrders() (spot.OpenOrdersResult, error)
	ResumeOpenOrder(orderID, clientOrderID, pair, side string)
}

func (desk *Desk) resumePaperOrders() {
	if desk == nil || desk.transport == nil || len(desk.positions.OpenPositions()) == 0 {
		return
	}

	book, ok := desk.transport.(paperOrderBook)
	if !ok {
		return
	}

	open, err := book.OpenOrders()
	if err != nil {
		errnie.Warn("[desk] paper open orders were not restored: " + err.Error())
		return
	}

	for _, position := range desk.positions.OpenPositions() {
		for _, order := range ordersToResume(position, open) {
			book.ResumeOpenOrder(order.orderID, order.clientOrderID, order.pair, order.side)
		}
	}
}

/*
reconcilePaperFills copies venue buys onto open paper lots restored
without an entry price. Close-now PnL is empty until that price is on the lot.
*/
func (desk *Desk) reconcilePaperFills() {
	if desk == nil || desk.transport == nil || desk.positions == nil {
		return
	}

	ledger, ok := desk.transport.(interface {
		TradesHistory() (spot.TradesHistoryResult, error)
	})
	if !ok {
		return
	}

	history, err := ledger.TradesHistory()
	if err != nil {
		errnie.Warn("[desk] paper fills were not restored: " + err.Error())
		return
	}

	used := map[string]struct{}{}

	for _, position := range desk.positions.OpenPositions() {
		if position == nil || position.Virtual || filledSide(position, "buy") {
			continue
		}

		id, trade, matched := matchPaperBuy(position, history.Trades, used)
		if !matched {
			continue
		}

		used[id] = struct{}{}
		position.Apply(executionFromPaperTrade(position.Symbol, id, trade))

		if err := desk.snapshotOpen(position); err != nil {
			errnie.Error(err)
		}
	}
}

/*
retireFlatPaper drops restored lots the paper wallet does not hold.
A restart was reopening APR and XMN from leftover open objects after the
venue had already sold them, so the desk showed positions with no balance.
*/
func (desk *Desk) retireFlatPaper() {
	if desk == nil || desk.transport == nil || desk.positions == nil {
		return
	}

	ledger, ok := desk.transport.(interface {
		Balances() (*kraken.Balance, error)
		OpenOrders() (spot.OpenOrdersResult, error)
	})
	if !ok {
		return
	}

	wallet, err := ledger.Balances()
	if err != nil {
		errnie.Warn("[desk] paper balances were not read, so flat lots stayed open: " + err.Error())
		return
	}

	resting, err := ledger.OpenOrders()
	if err != nil {
		errnie.Warn("[desk] paper open orders were not read, so flat lots stayed open: " + err.Error())
		return
	}

	var trades map[string]spot.Trade
	if history, ok := desk.transport.(interface {
		TradesHistory() (spot.TradesHistoryResult, error)
	}); ok {
		if book, err := history.TradesHistory(); err == nil {
			trades = book.Trades
		}
	}

	for _, position := range desk.positions.OpenPositions() {
		if !flatAtVenue(position, holding(wallet, baseAsset(position.Symbol)), restingOn(position, resting)) {
			continue
		}

		if !filledSide(position, "sell") {
			if id, trade, matched := matchPaperSell(position, trades); matched {
				position.Apply(executionFromPaperTrade(position.Symbol, id, trade))
				if at := paperTradeTime(trade); !at.IsZero() {
					closed := at
					position.ClosedAt = &closed
				}
			}
		}

		errnie.Warn("[desk] " + position.Symbol + " is open on the desk and flat in the paper wallet; closing the lot")
		desk.settleFlat(position)
	}
}

func (desk *Desk) settleFlat(position *Position) {
	if position == nil {
		return
	}

	if position.ClosedAt == nil || position.ClosedAt.IsZero() {
		now := time.Now().UTC()
		position.ClosedAt = &now
	}

	position.exiting.Store(true)

	if desk.history == nil {
		desk.positions.Remove(position.Symbol)
		return
	}

	if err := desk.releaseLocked(position); err != nil {
		errnie.Error(err)
	}
}

func flatAtVenue(position *Position, held *decimal.Decimal, resting bool) bool {
	if position == nil || position.Virtual || resting {
		return false
	}

	return held == nil || held.Sign() <= 0
}

func baseAsset(symbol string) string {
	base, _, _ := strings.Cut(symbol, "/")
	return base
}

func holding(wallet *kraken.Balance, asset string) *decimal.Decimal {
	if wallet == nil || asset == "" {
		return nil
	}

	for _, row := range wallet.Data {
		if !strings.EqualFold(row.Asset, asset) {
			continue
		}

		if row.Balance != nil {
			return row.Balance
		}

		return row.Available
	}

	return nil
}

func restingOn(position *Position, open spot.OpenOrdersResult) bool {
	if position == nil || len(open.Open) == 0 {
		return false
	}

	for _, row := range open.Open {
		if row.ClOrdID != "" && position.EntryOrder != nil && row.ClOrdID == position.EntryOrder.ClOrdId {
			return true
		}

		if row.ClOrdID != "" && position.ExitOrder != nil && row.ClOrdID == position.ExitOrder.ClOrdId {
			return true
		}

		if row.Description != nil && sameVenuePair(row.Description.Pair, position.Symbol) {
			return true
		}
	}

	return false
}

func matchPaperSell(
	position *Position,
	trades map[string]spot.Trade,
) (string, spot.Trade, bool) {
	if position == nil {
		return "", spot.Trade{}, false
	}

	var (
		bestID string
		best   spot.Trade
		bestAt time.Time
		have   bool
	)

	for id, trade := range trades {
		if !strings.EqualFold(trade.Type, "sell") || !sameVenuePair(trade.Pair, position.Symbol) {
			continue
		}

		at := paperTradeTime(trade)
		if !position.OpenedAt.IsZero() && !at.IsZero() && at.Before(position.OpenedAt) {
			continue
		}

		if !have || at.After(bestAt) {
			best, bestID, bestAt, have = trade, id, at, true
		}
	}

	return bestID, best, have
}

func executionFromPaperTrade(symbol, id string, trade spot.Trade) kraken.ExecutionData {
	when := time.Time{}
	if trade.Time != nil {
		sec := trade.Time.Float64()
		when = time.Unix(0, int64(sec*1e9)).UTC()
	}

	side := trade.Type
	if side == "" {
		side = "buy"
	}

	return kraken.ExecutionData{
		OrderID:     trade.OrderID,
		ExecID:      id,
		ExecType:    "trade",
		Symbol:      symbol,
		Side:        side,
		LastQty:     trade.Volume,
		CumQty:      trade.Volume,
		LastPrice:   trade.Price,
		AvgPrice:    trade.Price,
		Cost:        trade.Cost,
		FeeUsdEquiv: trade.Fee,
		OrderStatus: "filled",
		Timestamp:   when,
	}
}

func matchPaperBuy(
	position *Position,
	trades map[string]spot.Trade,
	used map[string]struct{},
) (string, spot.Trade, bool) {
	if position == nil {
		return "", spot.Trade{}, false
	}

	var (
		bestID    string
		best      spot.Trade
		bestDelta time.Duration
		have      bool
	)

	for id, trade := range trades {
		if _, taken := used[id]; taken || !paperBuyFor(position, trade) {
			continue
		}

		at := paperTradeTime(trade)

		if position.OpenedAt.IsZero() {
			if !have || at.After(paperTradeTime(best)) {
				best, bestID, have = trade, id, true
			}
			continue
		}

		delta := at.Sub(position.OpenedAt)
		if delta < 0 {
			delta = -delta
		}

		if delta > 5*time.Minute {
			continue
		}

		if !have || delta < bestDelta {
			best, bestID, bestDelta, have = trade, id, delta, true
		}
	}

	return bestID, best, have
}

func paperBuyFor(position *Position, trade spot.Trade) bool {
	if position == nil || !strings.EqualFold(trade.Type, "buy") {
		return false
	}

	if !sameVenuePair(trade.Pair, position.Symbol) {
		return false
	}

	return trade.Volume != nil && trade.Volume.Sign() > 0 &&
		trade.Price != nil && trade.Price.Sign() > 0
}

func paperTradeTime(trade spot.Trade) time.Time {
	if trade.Time == nil {
		return time.Time{}
	}

	sec := trade.Time.Float64()
	return time.Unix(0, int64(sec*1e9)).UTC()
}

func sameVenuePair(left, right string) bool {
	norm := func(symbol string) string {
		symbol = strings.ToUpper(strings.ReplaceAll(symbol, "/", ""))
		return strings.ReplaceAll(symbol, "XBT", "BTC")
	}

	return norm(left) == norm(right)
}

func ordersToResume(position *Position, open spot.OpenOrdersResult) []resumedOrder {
	if position == nil || position.Virtual || len(open.Open) == 0 {
		return nil
	}

	out := make([]resumedOrder, 0, 2)

	for _, leg := range []struct {
		request *spot.OrderRequest
		side    string
	}{
		{position.EntryOrder, "buy"},
		{position.ExitOrder, "sell"},
	} {
		if leg.request == nil || leg.request.ClOrdId == "" || filledSide(position, leg.side) {
			continue
		}

		for orderID, row := range open.Open {
			if row.ClOrdID != leg.request.ClOrdId {
				continue
			}

			out = append(out, resumedOrder{
				orderID:       orderID,
				clientOrderID: leg.request.ClOrdId,
				pair:          position.Symbol,
				side:          leg.side,
			})
		}
	}

	return out
}

func filledSide(position *Position, side string) bool {
	if position == nil {
		return false
	}

	for _, fill := range position.Fills {
		if fill == nil || !strings.EqualFold(fill.Side, side) {
			continue
		}

		if fill.LastQty != nil && fill.LastQty.Sign() > 0 {
			return true
		}
	}

	return false
}

/*
Enter opens a lot on one book. The venue book and the virtual book are
independent: a learning sample does not occupy the venue slot, and a
venue lot does not occupy the learning slot. A second lot on the same
book is rejected.
*/
func (desk *Desk) Enter(
	symbol string, path string, virtual bool, pathFound bool,
) error {
	cache, key := desk.cacheFor(virtual)

	if cache.Has(symbol) {
		return errnie.Err(
			errnie.TooManyRequests,
			"[desk.Enter] "+key+" already exists",
			nil,
		)
	}

	qty, why := desk.price.Allocate(symbol)
	if qty == nil || qty.Sign() <= 0 {
		msg := "[desk.Enter] cannot size " + symbol
		if why != "" {
			msg += ": " + why
		}

		return errnie.Err(errnie.NotFound, msg, nil)
	}

	position := NewPosition(
		symbol,
		path,
		pathFound,
		virtual,
		desk.price,
		desk.transport,
		desk.onPositionSettled,
	)

	// Search on this symbol's settled record, then push what it learned onto
	// the lot. The stop reads the pullback off the lot, so the policy has to
	// be on it before the first mark reprices it.
	desk.refreshPolicy(symbol)
	desk.teachPolicy(position)

	// Register before Send. Paper publish blocks until the private ingress
	// reads the fill, and that read calls Apply. The lot has to already be
	// in the cache or the entry price is dropped and close-now PnL stays empty.
	cache.Add(position.Symbol, position)

	if err := position.Enter(path, qty); err != nil {
		cache.Drop(position.Symbol)
		return err
	}

	if err := desk.history.ReplaceOpen(data.NewValue(
		key, "open", symbol,
	).Append(
		position, "position",
	)); err != nil {
		cache.Drop(position.Symbol)

		return errnie.Error(errnie.Err(
			errnie.IO,
			"[desk.Enter] failed to store position",
			err,
		))
	}

	// The entry is live. Publish this lot; the rest of the book is already
	// on the page from the last snapshot or its own updates.
	io.Copy(ui.UI, data.NewValue("positions", "update", position.ID).Append(
		position, "position",
	))

	return nil
}

/*
Exit a position and report what settled.
*/
func (desk *Desk) Exit(
	symbol string, path string, virtual bool,
) error {
	cache, _ := desk.cacheFor(virtual)
	position := cache.Get(symbol, true)

	if position == nil {
		return errnie.Err(
			errnie.NotFound,
			"[desk.Exit] position not found",
			nil,
		)
	}

	if err := position.Exit(path); err != nil {
		if errors.Is(err, ErrExitInProgress) {
			return nil
		}

		return err
	}

	return desk.releaseLocked(position)
}

func (desk *Desk) releaseLocked(position *Position) error {
	if position == nil || position.Symbol == "" {
		return nil
	}

	if !position.settleOnce() {
		return nil
	}

	cache, key := desk.cacheFor(position.Virtual)

	if cache.Has(position.Symbol) {
		if position.Virtual {
			desk.virtuals.Remove(position.Symbol)
			desk.pending.Store(position.Symbol, position)
		} else {
			desk.positions.Remove(position.Symbol)
		}
	}

	// The round trip is over. Its trace is now an episode, and this symbol's
	// learned exit moment is one step closer to being worth trusting.
	desk.observePolicy(position)

	value := data.NewValue(
		key, "closed", position.Symbol,
	).Append(
		position, "position",
	)

	if err := desk.history.Move(
		value, key+"/open/"+position.Symbol+"/",
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[desk.Exit] failed to store position",
			err,
		))
	}

	io.Copy(ui.UI, data.NewValue("positions", "update", position.ID).Append(
		position, "position",
	))

	return nil
}

func (desk *Desk) TakePending(symbol string) (*Position, bool) {
	if desk == nil || symbol == "" {
		return nil, false
	}

	val, ok := desk.pending.LoadAndDelete(symbol)
	if !ok {
		return nil, false
	}

	position, ok := val.(*Position)
	return position, ok && position != nil
}

func (desk *Desk) Pending() []string {
	if desk == nil {
		return nil
	}

	return desk.pendingSymbols()
}
