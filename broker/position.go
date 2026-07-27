package broker

import (
	"context"
	"errors"
	"strings"

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

	errors.Join(err, position.Holding.Close())
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
		"positions", []Position{*position},
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

	if !response.IsSuccess() || (response.ReqID != position.EntryOrder.ReqID &&
		response.ReqID != position.ExitOrder.ReqID) {
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
	closed := row.Side == "sell" && row.OrderStatus == "filled"

	position.price.RecordFill(
		&position.pair, position.Holding, row, &position.Fills,
	)

	if row.Side == "buy" && row.OrderStatus == "filled" &&
		position.Holding.Stoploss != nil && position.Holding.EntryPrice != nil {
		position.Holding.Stoploss.Update(position.Holding.EntryPrice)
	}

	position.Status = types.Status(row.OrderStatus)
	position.Holding.Status = types.Status(row.OrderStatus)

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

	if closed {
		position.Close()
	}
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

		break
	}

	position.Publish()

	return nil
}

/*
Enter seeds the holding onto Balance and submits a market buy for its quantity.
*/
func (position *Position) Enter() *Position {
	if err := position.api.AddOrder(position.EntryOrder); err != nil {
		position.Status = types.ERROR

		errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	return position
}

/*
Exit submits a market sell for the sellable ledger quantity.
*/
func (position *Position) Exit() error {
	if position.closing || position.Status == types.CLOSED {
		return nil
	}

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
