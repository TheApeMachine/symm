package mockapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	balancesfixture "github.com/theapemachine/symm/tests/fixtures/balances"
	executionfixture "github.com/theapemachine/symm/tests/fixtures/execution"
	orderackfixture "github.com/theapemachine/symm/tests/fixtures/orderack"
)

/*
PaperOptions provides the authoritative touch, ledger, fee, and event clock
required by the deterministic paper venue.
*/
type PaperOptions struct {
	Quote    func(symbol string) (bid, ask float64, exists bool)
	Now      func() time.Time
	Balances map[string]float64
	FeeRate  float64
}

/*
Paper matches market and simple limit orders against the simulated touch while
maintaining balances and resting orders from the same economic state.
*/
type Paper struct {
	mu       sync.Mutex
	options  PaperOptions
	balances map[string]float64
	open     map[string]paperOrder
	nextID   uint64
	nextExec uint64
}

type paperOrder struct {
	id       string
	reqID    int64
	symbol   string
	side     string
	typ      string
	quantity float64
	limit    float64
}

type orderRequest struct {
	Method string `json:"method"`
	ReqID  int64  `json:"req_id"`
	Params struct {
		OrderType  string      `json:"order_type"`
		Side       string      `json:"side"`
		OrderQty   json.Number `json:"order_qty"`
		Symbol     string      `json:"symbol"`
		LimitPrice json.Number `json:"limit_price"`
	} `json:"params"`
}

/*
EnablePaper attaches the small execution ledger used by the production paper
path without replacing the injected Conn boundary.
*/
func (conn *MockConn) EnablePaper(options PaperOptions) error {
	if options.Quote == nil || options.Now == nil || len(options.Balances) == 0 ||
		options.FeeRate < 0 {
		return errnie.Err(errnie.Validation, "tests/mockapi: complete paper options required", nil)
	}

	balances := make(map[string]float64, len(options.Balances))

	for asset, balance := range options.Balances {
		balances[asset] = balance
	}

	conn.paperMu.Lock()
	conn.paper = &Paper{
		options:  options,
		balances: balances,
		open:     map[string]paperOrder{},
	}
	conn.paperMu.Unlock()
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
Handle validates one add_order request and queues its acknowledgement plus any
immediate crossing fill.
*/
func (paper *Paper) Handle(raw []byte) ([]outbound, error) {
	request, err := decodeOrder(raw)

	if err != nil {
		return nil, err
	}

	paper.mu.Lock()
	defer paper.mu.Unlock()
	paper.nextID++
	order := paperOrder{
		id:       fmt.Sprintf("PAPER-%05d", paper.nextID),
		reqID:    request.ReqID,
		symbol:   request.Params.Symbol,
		side:     request.Params.Side,
		typ:      request.Params.OrderType,
		quantity: number(request.Params.OrderQty),
		limit:    number(request.Params.LimitPrice),
	}

	if err := paper.validate(order); err != nil {
		return nil, err
	}

	frames := []outbound{{
		channel: "add_order",
		payload: orderackfixture.Frame(orderackfixture.Options{
			ReqID:   order.reqID,
			OrderID: order.id,
			Success: true,
		}),
	}}
	bid, ask, _ := paper.options.Quote(order.symbol)

	if order.typ == "market" || order.side == "buy" && order.limit >= ask ||
		order.side == "sell" && order.limit <= bid {
		return append(frames, paper.fill(order, bid, ask)...), nil
	}

	paper.open[order.id] = order
	frames = append(frames, paper.execution(order, 0, "new", "open"))
	return frames, nil
}

/*
Match evaluates resting limits against the latest deterministic touch.
*/
func (paper *Paper) Match() ([]outbound, error) {
	paper.mu.Lock()
	defer paper.mu.Unlock()
	frames := []outbound{}

	for orderID, order := range paper.open {
		bid, ask, exists := paper.options.Quote(order.symbol)

		if !exists || order.side == "buy" && order.limit < ask ||
			order.side == "sell" && order.limit > bid {
			continue
		}

		frames = append(frames, paper.fill(order, bid, ask)...)
		delete(paper.open, orderID)
	}

	return frames, nil
}

/*
OpenOrders returns independent paper order entries keyed by venue identity.
*/
func (paper *Paper) OpenOrders() map[string]spot.Order {
	paper.mu.Lock()
	defer paper.mu.Unlock()
	orders := make(map[string]spot.Order, len(paper.open))

	for orderID := range paper.open {
		orders[orderID] = spot.Order{}
	}

	return orders
}

/*
fill applies one all-or-nothing touch execution and returns execution and
balance frames derived from the updated ledger.
*/
func (paper *Paper) fill(order paperOrder, bid, ask float64) []outbound {
	price := ask

	if order.side == "sell" {
		price = bid
	}

	base, quote, _ := strings.Cut(order.symbol, "/")
	cost := order.quantity * price
	fee := cost * paper.options.FeeRate

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
			payload: balancesfixture.Frame(paper.balances, balancesfixture.UPDATE),
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
			FeeUsdEquiv: text(cost * paper.options.FeeRate),
			Timestamp:   paper.options.Now().Format(time.RFC3339Nano),
		}),
	}
}

/*
validate rejects orders that cannot be executed or represented by the current
simulated market and ledger.
*/
func (paper *Paper) validate(order paperOrder) error {
	if order.symbol == "" || order.quantity <= 0 ||
		order.side != "buy" && order.side != "sell" ||
		order.typ != "market" && order.typ != "limit" {
		return errnie.Err(errnie.Validation, "tests/mockapi: valid order required", nil)
	}

	if order.typ == "limit" && order.limit <= 0 {
		return errnie.Err(errnie.Validation, "tests/mockapi: limit price required", nil)
	}

	bid, ask, exists := paper.options.Quote(order.symbol)

	if !exists || bid <= 0 || ask <= bid {
		return errnie.Err(errnie.Validation, "tests/mockapi: executable quote required", nil)
	}

	base, quote, ok := strings.Cut(order.symbol, "/")

	if !ok {
		return errnie.Err(errnie.Validation, "tests/mockapi: normalized order symbol required", nil)
	}

	price := ask

	if order.side == "sell" {
		price = bid
	}

	if order.side == "buy" && paper.balances[quote] < order.quantity*price*(1+paper.options.FeeRate) ||
		order.side == "sell" && paper.balances[base] < order.quantity {
		return errnie.Err(errnie.Validation, "tests/mockapi: insufficient paper balance", nil)
	}

	return nil
}

/*
decodeOrder preserves exact JSON numbers while parsing a generic add_order
request accepted by the Conn interface.
*/
func decodeOrder(raw []byte) (orderRequest, error) {
	request := orderRequest{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	if err := decoder.Decode(&request); err != nil {
		return request, errnie.Err(errnie.Validation, "tests/mockapi: decode order", err)
	}

	return request, nil
}

/*
number parses an optional exact wire number without converting through an
untyped JSON map.
*/
func number(value json.Number) float64 {
	parsed, _ := value.Float64()
	return parsed
}
