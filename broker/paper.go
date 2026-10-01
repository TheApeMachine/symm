package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Paper is the simulated spot websocket and REST transport. It shells out to the
native `kraken paper` CLI so balances, fills, and history stay owned by the
venue ledger under Application Support — not an in-process invented matcher.
Private frames publish onto explicit typed subscriptions so Desk and tests use
the same direct wiring as the live transport.
*/
type Paper struct {
	*runtime.System
	commandGate atomic.Pointer[chan struct{}]
	executions  func(*kraken.Execution)
	// watching tracks limit orders that acknowledged open so a later CLI fill
	// can be published as kraken.Execution → ApplyExecution.
	watching sync.Map // orderID -> paperWatch
}

type paperWatch struct {
	orderID       string
	clientOrderID string
	pair          string
	side          string
}

/*
NewPaper opens the paper spot transport with explicit private subscriptions.
*/
func NewPaper(
	ctx context.Context,
) *Paper {
	paper := &Paper{}
	paper.System = runtime.NewSystem(ctx, "paper", paper)

	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	paper.commandGate.Store(&gate)

	return paper
}

/*
Initialize is a no-op; readiness follows the injected simulator.
*/
func (paper *Paper) Initialize() error {
	return nil
}

func (paper *Paper) OnExecution(handler func(*kraken.Execution)) {
	paper.executions = handler
}

/*
Balances loads the current paper wallet through the native CLI and returns the
same asset-to-decimal map used by Kraken's real REST balance endpoint.
*/
func (paper *Paper) Balances() (*kraken.Balance, error) {
	var (
		model datura.Map[any]
		err   error
	)

	model, err = paper.execute("balances", "balance", "--verbose")

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

	return kraken.NewPaperBalance(raw), nil
}

/*
Write routes the same subscription and order envelopes used by live transports
through the paper CLI under simulator latency.
*/
func (paper *Paper) Write(
	buf []byte,
) error {
	request := struct {
		Method string `json:"method"`
		ReqID  int64  `json:"req_id"`
		Params struct {
			Channel    string      `json:"channel"`
			ClOrdID    string      `json:"cl_ord_id"`
			OrderType  string      `json:"order_type"`
			Side       string      `json:"side"`
			Symbol     string      `json:"symbol"`
			OrderQty   json.Number `json:"order_qty"`
			LimitPrice json.Number `json:"limit_price"`
		} `json:"params"`
	}{}

	if err := sonic.Unmarshal(buf, &request); err != nil {
		return err
	}

	if request.Method == "add_order" {
		model, err := paper.placeOrder(
			request.Params.Side,
			request.Params.Symbol,
			request.Params.OrderQty.String(),
			request.Params.OrderType,
			request.Params.LimitPrice.String(),
			request.Params.ClOrdID,
		)

		if err != nil {
			return err
		}

		return paper.publishPlace(model, request.ReqID)
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
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"paper: something went wrong",
			nil,
		))
	}
}

/*
Post satisfies Conn; paper REST operations remain explicit typed methods.
*/
func (paper *Paper) Post(string, json.Marshaler) ([]byte, error) {
	return nil, errnie.Error(errnie.Err(
		errnie.Internal,
		"paper: something went wrong",
		nil,
	))
}

/*
ResetPaperAccount calls `kraken paper reset --yes` via the system shell to restore
the paper trading account to its default state.
*/
func ResetPaperAccount(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "kraken", "paper", "reset", "--yes", "--output", "json")

	if errors.Is(cmd.Err, exec.ErrDot) {
		cmd.Err = nil
	}

	stdout, err := cmd.Output()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"kraken paper reset failed",
			err,
		))
	}

	_ = stdout

	return nil
}

/*
Reset resets the paper trading account via ResetPaperAccount and publishes a fresh balance snapshot.
*/
func (paper *Paper) Reset() error {
	var err error

	err = ResetPaperAccount(paper.Context())

	if err != nil {
		return errnie.Error(err)
	}

	return paper.publishBalance("snapshot")
}

/*
TradesHistory loads paper fills from `kraken paper history`.
*/
func (paper *Paper) TradesHistory() (spot.TradesHistoryResult, error) {
	var (
		model datura.Map[any]
		err   error
	)

	model, err = paper.execute("history", "history", "--verbose")

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
OpenOrders loads the paper venue's working orders for restart reconciliation.
*/
func (paper *Paper) OpenOrders() (spot.OpenOrdersResult, error) {
	var (
		model datura.Map[any]
		err   error
	)

	model, err = paper.execute("orders", "orders", "--verbose")

	if err != nil {
		return spot.OpenOrdersResult{}, errnie.Error(err)
	}

	raw, err := sonic.Marshal(model["open_orders"])

	if err != nil {
		return spot.OpenOrdersResult{}, errnie.Error(err)
	}

	rows := []struct {
		ID            string           `json:"id"`
		OrderID       string           `json:"order_id"`
		ClientOrderID string           `json:"cl_ord_id"`
		Pair          string           `json:"pair"`
		Side          string           `json:"side"`
		OrderType     string           `json:"order_type"`
		Status        string           `json:"status"`
		Volume        *decimal.Decimal `json:"volume"`
		Filled        *decimal.Decimal `json:"filled_volume"`
		Price         *decimal.Decimal `json:"price"`
	}{}

	if err := sonic.Unmarshal(raw, &rows); err != nil {
		return spot.OpenOrdersResult{}, errnie.Error(err)
	}

	result := spot.OpenOrdersResult{Open: map[string]spot.Order{}}

	for _, row := range rows {
		orderID := row.OrderID

		if orderID == "" {
			orderID = row.ID
		}

		if orderID == "" || row.Pair == "" || row.Side == "" {
			return spot.OpenOrdersResult{}, errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"paper open order is missing identity, pair, or side",
				nil,
			))
		}

		result.Open[orderID] = spot.Order{
			ClOrdID:        row.ClientOrderID,
			Status:         row.Status,
			Volume:         row.Volume,
			VolumeExecuted: row.Filled,
			Price:          row.Price,
			Description: &spot.OrderDescription{
				Pair:      row.Pair,
				Type:      row.Side,
				OrderType: row.OrderType,
			},
		}
	}

	return result, nil
}

/*
CancelOrder cancels one paper order by its venue order identifier.
*/
func (paper *Paper) CancelOrder(
	request *spot.CancelOrderRequest,
) (spot.CancelResult, error) {
	if request == nil {
		return spot.CancelResult{}, errnie.Error(errnie.Err(
			errnie.Validation, "paper cancel order is missing its request", nil,
		))
	}

	orderID, _ := request.TxID.(string)

	if orderID == "" {
		return spot.CancelResult{}, errnie.Error(errnie.Err(
			errnie.Validation, "paper cancel order requires a venue order ID", nil,
		))
	}

	var err error

	_, err = paper.execute("cancel", "cancel", orderID, "--yes")

	if err != nil {
		return spot.CancelResult{}, errnie.Error(err)
	}

	return spot.CancelResult{Count: 1}, nil
}

/*
TradeBalance returns the paper account status.

kraken paper status --verbose --output json
[verbose] GET https://api.kraken.com/0/public/Ticker
[verbose] Response 200 OK: {"error":[],"result":{"WARDUSD":{"a":["0.003190000","7863","7863.000"],"b":["0.003160000","19402","19402.000"],"c":["0.003160000","598.18293"],"v":["2637691.59698","2668461.40298"],"p":["0.003233967","0.003234724"],"t":[462,466],"l":["0.003100000","0.003100000"],"h":["0.003620000","0.003620000"],"o":"0.003290000"}}}
{"current_value":199.16016971858227,"fee_rate":0.0026,"mode":"paper","open_orders":0,"slippage_rate":0.0,"starting_balance":200.0,"starting_currency":"USD","total_trades":5,"unrealized_pnl":-0.8398302814177327,"unrealized_pnl_pct":-0.4199151407088664,"valuation_complete":true}
*/
func (paper *Paper) TradeBalance() (*kraken.TradeBalanceResult, error) {
	var (
		model  datura.Map[any]
		wallet datura.Map[any]
		err    error
	)

	model, err = paper.execute("status", "status", "--verbose")

	if err == nil {
		wallet, err = paper.execute("balances", "balance", "--verbose")
	}

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal, "[paper] failed to get trade balance", err,
		))
	}

	result := kraken.NewTradeBalanceFromMap(model)
	quote, valid := model["starting_currency"].(string)

	if !valid || quote == "" {
		return &result, errnie.Error(errnie.Err(
			errnie.Validation, "[paper] quote currency required", nil,
		))
	}

	raw, err := sonic.Marshal(wallet)

	if err != nil {
		return &result, errnie.Error(errnie.Err(
			errnie.Validation, "[paper] failed to marshal wallet", nil,
		))
	}

	var balance kraken.PaperBalance

	if err := sonic.Unmarshal(raw, &balance); err != nil {
		return &result, errnie.Error(errnie.Err(
			errnie.Validation, "[paper] complete wallet required", nil,
		))
	}

	if balance.Balances == nil {
		return &result, errnie.Error(errnie.Err(
			errnie.Validation, "[paper] complete wallet required", nil,
		))
	}

	result.AvailableCash = decimal.NewFromInt64(0)

	if row, found := balance.Balances[quote]; found {
		result.AvailableCash = row.Available
	}

	return &result, nil
}

/*
TradeVolume returns the paper account's taker-fee schedule for the requested
pairs. Paper charges one configured fee rate across every pair, so the schedule
is the same fee under each requested canonical symbol. The simulator reports fee_rate as a fraction; TradeVolumeFee.Fee is a
percentage, so the fraction is scaled into its percent form.
*/
func (paper *Paper) TradeVolume(symbols []string) (*kraken.TradeVolumeResult, error) {
	var model datura.Map[any]
	var err error

	model, err = paper.execute("status", "status", "--verbose")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get paper fee schedule",
			err,
		))
	}

	feeRate, ok := model["fee_rate"].(float64)

	if !ok || feeRate <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"paper fee schedule requires a positive fee rate",
			nil,
		))
	}

	feePercent := decimal.NewFromFloat64(feeRate).Mul(decimal.NewFromInt64(100))
	fees := make(map[string]kraken.TradeVolumeFee, len(symbols))
	feesMaker := make(map[string]kraken.TradeVolumeFee, len(symbols))

	for _, symbol := range symbols {
		fee := kraken.TradeVolumeFee{
			Fee:    feePercent,
			Minfee: feePercent,
			Maxfee: feePercent,
		}

		fees[symbol] = fee
		feesMaker[symbol] = fee
	}

	return &kraken.TradeVolumeResult{Fees: fees, FeesMaker: feesMaker}, nil
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
	clientOrderID, _ := request["cl_ord_id"].(string)

	model, err := paper.placeOrder(
		side,
		symbol,
		quantity,
		orderType,
		limitPrice,
		clientOrderID,
	)

	if err != nil {
		return spot.AddOrderResult{}, err
	}

	if err := paper.publishPlace(model, 0); err != nil {
		return spot.AddOrderResult{}, err
	}

	orderID, _ := model["order_id"].(string)

	if orderID == "" {
		return spot.AddOrderResult{}, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"paper order acknowledgement is missing its order ID",
			nil,
		))
	}

	return spot.AddOrderResult{
		OrderPlacementSingle: spot.OrderPlacementSingle{ID: []string{orderID}},
	}, nil
}

func (paper *Paper) gate() chan struct{} {
	for {
		ptr := paper.commandGate.Load()

		if ptr != nil {
			return *ptr
		}

		ch := make(chan struct{}, 1)
		ch <- struct{}{}

		if paper.commandGate.CompareAndSwap(nil, &ch) {
			return ch
		}
	}
}

func (paper *Paper) execute(entity string, command ...string) (datura.Map[any], error) {
	gate := paper.gate()

	select {
	case <-gate:
		defer func() { gate <- struct{}{} }()
	case <-paper.Context().Done():
		return nil, paper.Context().Err()
	}

	input := []string{"paper"}
	input = append(input, command...)
	input = append(input, "--output", "json")

	cmd := exec.CommandContext(paper.Context(), "kraken", input...)

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
func (paper *Paper) Balance() (*kraken.Balance, error) {
	return paper.Balances()
}

/*
publishBalance emits a paper wallet frame from `kraken paper balance --verbose`.
*/
func (paper *Paper) publishBalance(frameType string) error {
	balance, err := paper.Balances()

	if err != nil {
		return err
	}

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
publishPlace emits order ack + execution, then soft-fails balance refresh.
Open limit acks are tracked so a later CLI fill becomes a real Execution.
*/
func (paper *Paper) publishPlace(
	model datura.Map[any],
	reqID int64,
) error {
	orderAck := kraken.NewOrderResponseFromMap(model, reqID)
	paper.publish("add_order", orderAck)

	execution := kraken.NewExecutionFromMap(model)
	paper.publish("executions", execution)

	if paperExecutionStillOpen(execution) {
		paper.watchOpenExecution(execution)
	}

	// Success of the execution must not fail if only the wallet refresh fails.
	if err := paper.publishBalance("snapshot"); err != nil {
		errnie.Warn("[paper] balance refresh after place failed: " + err.Error())
	}

	return nil
}

func paperExecutionStillOpen(execution *kraken.Execution) bool {
	if execution == nil || len(execution.Data) == 0 {
		return false
	}
	for _, row := range execution.Data {
		status := strings.ToLower(row.OrderStatus)
		execType := strings.ToLower(row.ExecType)
		if status == "open" || status == "pending" || status == "new" ||
			execType == "new" || execType == "pending_new" {
			return true
		}
	}
	return false
}

func (paper *Paper) watchOpenExecution(execution *kraken.Execution) {
	if paper == nil || execution == nil {
		return
	}
	for _, row := range execution.Data {
		orderID := row.OrderID
		if orderID == "" {
			continue
		}
		watch := paperWatch{
			orderID:       orderID,
			clientOrderID: row.ClientOrderID,
			pair:          row.Symbol,
			side:          row.Side,
		}
		if _, loaded := paper.watching.LoadOrStore(orderID, watch); loaded {
			continue
		}
		go paper.pollOpenFill(watch)
	}
}

/*
pollOpenFill converts a later CLI fill/cancel into an Execution for ApplyExecution.
Paper has no private WS; OpenOrders + TradesHistory is the venue ledger.
*/
func (paper *Paper) pollOpenFill(watch paperWatch) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.Now().Add(2 * time.Minute)

	for {
		select {
		case <-paper.Context().Done():
			paper.watching.Delete(watch.orderID)
			return
		case <-ticker.C:
		}

		if time.Now().After(deadline) {
			paper.watching.Delete(watch.orderID)
			errnie.Warn("[paper] open order watch timed out: " + watch.orderID)
			return
		}

		open, err := paper.OpenOrders()
		if err != nil {
			continue
		}
		if _, stillOpen := open.Open[watch.orderID]; stillOpen {
			continue
		}

		// Order left the open book — look up the fill in history.
		history, err := paper.TradesHistory()
		if err != nil {
			continue
		}

		for tradeID, trade := range history.Trades {
			if trade.OrderID != watch.orderID {
				continue
			}
			fill := datura.Map[any]{
				"order_id":     watch.orderID,
				"cl_ord_id":    watch.clientOrderID,
				"id":           tradeID,
				"pair":         trade.Pair,
				"side":         trade.Type,
				"price":        trade.Price.Float64(),
				"cost":         trade.Cost.Float64(),
				"fee":          trade.Fee.Float64(),
				"volume":       trade.Volume.Float64(),
				"time":         trade.Time.String(),
				"status":       "filled",
				"order_status": "filled",
				"exec_type":    "trade",
			}
			if watch.pair != "" {
				fill["pair"] = watch.pair
			}
			if watch.side != "" {
				fill["side"] = watch.side
			}
			paper.publish("executions", kraken.NewExecutionFromMap(fill))
			if err := paper.publishBalance("snapshot"); err != nil {
				errnie.Warn("[paper] balance refresh after fill failed: " + err.Error())
			}
			paper.watching.Delete(watch.orderID)
			return
		}

		// Not in open orders and no matching trade → canceled/expired.
		cancel := datura.Map[any]{
			"order_id":     watch.orderID,
			"cl_ord_id":    watch.clientOrderID,
			"pair":         watch.pair,
			"side":         watch.side,
			"status":       "canceled",
			"order_status": "canceled",
			"exec_type":    "canceled",
			"action":       "order_cancelled",
		}
		paper.publish("executions", kraken.NewExecutionFromMap(cancel))
		paper.watching.Delete(watch.orderID)
		return
	}
}

func (paper *Paper) placeOrder(
	side string,
	symbol string,
	quantity string,
	orderType string,
	limitPrice string,
	clientOrderID string,
) (datura.Map[any], error) {
	if quantity == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"paper: order_qty is required (empty volume string)",
			nil,
		))
	}

	command := []string{side, symbol, quantity}

	if orderType == "limit" && limitPrice != "" {
		command = append(command, "--type", "limit", "--price", limitPrice)
	}

	var (
		model datura.Map[any]
		err   error
	)

	model, err = paper.execute("executions", command...)

	if err != nil {
		// Keep insufficient-available / validation place failures soft so
		// Execution/Training stay READY instead of cascading to ERROR.
		if IsEnterSoftFail(err) {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"paper: order rejected — insufficient available funds or below minimum",
				err,
			))
		}

		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place paper order",
			err,
		))
	}

	model["pair"] = symbol
	model["cl_ord_id"] = clientOrderID
	return model, nil
}

func (paper *Paper) publish(channel string, payload any) {
	if channel == "executions" && paper.executions != nil {
		paper.executions(payload.(*kraken.Execution))
	}
}
