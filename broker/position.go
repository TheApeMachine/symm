package broker

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Position is one lot shell owned by Desk. Order correlation uses request ID then
exchange order ID; unmatched executions buffer until the ack binds them.
Position subscribes to the market ticker topic for live mark and stoploss
updates, and to the account execution and add_order topics for fills and
order acks so it handles its own lifecycle without Desk absorbing the logic.
*/
type Position struct {
	*types.Actor
	ctx        context.Context
	cancel     context.CancelFunc
	ID         string       `json:"id"`
	Status     types.Status `json:"status"`
	api        *websocket.API
	ui         chan []byte
	instrument *Instrument
	price      *Price
	balance    *Balance
	pair       kraken.InstrumentPair
	EntryOrder *kraken.MarketOrder `json:"entry_order"`
	ExitOrder  *kraken.MarketOrder `json:"exit_order"`
	OrderID    string              `json:"order_id"`
	intentID   string
	Fills      []Fill `json:"fills"`
	seenExec   map[string]struct{}
	Buffered   []kraken.ExecutionData `json:"buffered"`
	Holding    *types.Holding         `json:"holding"`
	closing    bool
	market     *types.Actor
	account    *types.Actor
}

/*
Fill is one immutable execution print used to derive lot economics.
*/
type Fill struct {
	ExecID string
	Side   string
	Qty    *decimal.Decimal
	Price  *decimal.Decimal
	Fee    *decimal.Decimal
}

type positionSnapshot struct {
	ID         string                 `json:"id"`
	Status     types.Status           `json:"status"`
	EntryOrder *kraken.MarketOrder    `json:"entry_order"`
	ExitOrder  *kraken.MarketOrder    `json:"exit_order"`
	OrderID    string                 `json:"order_id"`
	Fills      []Fill                 `json:"fills"`
	Buffered   []kraken.ExecutionData `json:"buffered"`
	Holding    *types.Holding         `json:"holding"`
}

/*
NewPosition constructs one lot shell; Desk routes order and execution
rows initially but Position subscribes to the market ticker, account
executions, and account add_order topics so it responds to live
websocket messages directly.
*/
func NewPosition(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	pair kraken.InstrumentPair,
	qty *decimal.Decimal,
	market *types.Actor,
	account *types.Actor,
) *Position {
	errnie.Info("creating position for: " + pair.Symbol)

	ctx, cancel := context.WithCancel(ctx)

	entryOrder := kraken.NewMarketOrder(
		"buy", pair.Symbol, qty,
	)

	exitOrder := kraken.NewMarketOrder(
		"sell", pair.Symbol, qty,
	)

	position := &Position{
		ctx:        ctx,
		cancel:     cancel,
		Status:     types.INITIALIZING,
		api:        api,
		ui:         ui,
		instrument: instrument,
		price:      price,
		balance:    balance,
		pair:       pair,
		seenExec:   make(map[string]struct{}),
		EntryOrder: entryOrder,
		ExitOrder:  exitOrder,
		market:     market,
		account:    account,
	}
	mark := entryOrder.Params.LimitPrice

	if mark == nil {
		mark = decimal.NewFromInt64(0)
	}

	position.Holding = types.NewHolding(
		ctx,
		pair.Symbol,
		entryOrder.Params.OrderQty,
		mark,
		position.Exit,
		position.Publish,
		market,
	)
	position.Actor = types.NewActor(ctx, "position", map[string]types.Handler{
		"add_order":  {Topic: "add_order", Fn: position.onOrder},
		"executions": {Topic: "executions", Fn: position.onExecutions},
		"ticker":     {Topic: "ticker", Fn: position.onTicker},
	})

	topics := make([]types.Topic, 0, 3)

	topics = append(topics,
		types.Topic{Name: "ticker", Actor: market},
		types.Topic{Name: "executions", Actor: account},
		types.Topic{Name: "add_order", Actor: account},
	)

	position.Actor.Initialize(topics...)
	position.Publish()

	return position
}

/*
NewDecisionPosition constructs one live position directly from the strategy
decision that selected and sized the entry.
*/
func NewDecisionPosition(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	pair kraken.InstrumentPair,
	decision *types.Decision,
	market *types.Actor,
	account *types.Actor,
) *Position {
	if decision == nil || decision.Symbol == "" || decision.ProposedQuantity == nil {
		panic("broker.Position: decision with symbol and quantity required")
	}

	position := NewPosition(
		ctx,
		api,
		ui,
		instrument,
		price,
		balance,
		pair,
		decision.ProposedQuantity,
		market,
		account,
	)
	position.ID = decision.ID

	if decision.ReferencePrice != nil {
		position.Holding.Mark = decision.ReferencePrice.Copy()
		position.Holding.Stoploss.Mark = decision.ReferencePrice.Copy()
		position.Holding.Stoploss.Entry = decision.ReferencePrice.Copy()
	}

	position.Holding.IsOpportunity = decision.Opportunity
	position.Holding.ReservationID = decision.ReservationID

	return position
}
func (position *Position) Initialize(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	pair kraken.InstrumentPair,
	market *types.Actor,
	account *types.Actor,
) {
	errnie.Info("initializing position for: " + pair.Symbol)

	position.ctx = ctx
	position.api = api
	position.ui = ui
	position.instrument = instrument
	position.price = price
	position.balance = balance
	position.pair = pair
	position.market = market
	position.account = account

	topics := make([]types.Topic, 0, 3)

	topics = append(topics,
		types.Topic{Name: "ticker", Actor: market},
		types.Topic{Name: "executions", Actor: account},
		types.Topic{Name: "add_order", Actor: account},
	)

	position.Actor.Initialize(topics...)
	position.Holding.Initialize(
		position.ctx,
		position.EntryOrder.Params.OrderQty,
		position.EntryOrder.Params.LimitPrice,
		position.Exit,
		position.Publish,
		market,
	)

	position.Publish()
}

/*
Close marks the lot closed once Desk drops it from the open map.
*/
func (position *Position) Close() (err error) {
	if position.Status == types.CLOSED {
		return nil
	}

	if position.cancel != nil {
		position.cancel()
	}

	if position.Holding != nil {
		err = errors.Join(err, position.Holding.Close())
	}

	position.closing = false
	position.Status = types.CLOSED

	return errnie.Error(err)
}

/*
Publish the position to the UI, which will automatically marshal the Holding
and its Stoploss into the JSON payload. For clarity, the balance is kept out
of this, as there must be a way to get that more accurate to reality, where
the exchange publishes the wallet state at the sensible moments. The paper
trading implementation we use is based on the kraken-cli, where under normal
use you would also not be manually managing the balances.
*/
func (position *Position) Publish() {
	if position.ui == nil {
		return
	}

	payload := datura.NewMap(
		"positions", []positionSnapshot{position.snapshot()},
	).MarshalAndFree()

	if len(payload) == 0 {
		return
	}

	select {
	case <-position.ctx.Done():
		return
	case position.ui <- payload:
	}

	position.balance.PublishTradeBalance()
}

func (position *Position) snapshot() positionSnapshot {
	return positionSnapshot{
		ID:         position.ID,
		Status:     position.Status,
		EntryOrder: position.copyOrder(position.EntryOrder),
		ExitOrder:  position.copyOrder(position.ExitOrder),
		OrderID:    position.OrderID,
		Fills:      position.copyFills(),
		Buffered:   position.copyBuffered(),
		Holding:    position.copyHolding(),
	}
}

func (position *Position) copyOrder(order *kraken.MarketOrder) *kraken.MarketOrder {
	if order == nil {
		return nil
	}

	copy := &kraken.MarketOrder{
		Method: order.Method,
		ReqID:  order.ReqID,
		Params: kraken.MarketOrderParams{
			OrderType:  order.Params.OrderType,
			Side:       order.Params.Side,
			Symbol:     order.Params.Symbol,
			OrderQty:   copyDecimal(order.Params.OrderQty),
			LimitPrice: copyDecimal(order.Params.LimitPrice),
		},
	}

	return copy
}

func (position *Position) copyFills() []Fill {
	if len(position.Fills) == 0 {
		return nil
	}

	fills := make([]Fill, len(position.Fills))

	for index, fill := range position.Fills {
		fills[index] = Fill{
			ExecID: fill.ExecID,
			Side:   fill.Side,
			Qty:    copyDecimal(fill.Qty),
			Price:  copyDecimal(fill.Price),
			Fee:    copyDecimal(fill.Fee),
		}
	}

	return fills
}

func (position *Position) copyBuffered() []kraken.ExecutionData {
	if len(position.Buffered) == 0 {
		return nil
	}

	buffered := make([]kraken.ExecutionData, len(position.Buffered))

	for index, row := range position.Buffered {
		buffered[index] = kraken.ExecutionData{
			OrderID:      row.OrderID,
			OrderUserref: row.OrderUserref,
			ExecID:       row.ExecID,
			ExecType:     row.ExecType,
			TradeID:      row.TradeID,
			Symbol:       row.Symbol,
			Side:         row.Side,
			LastQty:      copyDecimal(row.LastQty),
			LastPrice:    copyDecimal(row.LastPrice),
			LiquidityInd: row.LiquidityInd,
			Cost:         copyDecimal(row.Cost),
			OrderType:    row.OrderType,
			Timestamp:    row.Timestamp,
			OrderStatus:  row.OrderStatus,
			CumQty:       copyDecimal(row.CumQty),
			CumCost:      copyDecimal(row.CumCost),
			AvgPrice:     copyDecimal(row.AvgPrice),
			FeeUsdEquiv:  copyDecimal(row.FeeUsdEquiv),
			Fees:         append([]kraken.ExecutionFee(nil), row.Fees...),
		}
	}

	return buffered
}

func (position *Position) copyHolding() *types.Holding {
	holding := position.Holding

	if holding == nil {
		return nil
	}

	copy := &types.Holding{
		Status:        holding.Status,
		Symbol:        holding.Symbol,
		Asset:         holding.Asset,
		Qty:           copyDecimal(holding.Qty),
		SellableQty:   copyDecimal(holding.SellableQty),
		EntryAt:       copyTime(holding.EntryAt),
		ExitAt:        copyTime(holding.ExitAt),
		EntryPrice:    copyDecimal(holding.EntryPrice),
		EntryFee:      copyDecimal(holding.EntryFee),
		ExitPrice:     copyDecimal(holding.ExitPrice),
		ExitFee:       copyDecimal(holding.ExitFee),
		PnL:           copyDecimal(holding.PnL),
		Mark:          copyDecimal(holding.Mark),
		IsOpportunity: holding.IsOpportunity,
		ReservationID: holding.ReservationID,
		Stoploss:      copyStoploss(holding.Stoploss),
	}

	if holding.ReturnPct != nil {
		returnPct := *holding.ReturnPct
		copy.ReturnPct = &returnPct
	}

	return copy
}

func copyStoploss(stoploss *types.Stoploss) *types.Stoploss {
	if stoploss == nil {
		return nil
	}

	return &types.Stoploss{
		Status: stoploss.Status,
		Symbol: stoploss.Symbol,
		Entry:  copyDecimal(stoploss.Entry),
		Peak:   copyDecimal(stoploss.Peak),
		Mark:   copyDecimal(stoploss.Mark),
		Floor:  copyDecimal(stoploss.Floor),
	}
}

func copyDecimal(value *decimal.Decimal) *decimal.Decimal {
	if value == nil {
		return nil
	}

	return value.Copy()
}

func copyTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	copy := *value
	return &copy
}

/*
onOrder binds the venue order identifier to this position after the broker has
already correlated the request id. The position stays pending until execution
frames prove whether the market order opened or closed the lot.
*/
func (position *Position) onOrder(message any) any {
	var response *kraken.OrderResponse

	switch v := message.(type) {
	case *kraken.OrderResponse:
		response = v
	case []byte:
		response = kraken.NewOrderResponse(v)
	default:
		return nil
	}

	row := response.Result

	if response.ReqID != position.EntryOrder.ReqID &&
		response.ReqID != position.ExitOrder.ReqID {
		return nil
	}

	if !response.IsSuccess() {
		position.Status = types.REJECTED
		position.Holding.Status = types.REJECTED
		position.closing = false
		position.Publish()
		return nil
	}

	position.OrderID = row.OrderID
	position.Status = types.PENDING
	position.closing = response.ReqID == position.ExitOrder.ReqID

	buffered := position.Buffered
	position.Buffered = nil

	for _, row := range buffered {
		position.applyExecution(row)
	}

	position.Publish()

	return nil
}

/*
onExecutions applies fills onto the exact holding published to Balance. Live
entries must become the same enriched lot as restart-adopted positions, including
entry economics, mark, PnL, and the bound stoploss floor/peak regulator.
*/
func (position *Position) onExecutions(message any) any {
	var execution *kraken.Execution

	switch v := message.(type) {
	case *kraken.Execution:
		execution = v
	case []byte:
		execution = kraken.NewExecution(v)
	default:
		return nil
	}

	rows := execution.Data

	for _, row := range rows {
		if position.OrderID == "" {
			if row.Symbol != position.Holding.Symbol {
				continue
			}

			position.Buffered = append(position.Buffered, row)
			continue
		}

		position.applyExecution(row)
	}

	return nil
}

func (position *Position) applyExecution(row kraken.ExecutionData) {
	if position.Holding == nil || position.Holding.Symbol == "" ||
		row.Symbol != position.Holding.Symbol {
		return
	}

	if row.OrderID != "" && position.OrderID != "" && row.OrderID != position.OrderID {
		return
	}

	if row.ExecID != "" {
		if _, seen := position.seenExec[row.ExecID]; seen {
			return
		}

		position.seenExec[row.ExecID] = struct{}{}
	}

	row.Side = strings.ToLower(row.Side)
	position.price.RecordFill(
		&position.pair, position.Holding, row, &position.Fills,
	)

	if row.Side == "buy" && row.LastQty != nil {
		position.Holding.Qty = position.filledQty("buy")
		position.ExitOrder.Params.OrderQty = position.Holding.Qty.Copy()
	}

	if row.Side == "sell" && row.LastQty != nil {
		position.Holding.Qty = position.remainingQty()
		position.ExitOrder.Params.OrderQty = position.Holding.Qty.Copy()
	}

	if row.Side == "buy" && row.OrderStatus == "filled" &&
		position.Holding.Stoploss != nil && position.Holding.EntryPrice != nil {
		position.Holding.Stoploss.Update(position.Holding.EntryPrice)
	}

	status, err := types.StatusFromMarket(row.ExecType)

	if err != nil {
		status = types.Status(row.OrderStatus)
	}

	position.Status = status
	position.Holding.Status = status

	if err := position.balance.Refresh(); err != nil {
		errnie.Error(err)
	}

	if row.Side == "sell" {
		switch position.Status {
		case types.CANCELED, types.ERROR, types.EXPIRED, types.REJECTED:
			position.closing = false
		}
	}

	if err := position.balance.Publish(); err != nil {
		errnie.Error(err)
	}

	position.Publish()

	if row.Side == "sell" && position.Holding.Qty != nil &&
		position.Holding.Qty.Sign() <= 0 {
		position.Close()
	}
}

/*
filledQty totals one side of the immutable execution ledger so order requests
never substitute for exchange-confirmed inventory.
*/
func (position *Position) filledQty(side string) *decimal.Decimal {
	quantity := decimal.NewFromInt64(0)

	for _, fill := range position.Fills {
		if fill.Side == side && fill.Qty != nil {
			quantity = quantity.Add(fill.Qty)
		}
	}

	return quantity
}

/*
remainingQty derives sellable inventory from confirmed fills, preserving a
partially filled entry instead of submitting the originally requested size.
*/
func (position *Position) remainingQty() *decimal.Decimal {
	return position.filledQty("buy").Sub(position.filledQty("sell"))
}

/*
onTicker refreshes the mark cache for this position's holding and
lets the bound stoploss regulator evaluate the live bid path for
exit decisions.
*/
func (position *Position) onTicker(message any) any {
	ticker, ok := message.(*kraken.Ticker)

	if !ok {
		ticker = kraken.NewTicker(message.([]byte))
	}

	for _, row := range ticker.Data {
		if row.Symbol != position.pair.Symbol {
			continue
		}

		if err := position.price.Mark(&position.pair, position.Holding); err != nil {
			errnie.Error(err)
		}

		if position.Holding != nil && position.Holding.Mark != nil &&
			position.Holding.Mark.Sign() > 0 && position.Holding.Stoploss != nil {
			position.Holding.Stoploss.Update(position.Holding.Mark)
		}

		break
	}

	return nil
}

/*
Enter submits a market buy for its quantity and returns the transport error so
Desk cannot publish a false entry-submitted lifecycle.
*/
func (position *Position) Enter() (*Position, error) {
	if err := position.api.AddOrder(position.EntryOrder); err != nil {
		position.Status = types.ERROR
		position.Holding.Status = types.ERROR

		return position, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	return position, nil
}

/*
Exit submits a market sell for the sellable ledger quantity.
*/
func (position *Position) Exit() error {
	if position.closing || position.Status == types.CLOSED {
		return nil
	}

	if position.Holding == nil || position.Holding.Qty == nil ||
		position.Holding.Qty.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position: no sellable filled inventory",
			nil,
		))
	}

	position.ExitOrder.Params.OrderQty = position.Holding.Qty.Copy()

	if err := position.api.AddOrder(position.ExitOrder); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market sell order",
			err,
		))
	}

	position.closing = true
	position.Status = types.PENDING
	position.Holding.Status = types.PENDING
	position.Publish()

	return nil
}
