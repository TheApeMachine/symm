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
	status     types.Status
	api        *websocket.API
	ui         chan []byte
	instrument *Instrument
	price      *Price
	balance    *Balance
	pair       kraken.InstrumentPair
	entryOrder *kraken.MarketOrder
	exitOrder  *kraken.MarketOrder
	orderID    string
	intentID   string
	fills      []Fill
	seenExec   map[string]struct{}
	buffered   []kraken.ExecutionData
	holding    *types.Holding
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
		status:     types.INITIALIZING,
		api:        api,
		ui:         ui,
		instrument: instrument,
		price:      price,
		balance:    balance,
		pair:       pair,
		seenExec:   make(map[string]struct{}),
		entryOrder: entryOrder,
		exitOrder:  exitOrder,
		market:     market,
		account:    account,
	}
	mark := entryOrder.Params.LimitPrice

	if mark == nil {
		mark = decimal.NewFromInt64(0)
	}

	position.holding = types.NewHolding(
		ctx,
		pair.Symbol,
		entryOrder.Params.OrderQty,
		mark,
		position.Exit,
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
	position.holding.Initialize(
		position.ctx,
		position.entryOrder.Params.OrderQty,
		position.entryOrder.Params.LimitPrice,
		position.Exit,
		market,
	)

	position.Publish()
}

/*
Status reports the lot lifecycle.
*/
func (position *Position) Status() types.Status {
	return position.status
}

/*
Close marks the lot closed once Desk drops it from the open map.
*/
func (position *Position) Close() (err error) {
	if position.status == types.CLOSED {
		return nil
	}

	errors.Join(err, position.holding.Close())
	position.status = types.CLOSED

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
	select {
	case <-position.ctx.Done():
		return
	case position.ui <- datura.Map[any]{
		"positions": []Position{*position},
	}.Marshal():
	default:
	}
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
	position.orderID = row.OrderID
	position.status = types.PENDING

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
		symbol := position.holding.Symbol

		if symbol == "" {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"position: nil holding for execution symbol "+row.Symbol,
				nil,
			))

			return nil
		}

		if row.ExecID != "" {
			position.seenExec[row.ExecID] = struct{}{}
		}

		row.Side = strings.ToLower(row.Side)
		closed := false

		position.price.RecordFill(
			&position.pair, position.holding, row, &position.fills,
		)

		if row.Side == "buy" && row.OrderStatus == "filled" {
			if position.holding.Stoploss != nil {
				position.holding.EntryPrice = row.Cost
				position.holding.Stoploss.Update(row.Cost)
			}
		}

		position.status = types.Status(row.OrderStatus)
		position.holding.Status = types.Status(row.OrderStatus)

		if err := position.balance.Publish(); err != nil {
			errnie.Error(err)
		}

		if closed {
			position.Close()
		}
	}

	return nil
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

		if err := position.price.Mark(&position.pair, position.holding); err != nil {
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
	if err := position.api.AddOrder(position.entryOrder); err != nil {
		position.balance.DeleteHolding(position.holding.Symbol)
		position.status = types.ERROR

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
	if err := position.api.AddOrder(position.exitOrder); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market sell order",
			err,
		))
	}

	return position.Close()
}
