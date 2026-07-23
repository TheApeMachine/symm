package mockapi

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
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
	quantity := order.quantity

	if execType == "new" {
		quantity = 0
	}

	cost := quantity * price
	text := func(value float64) string {
		return strconv.FormatFloat(value, 'f', 8, 64)
	}

	return outbound{
		channel: "executions",
		payload: executionfixture.Frame(executionfixture.Options{
			OrderID:     order.id,
			ExecID:      fmt.Sprintf("EXEC-%05d", paper.nextExec),
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
			Timestamp:   paper.options.Now().Format(time.RFC3339Nano),
		}),
	}
}
