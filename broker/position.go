package broker

import (
	"sort"
	"strconv"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

var orderStatuses = map[string]types.Status{
	"open":              types.OPEN,
	"filled":            types.FILLED,
	"cancelled":         types.CANCELLED,
	"rejected":          types.REJECTED,
	"expired":           types.EXPIRED,
	"partial":           types.PARTIAL,
	"partial_filled":    types.PARTIAL_FILLED,
	"partially_filled":  types.PARTIAL_FILLED,
	"partial_cancelled": types.PARTIAL_CANCELLED,
	"partial_rejected":  types.PARTIAL_REJECTED,
	"partial_expired":   types.PARTIAL_EXPIRED,
}

type PositionData struct {
	Symbol     string          `json:"symbol"`
	Qty        decimal.Decimal `json:"qty"`
	EntryPrice decimal.Decimal `json:"entry_price"`
	Mark       decimal.Decimal `json:"mark"`
	PnL        decimal.Decimal `json:"pnl"`
	ReturnPct  float64         `json:"return_pct"`
}

type StopData struct {
	Symbol     string          `json:"symbol"`
	Armed      bool            `json:"-"`
	PeakPrice  decimal.Decimal `json:"-"`
	StopPrice  decimal.Decimal `json:"stop_price"`
	PeakReturn float64         `json:"peak_return"`
	StopReturn float64         `json:"stop_return"`
}

type Position struct {
	status        types.Status
	api           *websocket.API
	ui            chan []byte
	price         *Price
	balance       *Balance
	orderID       string
	clientID      int
	reqID         int
	requestedQty  decimal.Decimal
	priorQty      decimal.Decimal
	currentAction string
	order         *kraken.LimitOrder
	executions    []*kraken.Execution
	Data          *PositionData
	Stop          *StopData
	tickers       []*kraken.TickerData
	thesis        *types.Thesis
	snapshot      func() []PositionData
}

func NewPosition(
	api *websocket.API,
	ui chan []byte,
	price *Price,
	balance *Balance,
	data *PositionData,
	thesis *types.Thesis,
	snapshot func() []PositionData,
) *Position {
	position := &Position{
		status:     types.INITIALIZING,
		api:        api,
		ui:         ui,
		price:      price,
		balance:    balance,
		Data:       data,
		thesis:     thesis,
		snapshot:   snapshot,
		executions: make([]*kraken.Execution, 0),
		tickers:    make([]*kraken.TickerData, 0),
	}

	position.api.On("add_order", position.OrderAck)
	position.api.On("executions", position.ExecutionAck)
	position.api.On("ticker", position.TickerAck)

	return position
}

/*
Thesis returns the lifecycle record associated when this Position was opened
or reconciled. Strategy uses it to append later management decisions in place.
*/
func (position *Position) Thesis() *types.Thesis {
	return position.thesis
}

/*
record appends one immutable position fact to the associated lifecycle. A nil
Thesis is permitted only for directly constructed positions in focused tests.
*/
func (position *Position) record(observation types.TradeObservation) {
	if position.thesis == nil {
		return
	}

	position.thesis.RecordTrade(observation)
}

func (position *Position) Status() types.Status {
	return position.status
}

/*
Publish broadcasts the full current position set, not just this one.

The frontend treats every "positions" message as the complete
authoritative state and replaces its store with it wholesale. Sending
only this position here would mean any single position's own update
(a fill, an order ack, its next ticker) wipes every other open
position off the dashboard. snapshot is nil for positions built
directly, such as in tests that construct a Position without a Desk,
in which case this position is all there is to report.
*/
func (position *Position) Publish() {
	positions := []PositionData{*position.Data}

	if position.snapshot != nil {
		positions = position.snapshot()
	}

	position.ui <- datura.Map[any]{
		"positions": positions,
	}.Marshal()
}

/*
Hydrate connects a position to an existing wallet holding and its
corresponding buy trade.

Price owns the valuation calculation. Position only stores the result.

The current ticker may not have arrived yet this early in boot, since
ticker subscriptions for the whole tradable universe are still in
flight. A missing quote does not mean the holding is not real, so the
position still opens on its confirmed quantity and entry price; Mark,
PnL and ReturnPct stay at whatever Price already computed (or zero) and
self-correct the moment TickerAck sees this symbol.
*/
func (position *Position) Hydrate(
	symbol string,
	history *kraken.TradesHistory,
) *Position {
	if errnie.Error(kraken.Validate(history)) != nil {
		return position
	}

	if position.balance == nil ||
		position.api == nil ||
		position.price == nil {
		return position
	}

	position.Data.Symbol = symbol

	holding, ok := position.holding(symbol)

	if !ok {
		return position
	}

	if err := position.reconcile(history, symbol, holding); err != nil {
		position.status = types.ERROR
		position.record(types.TradeObservation{
			Kind: "reconciliation_error", Symbol: symbol, Error: err.Error(), At: time.Now(),
		})
		errnie.Error(err)

		return position
	}

	quote, err := position.price.PositionQuote(
		position.Data.Symbol,
		position.Data.EntryPrice,
		position.Data.Qty,
	)

	if err != nil {
		errnie.Warn(
			"position quote pending for " + position.Data.Symbol + ": " + err.Error(),
		)
	} else {
		position.Data.Mark = quote.Mark
		position.Data.PnL = quote.PnL
		position.Data.ReturnPct = quote.ReturnPct
	}

	position.status = types.OPEN

	if err := position.thesis.Transition(
		symbol, types.LifecycleManaging, time.Now(),
	); err != nil {
		position.status = types.ERROR
		errnie.Error(err)

		return position
	}

	position.Publish()

	return position
}

/*
reconcile matches chronological sells against earlier buys and retains only the
actual buy lots still represented by the wallet holding. This prevents closed
round trips from contaminating the cost basis of a newly managed position.
*/
func (position *Position) reconcile(
	history *kraken.TradesHistory,
	symbol string,
	holding SpotHolding,
) error {
	trades := make([]struct {
		id    string
		trade spot.Trade
	}, 0)

	for id, trade := range history.Result.Trades {
		if position.balance.Symbol(trade.Pair) != symbol {
			continue
		}

		if (trade.Type != "buy" && trade.Type != "sell") || trade.Price == nil ||
			trade.Cost == nil || trade.Fee == nil || trade.Volume == nil ||
			trade.Time == nil || trade.Volume.Sign() <= 0 {
			return errnie.Err(errnie.Validation, "incomplete trade history for "+symbol, nil)
		}

		trades = append(trades, struct {
			id    string
			trade spot.Trade
		}{id: id, trade: trade})
	}

	sort.Slice(trades, func(left, right int) bool {
		order := trades[left].trade.Time.Cmp(trades[right].trade.Time)

		if order == 0 {
			return trades[left].id < trades[right].id
		}

		return order < 0
	})

	lots := make([]struct {
		id        string
		trade     spot.Trade
		remaining decimal.Decimal
	}, 0)

	for _, item := range trades {
		if item.trade.Type == "buy" {
			lots = append(lots, struct {
				id        string
				trade     spot.Trade
				remaining decimal.Decimal
			}{id: item.id, trade: item.trade, remaining: *item.trade.Volume.Copy()})

			continue
		}

		remaining := item.trade.Volume.Copy()

		for index := range lots {
			if remaining.Sign() <= 0 {
				break
			}

			consumed := remaining

			if lots[index].remaining.Cmp(remaining) < 0 {
				consumed = lots[index].remaining.Copy()
			}

			lots[index].remaining = *lots[index].remaining.Sub(consumed)
			remaining = remaining.Sub(consumed)
		}

		if remaining.Sign() > 0 {
			return errnie.Err(errnie.Validation, "sell exceeds known inventory for "+symbol, nil)
		}
	}

	quantityScale := int64(0)
	costScale := int64(0)

	for _, lot := range lots {
		if lot.remaining.GetScale() > quantityScale {
			quantityScale = lot.remaining.GetScale()
		}

		if lot.trade.Cost.GetScale() > costScale {
			costScale = lot.trade.Cost.GetScale()
		}
	}

	calculationScale := max(quantityScale, costScale)
	totalQuantity := decimal.NewFromInt64(0).SetScale(calculationScale)
	totalCost := decimal.NewFromInt64(0).SetScale(calculationScale)

	for _, lot := range lots {
		if lot.remaining.Sign() <= 0 {
			continue
		}

		ratio := lot.remaining.Div(lot.trade.Volume)
		cost := lot.trade.Cost.Mul(ratio)
		fee := lot.trade.Fee.Mul(ratio)
		tradeAt := time.Unix(0, int64(lot.trade.Time.Float64()*float64(time.Second)))
		execution := &kraken.Execution{Data: []kraken.ExecutionData{{
			OrderID: lot.trade.OrderID, ExecID: lot.id, ExecType: "trade",
			Symbol: symbol, Side: "buy", LastQty: lot.remaining.Float64(),
			LastPrice: *lot.trade.Price, Cost: *cost, FeeUsdEquiv: *fee,
			Timestamp: tradeAt, OrderStatus: "filled",
		}}}
		position.executions = append(position.executions, execution)
		position.record(types.TradeObservation{
			Kind: "position_reconciliation", Symbol: symbol, Side: "buy",
			OrderID: lot.trade.OrderID, ExecutionID: lot.id,
			Quantity: lot.remaining.String(), Price: lot.trade.Price.String(),
			Cost: cost.String(), Fee: fee.String(), At: tradeAt,
		})
		totalQuantity = totalQuantity.Add(&lot.remaining)
		totalCost = totalCost.Add(cost)
	}

	if totalQuantity.Cmp(&holding.Qty) != 0 || totalQuantity.Sign() <= 0 {
		return errnie.Err(errnie.Validation, "trade history does not reconcile wallet quantity for "+symbol, nil)
	}

	position.Data.Qty = holding.Qty
	position.Data.EntryPrice = *totalCost.Div(totalQuantity)

	return nil
}

/*
holding returns the wallet holding whose asset is symbol's own base
asset, skipping the quote currency and empty holdings.

Hydrate previously took whichever non-quote holding it iterated to
first and paired it with symbol's own entry price, so a wallet holding
several assets (e.g. GALA, BTC) could hydrate the BTC/USD position
with GALA's quantity the moment GALA happened to sort before BTC.
*/
func (position *Position) holding(symbol string) (SpotHolding, bool) {
	for _, holding := range position.balance.Holdings() {
		if holding.Asset == position.balance.quote || holding.Qty.Sign() <= 0 {
			continue
		}

		if position.balance.Symbol(holding.Asset) == symbol {
			return holding, true
		}
	}

	return SpotHolding{}, false
}

func (position *Position) OrderAck(buf []byte) {
	orderAck := kraken.NewOrderResponse(buf)

	if errnie.Error(kraken.Validate(orderAck)) != nil {
		position.status = types.ERROR
		position.record(types.TradeObservation{
			Kind: "order_acknowledgement", Symbol: position.Data.Symbol,
			Status: string(types.ERROR), Error: orderAck.Error, At: orderAck.TimeOut,
		})
		return
	}

	if orderAck.ReqID != position.reqID {
		return
	}

	position.orderID = orderAck.Result.OrderID
	position.clientID = orderAck.Result.OrderUserref
	position.status = types.OPEN
	position.record(types.TradeObservation{
		Kind: "order_acknowledgement", Symbol: position.Data.Symbol,
		Status: string(position.status), OrderID: position.orderID, At: orderAck.TimeOut,
	})

	position.Publish()
}

func (position *Position) ExecutionAck(buf []byte) {
	execution := kraken.NewExecution(buf)

	if errnie.Error(kraken.Validate(execution)) != nil {
		position.status = types.ERROR
		return
	}

	matchedExecution := &kraken.Execution{
		Channel:  execution.Channel,
		Type:     execution.Type,
		Data:     make([]kraken.ExecutionData, 0),
		Sequence: execution.Sequence,
	}
	matchedAt := time.Time{}

	for _, executionData := range execution.Data {
		if executionData.OrderID != position.orderID {
			continue
		}

		cumulativeQuantity := decimal.NewFromFloat64(
			executionData.CumQty,
		)

		switch executionData.Side {
		case "buy":
			position.Data.Qty = *cumulativeQuantity
			position.Data.EntryPrice = executionData.AvgPrice

		case "sell":
			position.Data.Qty = *position.priorQty.Sub(
				cumulativeQuantity,
			)
		}

		position.Data.Mark = executionData.LastPrice

		if status, ok := orderStatuses[executionData.OrderStatus]; ok {
			position.status = status
		}

		next := ""

		if executionData.Side == "buy" && position.status == types.PARTIAL_FILLED {
			next = types.LifecyclePartiallyEntered
		}

		if executionData.Side == "buy" && position.status == types.FILLED {
			next = types.LifecycleEntered
		}

		if executionData.Side == "sell" && position.currentAction == "exit" &&
			position.status == types.PARTIAL_FILLED {
			next = types.LifecyclePartiallyExited
		}

		if executionData.Side == "sell" && position.currentAction == "exit" &&
			position.Data.Qty.Sign() <= 0 {
			next = types.LifecycleClosed
		}

		if next != "" && position.thesis != nil {
			if err := position.thesis.Transition(
				position.Data.Symbol, next, executionData.Timestamp,
			); err != nil {
				position.status = types.ERROR
				position.record(types.TradeObservation{
					Kind: "execution_error", Symbol: position.Data.Symbol,
					Error: err.Error(), At: executionData.Timestamp,
				})
				errnie.Error(err)

				return
			}
		}

		position.record(types.TradeObservation{
			Kind:        "execution",
			Symbol:      executionData.Symbol,
			Side:        executionData.Side,
			Status:      executionData.OrderStatus,
			OrderID:     executionData.OrderID,
			ExecutionID: executionData.ExecID,
			Quantity: strconv.FormatFloat(
				executionData.LastQty, 'f', -1, 64,
			),
			Price: executionData.LastPrice.String(),
			Cost:  executionData.Cost.String(),
			Fee:   executionData.FeeUsdEquiv.String(),
			At:    executionData.Timestamp,
		})

		matchedExecution.Data = append(matchedExecution.Data, executionData)
		matchedAt = executionData.Timestamp
	}

	if len(matchedExecution.Data) > 0 {
		if err := position.Execution(matchedExecution); err != nil {
			position.status = types.ERROR
			return
		}

		position.executions = append(
			position.executions,
			matchedExecution,
		)

		if position.currentAction == "reduce" && position.status == types.FILLED {
			position.status = types.OPEN
			position.record(types.TradeObservation{
				Kind: "position_snapshot", Action: "reduce", Symbol: position.Data.Symbol,
				Status: string(types.OPEN), Quantity: position.Data.Qty.String(),
				Price: position.Data.Mark.String(), At: matchedAt,
			})
		}

		if position.Data.Qty.Sign() <= 0 {
			position.status = types.CLOSED
			outcome, err := position.price.Reconcile(
				position.Data.Symbol, position.executions,
			)

			if err != nil {
				position.record(types.TradeObservation{
					Kind: "reconciliation_error", Symbol: position.Data.Symbol,
					Status: string(types.CLOSED), Error: err.Error(), At: matchedAt,
				})
				errnie.Error(err)
			} else {
				position.Data.Mark = outcome.Mark
				position.Data.PnL = outcome.PnL
				position.Data.ReturnPct = outcome.ReturnPct
				fees := outcome.EntryFee.Add(&outcome.ExitFee)
				position.record(types.TradeObservation{
					Kind: "final_outcome", Symbol: position.Data.Symbol,
					Status: string(types.CLOSED), Quantity: position.Data.Qty.String(),
					Price: position.Data.Mark.String(), Fee: fees.String(),
					PnL: position.Data.PnL.String(), ReturnPct: position.Data.ReturnPct,
					At: matchedAt,
				})
			}

			position.record(types.TradeObservation{
				Kind: "position_snapshot", Symbol: position.Data.Symbol,
				Status: string(types.CLOSED), Quantity: position.Data.Qty.String(),
				Price: position.Data.Mark.String(), At: matchedAt,
			})
		}

		//Refresh the PnL after the execution changes quantity
		//or average entry price.
		if position.Data.Qty.Sign() > 0 &&
			position.Data.EntryPrice.Sign() > 0 {
			quote, err := position.price.PositionQuote(
				position.Data.Symbol,
				position.Data.EntryPrice,
				position.Data.Qty,
			)

			if err == nil {
				position.Data.Mark = quote.Mark
				position.Data.PnL = quote.PnL
				position.Data.ReturnPct = quote.ReturnPct
			} else {
				errnie.Error(err)
			}
		}
	}

	position.Publish()
}

/*
Execution validates an execution belonging to this position.
Fee and PnL calculations do not belong here. Price owns those
calculations centrally.
*/
func (position *Position) Execution(
	execution *kraken.Execution,
) error {
	if errnie.Error(kraken.Validate(execution)) != nil {
		position.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"invalid execution",
			nil,
		))
	}

	return nil
}

func (position *Position) Enter() error {
	position.requestedQty = position.Data.Qty
	position.Data.Qty = *decimal.NewFromInt64(0)

	/*
		Taker returns the estimated quote-currency cost of buying
		the requested quantity, including one taker fee.
	*/
	amount, err := position.price.Taker(
		position.Data.Symbol,
		position.requestedQty,
	)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get taker: "+err.Error(),
			err,
		))
	}

	if !position.balance.Available(*amount) {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"insufficient balance",
			nil,
		))
	}

	order := kraken.NewMarketOrder(
		"buy",
		position.requestedQty.Float64(),
		position.Data.Symbol,
	)

	position.reqID = order.ReqID
	position.status = types.PENDING

	if position.thesis != nil {
		if err := position.thesis.Transition(
			position.Data.Symbol, types.LifecycleEntrySubmitted, time.Now(),
		); err != nil {
			position.status = types.ERROR

			return errnie.Error(err)
		}
	}

	position.record(types.TradeObservation{
		Kind: "order_submission", Action: "enter", Symbol: position.Data.Symbol,
		Side: "buy", Status: string(position.status),
		Quantity: position.requestedQty.String(), At: time.Now(),
	})

	if err := position.api.AddOrder(order); err != nil {
		position.status = types.ERROR

		if position.thesis != nil {
			errnie.Error(position.thesis.Transition(
				position.Data.Symbol, types.LifecycleRejected, time.Now(),
			))
		}

		position.record(types.TradeObservation{
			Kind: "execution_error", Action: "enter", Symbol: position.Data.Symbol,
			Side: "buy", Status: string(position.status), Error: err.Error(), At: time.Now(),
		})

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	position.Publish()

	return nil
}

func (position *Position) Exit(action string, quantity decimal.Decimal) error {
	if quantity.Sign() <= 0 || quantity.Cmp(&position.Data.Qty) > 0 {
		return errnie.Error(errnie.Err(
			errnie.Forbidden,
			"position does not contain requested sell quantity",
			nil,
		))
	}

	position.priorQty = position.Data.Qty
	position.requestedQty = quantity
	position.currentAction = action

	order := kraken.NewMarketOrder(
		"sell",
		position.requestedQty.Float64(),
		position.Data.Symbol,
	)

	position.reqID = order.ReqID
	position.status = types.PENDING

	if position.thesis != nil && action == "exit" {
		if err := position.thesis.Transition(
			position.Data.Symbol, types.LifecycleExitSubmitted, time.Now(),
		); err != nil {
			position.status = types.ERROR

			return errnie.Error(err)
		}
	}

	position.record(types.TradeObservation{
		Kind: "order_submission", Action: action, Symbol: position.Data.Symbol,
		Side: "sell", Status: string(position.status),
		Quantity: position.requestedQty.String(), At: time.Now(),
	})

	if err := position.api.AddOrder(order); err != nil {
		position.status = types.ERROR

		if position.thesis != nil && action == "exit" {
			errnie.Error(position.thesis.Transition(
				position.Data.Symbol, types.LifecycleManaging, time.Now(),
			))
		}
		position.record(types.TradeObservation{
			Kind: "execution_error", Action: action, Symbol: position.Data.Symbol,
			Side: "sell", Status: string(position.status), Error: err.Error(), At: time.Now(),
		})

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	position.Publish()

	return nil
}

func (position *Position) Executions() []*kraken.Execution {
	return position.executions
}

/*
TickerAck updates this position from its own ticker only.

Position does not perform fee, notional, PnL, or return calculations.
It delegates the entire valuation to Price.
*/
func (position *Position) TickerAck(buf []byte) {
	ticker := kraken.NewTicker(buf)

	if errnie.Error(kraken.Validate(ticker)) != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"invalid ticker",
			nil,
		))

		return
	}

	for _, tickerData := range ticker.Data {
		if tickerData.Symbol != position.Data.Symbol ||
			tickerData.Last == nil ||
			position.Data.EntryPrice.Sign() <= 0 ||
			position.Data.Qty.Sign() <= 0 {
			continue
		}

		quote, err := position.price.PositionQuoteAt(
			position.Data.Symbol,
			position.Data.EntryPrice,
			*tickerData.Last,
			position.Data.Qty,
		)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to get position quote",
				err,
			))

			return
		}

		position.Data.Mark = quote.Mark
		position.Data.PnL = quote.PnL
		position.Data.ReturnPct = quote.ReturnPct
		position.record(types.TradeObservation{
			Kind: "position_snapshot", Symbol: position.Data.Symbol,
			Status: string(position.status), Quantity: position.Data.Qty.String(),
			Price: position.Data.Mark.String(), PnL: position.Data.PnL.String(),
			ReturnPct: position.Data.ReturnPct, At: tickerData.Timestamp,
		})

		position.Publish()

		return
	}
}
