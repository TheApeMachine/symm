package websocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Paper is the simulated spot websocket and REST transport. It shells out to the
native `kraken paper` CLI so balances, fills, and history stay owned by the
venue ledger under Application Support — not an in-process invented matcher.
Private frames publish onto explicit typed subscriptions so Desk and tests use
the same direct wiring as the live transport.
*/
type Paper struct {
	ctx         context.Context
	cancel      context.CancelFunc
	simulator   *Simulator
	subMu       sync.Mutex
	subscribers map[string][]*types.Subscription[any]
	books       *spot.BookManager
}

/*
NewPaper opens the paper spot transport with explicit private subscriptions.
*/
func NewPaper(
	ctx context.Context,
	simulator *Simulator,
) *Paper {
	ctx, cancel := context.WithCancel(ctx)

	paper := &Paper{
		ctx:         ctx,
		cancel:      cancel,
		simulator:   simulator,
		subscribers: map[string][]*types.Subscription[any]{},
	}

	return paper
}

/*
Initialize is a no-op; readiness follows the injected simulator.
*/
func (paper *Paper) Initialize() error {
	return nil
}

/*
Status reports the backing simulator status.
*/
func (paper *Paper) Status() types.Status {
	return paper.simulator.Status()
}

/*
Balances loads the current paper wallet through the native CLI and returns the
same asset-to-decimal map used by Kraken's real REST balance endpoint.
*/
func (paper *Paper) Balances() (map[string]*decimal.Decimal, error) {
	var (
		model datura.Map[any]
		err   error
	)

	paper.simulator.Do(REST, func() {
		model, err = paper.execute("balances", "balance", "--verbose")
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get paper balances",
			err,
		))
	}

	raw, err := sonic.Marshal(model)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"failed to encode paper balances",
			err,
		))
	}

	return kraken.NewPaperBalance(raw).Totals(), nil
}

/*
Subscribe registers one consumer for a named paper channel.
*/
func (paper *Paper) Subscribe(
	key string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	paper.subMu.Lock()
	paper.subscribers[key] = append(paper.subscribers[key], subscription)
	paper.subMu.Unlock()

	return subscription
}

func (paper *Paper) SubInstrument(types.Subscription[any]) {}
func (paper *Paper) SubTicker([]string)                    {}
func (paper *Paper) SubBook([]string)                      {}
func (paper *Paper) SubTrades([]string)                    {}
func (paper *Paper) SubL3([]string)                        {}
func (paper *Paper) SubCandles([]string)                   {}

/*
Books returns no public book cache because paper hijacks the private transport
only; the public websocket remains the source of market data.
*/
func (paper *Paper) Books() map[string]*book.Book {
	return nil
}

/*
Book returns no public book because paper does not own market subscriptions.
*/
func (paper *Paper) Book(string) *book.Book {
	return nil
}

/*
Write routes the same subscription and order envelopes used by live transports
through the paper CLI under simulator latency.
*/
func (paper *Paper) Write(
	params json.Marshaler,
	callbacks ...Callback[any],
) error {
	raw, err := params.MarshalJSON()

	if err != nil {
		return err
	}

	request := struct {
		Method string `json:"method"`
		ReqID  int64  `json:"req_id"`
		Params struct {
			Channel    string      `json:"channel"`
			OrderType  string      `json:"order_type"`
			Side       string      `json:"side"`
			Symbol     string      `json:"symbol"`
			OrderQty   json.Number `json:"order_qty"`
			LimitPrice json.Number `json:"limit_price"`
		} `json:"params"`
	}{}

	if err := sonic.Unmarshal(raw, &request); err != nil {
		return err
	}

	if request.Method == "add_order" {
		model, err := paper.placeOrder(
			request.Params.Side,
			request.Params.Symbol,
			request.Params.OrderQty.String(),
			request.Params.OrderType,
			request.Params.LimitPrice.String(),
		)

		if err != nil {
			return err
		}

		return paper.publishPlace(model, request.ReqID, callbacks)
	}

	switch request.Params.Channel {
	case "balances":
		return paper.publishBalance("snapshot")
	case "executions":
		history, err := paper.TradesHistory()

		if err != nil {
			return err
		}

		trades := make([]any, 0, len(history.Trades))

		for tradeID, trade := range history.Trades {
			trades = append(trades, map[string]any{
				"id":       tradeID,
				"order_id": trade.OrderID,
				"pair":     trade.Pair,
				"side":     trade.Type,
				"price":    trade.Price.Float64(),
				"cost":     trade.Cost.Float64(),
				"fee":      trade.Fee.Float64(),
				"volume":   trade.Volume.Float64(),
				"time":     trade.Time.String(),
				"status":   "filled",
			})
		}

		return paper.Replay(trades)
	default:
		return types.ClosedError{Component: "paper:" + request.Params.Channel}
	}
}

/*
Post satisfies Conn; paper REST operations remain explicit typed methods.
*/
func (paper *Paper) Post(string, json.Marshaler) ([]byte, error) {
	return nil, types.ClosedError{Component: "paper:rest"}
}

/*
Close cancels the paper transport context.
*/
func (paper *Paper) Close() {
	paper.cancel()
}

/*
TradesHistory loads paper fills from `kraken paper history`.
*/
func (paper *Paper) TradesHistory() (spot.TradesHistoryResult, error) {
	var (
		model datura.Map[any]
		err   error
	)

	paper.simulator.Do(REST, func() {
		model, err = paper.execute("history", "history", "--verbose")
	})

	if err != nil {
		return spot.TradesHistoryResult{}, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get trades history",
			err,
		))
	}

	return kraken.NewTradesHistoryFromMap(model), nil
}

/*
TradeBalance returns the paper account status.

kraken paper status --verbose --output json
[verbose] GET https://api.kraken.com/0/public/Ticker
[verbose] Response 200 OK: {"error":[],"result":{"WARDUSD":{"a":["0.003190000","7863","7863.000"],"b":["0.003160000","19402","19402.000"],"c":["0.003160000","598.18293"],"v":["2637691.59698","2668461.40298"],"p":["0.003233967","0.003234724"],"t":[462,466],"l":["0.003100000","0.003100000"],"h":["0.003620000","0.003620000"],"o":"0.003290000"}}}
{"current_value":199.16016971858227,"fee_rate":0.0026,"mode":"paper","open_orders":0,"slippage_rate":0.0,"starting_balance":200.0,"starting_currency":"USD","total_trades":5,"unrealized_pnl":-0.8398302814177327,"unrealized_pnl_pct":-0.4199151407088664,"valuation_complete":true}
*/
func (paper *Paper) TradeBalance() (kraken.TradeBalanceResult, error) {
	var (
		model datura.Map[any]
		err   error
	)

	paper.simulator.Do(REST, func() {
		model, err = paper.execute("status", "status", "--verbose")
	})

	if err != nil {
		return kraken.TradeBalanceResult{}, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get trade balance",
			err,
		))
	}

	return kraken.NewTradeBalanceFromMap(model), nil
}

/*
AddOrder places through `kraken paper buy|sell` under simulator latency.
*/
func (paper *Paper) AddOrder(order *spot.AddOrderRequest) (spot.AddOrderResult, error) {
	raw, err := sonic.Marshal(order)

	if err != nil {
		return spot.AddOrderResult{}, err
	}

	request := map[string]any{}

	if err := sonic.Unmarshal(raw, &request); err != nil {
		return spot.AddOrderResult{}, err
	}

	side, _ := request["type"].(string)
	symbol, _ := request["pair"].(string)
	quantity, _ := request["volume"].(string)
	orderType, _ := request["ordertype"].(string)
	limitPrice, _ := request["price"].(string)

	model, err := paper.placeOrder(side, symbol, quantity, orderType, limitPrice)

	if err != nil {
		return spot.AddOrderResult{}, err
	}

	if err := paper.publishPlace(model, 0, nil); err != nil {
		return spot.AddOrderResult{}, err
	}

	return spot.AddOrderResult{}, nil
}

func (paper *Paper) execute(entity string, command ...string) (datura.Map[any], error) {
	input := []string{"paper"}
	input = append(input, command...)
	input = append(input, "--output", "json")

	cmd := exec.CommandContext(paper.ctx, "kraken", input...)

	if errors.Is(cmd.Err, exec.ErrDot) {
		cmd.Err = nil
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.Output()

	if err != nil {
		details := strings.TrimSpace(stderr.String())

		if details == "" {
			details = strings.TrimSpace(string(stdout))
		}

		if details == "" {
			details = err.Error()
		}

		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"kraken paper "+entity+" command failed: "+details,
			err,
		))
	}

	model := datura.Map[any]{}

	if err := sonic.Unmarshal(stdout, &model); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to decode kraken paper "+entity,
			err,
		))
	}

	if errCategory, ok := model["error"].(string); ok && errCategory != "" {
		message, _ := model["message"].(string)

		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken paper: "+message,
			nil,
		))
	}

	return model, nil
}

/*
Balance returns the paper wallet in the same asset-total map shape as the real
private REST balance endpoint.
*/
func (paper *Paper) Balance() (map[string]*decimal.Decimal, error) {
	return paper.Balances()
}

/*
publishBalance emits a paper wallet frame from `kraken paper balance`.
*/
func (paper *Paper) publishBalance(frameType string) error {
	var model datura.Map[any]
	var err error

	paper.simulator.Do(REST, func() {
		model, err = paper.execute("balances", "balance")
	})

	if err != nil {
		return err
	}

	balance := kraken.NewBalanceFromMap(model)
	balance.Type = frameType

	paper.publish("balances", balance)
	return nil
}

/*
Replay emits historical paper fills as execution frames.
*/
func (paper *Paper) Replay(trades []any) error {
	for tradeIndex, tradeRaw := range trades {
		trade, ok := tradeRaw.(map[string]any)

		if !ok {
			continue
		}

		execution := kraken.NewExecutionFromMap(datura.Map[any](trade))

		if tradeIndex == 0 {
			execution.Type = "snapshot"
		}

		paper.publish("executions", execution)
	}

	return nil
}

/*
publishPlace emits order ack, fill, and a balance snapshot for one paper order.
*/
func (paper *Paper) publishPlace(
	model datura.Map[any],
	reqID int64,
	callbacks []Callback[any],
) error {
	orderAck := kraken.NewOrderResponseFromMap(model, reqID)
	paper.publish("add_order", orderAck)

	for _, callback := range callbacks {
		if callback.Channel != "add_order" {
			continue
		}

		callback.Send(orderAck)
	}

	paper.publish("executions", kraken.NewExecutionFromMap(model))

	// Paper balance is a full wallet dump that omits zero assets. Emitting it
	// as an incremental update leaves stale positive rows in Balance and keeps
	// phantom OPEN lots.
	return paper.publishBalance("snapshot")
}

func (paper *Paper) placeOrder(
	side string,
	symbol string,
	quantity string,
	orderType string,
	limitPrice string,
) (datura.Map[any], error) {
	command := []string{side, symbol, quantity}

	if orderType == "limit" && limitPrice != "" {
		command = append(command, "--type", "limit", "--price", limitPrice)
	}

	var (
		model datura.Map[any]
		err   error
	)

	paper.simulator.Do(REST, func() {
		model, err = paper.execute("executions", command...)
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place paper order",
			err,
		))
	}

	model["pair"] = symbol
	return model, nil
}

func (paper *Paper) publish(channel string, message any) {
	paper.subMu.Lock()
	subscribers := append([]*types.Subscription[any](nil), paper.subscribers[channel]...)
	paper.subMu.Unlock()

	for _, subscriber := range subscribers {
		subscriber.Send(message)
	}
}
