package mockapi

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	balancesfixture "github.com/theapemachine/symm/tests/fixtures/balances"
	executionfixture "github.com/theapemachine/symm/tests/fixtures/execution"
)

/*
PaperOptions provides the authoritative touch, ledger, fee, and event clock
required by the deterministic paper venue.
*/
type PaperOptions struct {
	Quote    func(symbol string) (bid, bidQty, ask, askQty float64, exists bool)
	Now      func() time.Time
	Balances map[string]float64
	MakerFee float64
	TakerFee float64
}

/*
Paper matches market and limit orders against displayed touch liquidity while
maintaining reservations and balances. Own-order fills are intentionally
non-impacting because public market impact is driven through Market actions.
*/
type Paper struct {
	mu          sync.Mutex
	options     PaperOptions
	balances    map[string]float64
	reserved    map[string]float64
	open        map[string]paperOrder
	order       []string
	history     map[string]spot.Trade
	nextID      uint64
	nextExec    uint64
	nextBalance int64
}

/*
EnablePaper attaches the small execution ledger used by the production paper
path without replacing the injected Conn boundary.
*/
func (conn *MockConn) EnablePaper(options PaperOptions) error {
	if options.Quote == nil || options.Now == nil || len(options.Balances) == 0 ||
		options.MakerFee < 0 || options.TakerFee < 0 {
		return errnie.Err(errnie.Validation, "tests/mockapi: complete paper options required", nil)
	}

	balances := make(map[string]float64, len(options.Balances))

	for asset, balance := range options.Balances {
		balances[asset] = balance
	}

	paper := &Paper{
		options:  options,
		balances: balances,
		reserved: map[string]float64{},
		open:     map[string]paperOrder{},
		history:  map[string]spot.Trade{},
	}
	conn.paperMu.Lock()
	conn.paper = paper
	conn.paperMu.Unlock()
	conn.RespondCurrent("balances", paper.BalanceSnapshot)
	conn.RespondCurrent("executions", paper.ExecutionSnapshot)
	return nil
}

/*
MatchPaper fills every resting limit that crosses the current authoritative
touch and queues the resulting private frames.
*/
func (conn *MockConn) MatchPaper() error {
	conn.paperMu.Lock()
	paper := conn.paper
	conn.paperMu.Unlock()

	if !conn.Active() {
		return errnie.Err(errnie.IO, "tests/mockapi: paper connection closed", nil)
	}

	if paper == nil {
		return nil
	}

	frames, err := paper.Match()

	if err != nil {
		return err
	}

	for _, frame := range frames {
		if !conn.Subscribed(frame.channel) {
			continue
		}

		if err := conn.Queue(frame.channel, frame.payload); err != nil {
			return err
		}
	}

	return nil
}

/*
OpenOrders exposes the paper venue's exact resting order identities for boot
reconciliation through websocket.API.OpenOrders.
*/
func (conn *MockConn) OpenOrders() (map[string]spot.Order, error) {
	conn.paperMu.Lock()
	paper := conn.paper
	conn.paperMu.Unlock()

	if !conn.Active() {
		return nil, errnie.Err(errnie.IO, "tests/mockapi: paper connection closed", nil)
	}

	if paper == nil {
		return map[string]spot.Order{}, nil
	}

	return paper.OpenOrders(), nil
}

/*
TradesHistory exposes the paper venue execution ledger for broker recovery
through websocket.API.TradesHistory.
*/
func (conn *MockConn) TradesHistory() (*kraken.TradesHistory, error) {
	conn.paperMu.Lock()
	paper := conn.paper
	conn.paperMu.Unlock()

	if !conn.Active() {
		return nil, errnie.Err(errnie.IO, "tests/mockapi: paper connection closed", nil)
	}

	if paper == nil {
		return &kraken.TradesHistory{
			Result: kraken.TradesHistoryResult{Trades: map[string]spot.Trade{}},
		}, nil
	}

	return paper.TradesHistory(), nil
}

/*
BalanceSnapshot exposes the current paper wallet frame for API.AccountBalances.
*/
func (conn *MockConn) BalanceSnapshot() (*kraken.Balance, error) {
	conn.paperMu.Lock()
	paper := conn.paper
	conn.paperMu.Unlock()

	if !conn.Active() {
		return nil, errnie.Err(errnie.IO, "tests/mockapi: paper connection closed", nil)
	}

	if paper == nil {
		return &kraken.Balance{Channel: "balances", Type: "snapshot", Data: []kraken.BalanceData{}}, nil
	}

	return kraken.NewBalance(paper.BalanceSnapshot()), nil
}

/*
TradeBalance exposes a deterministic trade-balance summary for integration tests
through the same API surface production uses.
*/
func (conn *MockConn) TradeBalance(asset string) (*kraken.TradeBalanceResult, error) {
	conn.paperMu.Lock()
	paper := conn.paper
	conn.paperMu.Unlock()

	if !conn.Active() {
		return nil, errnie.Err(errnie.IO, "tests/mockapi: paper connection closed", nil)
	}

	if paper == nil {
		return &kraken.TradeBalanceResult{
			EquivalentBalance: decimal.NewFromInt64(0),
			TradeBalance:      decimal.NewFromInt64(0),
			MarginAmount:      decimal.NewFromInt64(0),
			UnrealizedPnL:     decimal.NewFromInt64(0),
			CostBasis:         decimal.NewFromInt64(0),
			Valuation:         decimal.NewFromInt64(0),
			Equity:            decimal.NewFromInt64(0),
			FreeMargin:        decimal.NewFromInt64(0),
			MarginFreeOrders:  decimal.NewFromInt64(0),
			UnexecutedValue:   decimal.NewFromInt64(0),
		}, nil
	}

	return paper.TradeBalance(asset), nil
}

/*
Match evaluates resting limits against the latest deterministic touch.
*/
func (paper *Paper) Match() ([]outbound, error) {
	paper.mu.Lock()
	defer paper.mu.Unlock()
	draft := Paper{
		options:     paper.options,
		balances:    make(map[string]float64, len(paper.balances)),
		reserved:    make(map[string]float64, len(paper.reserved)),
		open:        make(map[string]paperOrder, len(paper.open)),
		order:       append([]string(nil), paper.order...),
		history:     make(map[string]spot.Trade, len(paper.history)),
		nextID:      paper.nextID,
		nextExec:    paper.nextExec,
		nextBalance: paper.nextBalance,
	}

	for asset, balance := range paper.balances {
		draft.balances[asset] = balance
	}

	for asset, reserved := range paper.reserved {
		draft.reserved[asset] = reserved
	}

	for orderID, order := range paper.open {
		draft.open[orderID] = order
	}

	for tradeID, trade := range paper.history {
		draft.history[tradeID] = trade
	}

	frames := []outbound{}
	remaining := make([]string, 0, len(draft.order))
	liquidity := map[string]*struct {
		bid float64
		ask float64
	}{}

	for _, orderID := range draft.order {
		order, exists := draft.open[orderID]

		if !exists {
			continue
		}

		bid, bidQty, ask, askQty, exists := draft.options.Quote(order.symbol)

		if !exists || order.side == "buy" && order.limit < ask ||
			order.side == "sell" && order.limit > bid {
			remaining = append(remaining, orderID)
			continue
		}

		budget := liquidity[order.symbol]

		if budget == nil {
			budget = &struct {
				bid float64
				ask float64
			}{bid: bidQty, ask: askQty}
			liquidity[order.symbol] = budget
		}

		if order.side == "buy" && order.quantity > budget.ask ||
			order.side == "sell" && order.quantity > budget.bid {
			return nil, errnie.Err(
				errnie.Validation,
				"tests/mockapi: resting order exceeds touch liquidity",
				nil,
			)
		}

		frames = append(frames, draft.fill(order, bid, ask)...)
		delete(draft.open, orderID)

		if order.side == "buy" {
			budget.ask -= order.quantity
		}

		if order.side == "sell" {
			budget.bid -= order.quantity
		}
	}

	draft.order = remaining
	paper.balances = draft.balances
	paper.reserved = draft.reserved
	paper.open = draft.open
	paper.order = draft.order
	paper.history = draft.history
	paper.nextID = draft.nextID
	paper.nextExec = draft.nextExec
	paper.nextBalance = draft.nextBalance

	return frames, nil
}

/*
OpenOrders returns independent paper order entries keyed by venue identity.
*/
func (paper *Paper) OpenOrders() map[string]spot.Order {
	paper.mu.Lock()
	defer paper.mu.Unlock()
	orders := make(map[string]spot.Order, len(paper.open))

	for orderID, order := range paper.open {
		orders[orderID] = order.snapshot()
	}

	return orders
}

/*
TradesHistory returns a stable copy of the paper fill ledger keyed by execution id.
*/
func (paper *Paper) TradesHistory() *kraken.TradesHistory {
	paper.mu.Lock()
	defer paper.mu.Unlock()

	trades := make(map[string]spot.Trade, len(paper.history))

	for tradeID, trade := range paper.history {
		trades[tradeID] = trade
	}

	return &kraken.TradesHistory{
		Result: kraken.TradesHistoryResult{Trades: trades},
	}
}

/*
TradeBalance summarizes current quote cash plus executable bid valuation of open
paper balances using the same field shape as Kraken trade balance.
*/
func (paper *Paper) TradeBalance(asset string) *kraken.TradeBalanceResult {
	normalized := strings.TrimPrefix(strings.TrimSpace(asset), "Z")

	if normalized == "" {
		normalized = "USD"
	}

	paper.mu.Lock()
	balanceSnapshot := make(map[string]float64, len(paper.balances))
	history := make(map[string]spot.Trade, len(paper.history))
	quote := paper.options.Quote

	for symbol, total := range paper.balances {
		balanceSnapshot[symbol] = total
	}

	for tradeID, trade := range paper.history {
		history[tradeID] = trade
	}

	quoteCash := decimal.NewFromFloat64(balanceSnapshot[normalized])
	paper.mu.Unlock()

	if quote == nil {
		return &kraken.TradeBalanceResult{
			EquivalentBalance: decimal.NewFromInt64(0),
			TradeBalance:      decimal.NewFromInt64(0),
			MarginAmount:      decimal.NewFromInt64(0),
			UnrealizedPnL:     decimal.NewFromInt64(0),
			CostBasis:         decimal.NewFromInt64(0),
			Valuation:         decimal.NewFromInt64(0),
			Equity:            decimal.NewFromInt64(0),
			FreeMargin:        decimal.NewFromInt64(0),
			MarginFreeOrders:  decimal.NewFromInt64(0),
			UnexecutedValue:   decimal.NewFromInt64(0),
		}
	}

	costBasis := decimal.NewFromInt64(0)
	valuation := decimal.NewFromInt64(0)

	for symbol, basis := range paper.openBasis(history) {
		qty := balanceSnapshot[symbol]

		if qty <= 0 {
			continue
		}

		bid, _, _, _, ok := quote(symbol + "/" + normalized)

		if !ok {
			continue
		}

		costBasis = costBasis.Add(basis)
		valuation = valuation.Add(decimal.NewFromFloat64(bid * qty))
	}

	unrealized := valuation.Sub(costBasis)
	tradeBalance := quoteCash.Add(costBasis)
	equity := tradeBalance.Add(unrealized)

	return &kraken.TradeBalanceResult{
		EquivalentBalance: equity.Copy(),
		TradeBalance:      tradeBalance,
		MarginAmount:      decimal.NewFromInt64(0),
		UnrealizedPnL:     unrealized,
		CostBasis:         costBasis,
		Valuation:         valuation,
		Equity:            equity,
		FreeMargin:        equity.Copy(),
		MarginFreeOrders:  equity.Copy(),
		UnexecutedValue:   decimal.NewFromInt64(0),
	}
}

func (paper *Paper) openBasis(history map[string]spot.Trade) map[string]*decimal.Decimal {
	keys := make([]string, 0, len(history))

	for tradeID := range history {
		keys = append(keys, tradeID)
	}

	sort.Slice(keys, func(left, right int) bool {
		leftTrade := history[keys[left]]
		rightTrade := history[keys[right]]

		if leftTrade.Time == nil && rightTrade.Time == nil {
			return keys[left] < keys[right]
		}

		if leftTrade.Time == nil {
			return true
		}

		if rightTrade.Time == nil {
			return false
		}

		if diff := leftTrade.Time.Cmp(rightTrade.Time); diff != 0 {
			return diff < 0
		}

		return keys[left] < keys[right]
	})
	basis := map[string]*decimal.Decimal{}
	quantity := map[string]*decimal.Decimal{}

	for _, tradeID := range keys {
		trade := history[tradeID]
		symbol, _, ok := strings.Cut(trade.Pair, "/")

		if !ok || symbol == "" || trade.Volume == nil || trade.Cost == nil {
			continue
		}

		side := strings.ToLower(trade.Type)

		if side == "buy" {
			if basis[symbol] == nil {
				basis[symbol] = trade.Cost.Copy()
				quantity[symbol] = trade.Volume.Copy()
				continue
			}

			basis[symbol] = basis[symbol].Add(trade.Cost)
			quantity[symbol] = quantity[symbol].Add(trade.Volume)
			continue
		}

		if side != "sell" || basis[symbol] == nil || quantity[symbol] == nil {
			continue
		}

		remainingQty := quantity[symbol].Sub(trade.Volume)

		if remainingQty.Sign() <= 0 {
			delete(basis, symbol)
			delete(quantity, symbol)
			continue
		}

		averageCost := basis[symbol].Div(quantity[symbol])
		basis[symbol] = averageCost.Mul(remainingQty)
		quantity[symbol] = remainingQty
	}

	return basis
}

/*
fill applies one all-or-nothing non-impacting touch execution and returns
execution and balance frames derived from the updated ledger.
*/
func (paper *Paper) fill(order paperOrder, bid, ask float64) []outbound {
	price := ask

	if order.side == "sell" {
		price = bid
	}

	base, quote, _ := strings.Cut(order.symbol, "/")
	cost := order.quantity * price
	feeRate := paper.options.TakerFee

	if order.maker {
		feeRate = paper.options.MakerFee
		paper.release(order)
	}

	fee := cost * feeRate

	if order.side == "buy" {
		paper.balances[quote] -= cost + fee
		paper.balances[base] += order.quantity
	}

	if order.side == "sell" {
		paper.balances[base] -= order.quantity
		paper.balances[quote] += cost - fee
	}

	return []outbound{
		paper.execution(order, price, "trade", "filled"),
		{
			channel: "balances",
			payload: paper.balanceFrame(balancesfixture.UPDATE),
		},
	}
}

/*
execution injects one order state into the existing Kraken execution template.
*/
func (paper *Paper) execution(
	order paperOrder,
	price float64,
	execType string,
	status string,
) outbound {
	paper.nextExec++
	execID := fmt.Sprintf("EXEC-%05d", paper.nextExec)
	quantity := order.quantity

	if execType == "new" {
		quantity = 0
	}

	cost := quantity * price
	timestamp := paper.options.Now()
	text := func(value float64) string {
		return strconv.FormatFloat(value, 'f', 8, 64)
	}

	if execType == "trade" && status == "filled" {
		paper.history[execID] = spot.Trade{
			OrderID:   order.id,
			Pair:      order.symbol,
			Time:      decimal.NewFromFloat64(float64(timestamp.UnixNano()) / 1e9),
			Type:      order.side,
			OrderType: order.typ,
			Price:     decimal.NewFromFloat64(price),
			Cost:      decimal.NewFromFloat64(cost),
			Fee:       decimal.NewFromFloat64(cost * paper.fee(order)),
			Volume:    decimal.NewFromFloat64(quantity),
			Maker:     order.maker,
		}
	}

	return outbound{
		channel: "executions",
		payload: executionfixture.Frame(executionfixture.Options{
			OrderID:     order.id,
			ExecID:      execID,
			Symbol:      order.symbol,
			Side:        order.side,
			LastQty:     text(quantity),
			LastPrice:   text(price),
			Cost:        text(cost),
			OrderStatus: status,
			OrderType:   order.typ,
			ExecType:    execType,
			CumQty:      text(quantity),
			CumCost:     text(cost),
			AvgPrice:    text(price),
			FeeUsdEquiv: text(cost * paper.fee(order)),
			Timestamp:   timestamp.Format(time.RFC3339Nano),
		}),
	}
}
