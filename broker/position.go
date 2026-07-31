package broker

import (
	"context"
	"errors"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Position is one lot shell owned by Desk. Order correlation uses request ID then
exchange order ID; unmatched executions buffer until the ack binds them.
Position subscribes to the market ticker topic for live mark and stoploss
updates, and to the account execution and add_order topics for fills and
order acks so it handles its own lifecycle without Desk absorbing the logic.
*/
type Position struct {
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	subscriptions map[string]*types.Subscription[any]
	ui            chan []byte
	instrument    *Instrument
	price         *Price
	balance       *Balance
	pair          kraken.InstrumentPair
	ID            string                `json:"id"`
	Status        types.Status          `json:"status"`
	EntryOrder    *spot.AddOrderRequest `json:"entry_order"`
	ExitOrder     *spot.AddOrderRequest `json:"exit_order"`
	Holding       *types.Holding        `json:"holding"`
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
	market types.MarketFeed,
	account types.AccountFeed,
) *Position {
	errnie.Info("creating position for: " + pair.Symbol)
	ctx, cancel := context.WithCancel(ctx)

	position := &Position{
		ctx:    ctx,
		cancel: cancel,
		Status: types.INITIALIZING,
		api:    api,
		subscriptions: map[string]*types.Subscription[any]{
			"ticker":     api.Subscribe("ticker", types.NewSubscription[any]()),
			"executions": api.Subscribe("executions", types.NewSubscription[any]()),
			"add_order":  api.Subscribe("add_order", types.NewSubscription[any]()),
		},
		ui:         ui,
		instrument: instrument,
		price:      price,
		balance:    balance,
		pair:       pair,
		EntryOrder: &spot.AddOrderRequest{
			Type:      "buy",
			OrderType: "market",
			Volume:    qty.String(),
			Pair:      pair.Symbol,
			Validate:  true,
		},
		ExitOrder: &spot.AddOrderRequest{
			Type:      "sell",
			OrderType: "market",
			Volume:    qty.String(),
			Pair:      pair.Symbol,
			Validate:  true,
		},
		Holding: types.NewHolding(
			ctx,
			pair.Symbol,
			qty,
			price.Mark(pair.Symbol, SELL),
		),
	}

	position.run()
	position.Publish()

	return position
}

func (position *Position) run() {
	go func() {
		for {
			select {
			case <-position.ctx.Done():
				return
			case message := <-position.subscriptions["ticker"].Channel:
				ticker, ok := message.(*kraken.Ticker)

				if !ok {
					continue
				}

				position.onTicker(ticker)
			}
		}
	}()
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
	out := datura.NewMap()
	out["positions"] = []Position{*position}
	utils.Publish(position.ui, out)
}

/*
onTicker refreshes the mark cache for this position's holding and
lets the bound stoploss regulator evaluate the live bid path for
exit decisions.
*/
func (position *Position) onTicker(ticker *kraken.Ticker) {
	if ticker == nil {
		return
	}

	for _, row := range ticker.Data {
		if row.Symbol != position.pair.Symbol {
			continue
		}

		if position.Holding != nil && position.Holding.Stoploss != nil {
			if err := position.Holding.Stoploss.Update(ticker); err != nil {
				errnie.Error(err)
			}
		}

		break
	}
}

/*
Enter submits a market buy for its quantity and returns the transport error so
Desk cannot publish a false entry-submitted lifecycle.
*/
func (position *Position) Enter() (*Position, error) {
	if _, err := position.api.AddOrder(position.EntryOrder); err != nil {
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
	if _, err := position.api.AddOrder(position.ExitOrder); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market sell order",
			err,
		))
	}

	position.Status = types.PENDING
	position.Holding.Status = types.PENDING
	position.Publish()

	return nil
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

	position.Status = types.CLOSED
	return errnie.Error(err)
}
