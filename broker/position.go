package broker

import (
	"context"
	"errors"

	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/api-go/v2/pkg/spot"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Position is one lot shell owned and event-routed by Desk. Order correlation uses
each decision's client order ID, then the exchange order ID returned by REST.
*/
type Position struct {
	ctx              context.Context
	cancel           context.CancelFunc
	api              *websocket.API
	ui               chan []byte
	instrument       *Instrument
	price            *Price
	balance          *Balance
	recorder         *audit.Recorder
	store            *PositionStore
	pair             kraken.InstrumentPair
	seenExecutions   map[string]struct{}
	Status           types.Status          `json:"status"`
	EntryOrder       *spot.AddOrderRequest `json:"entry_order"`
	ExitOrder        *spot.AddOrderRequest `json:"exit_order"`
	EntryOrderResult *spot.AddOrderResult  `json:"entry_order_result"`
	ExitOrderResult  *spot.AddOrderResult  `json:"exit_order_result"`
	Holding          *types.Holding        `json:"holding"`
}

/*
NewPosition constructs one desk-owned lot shell.
*/
func NewPosition(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	recorder *audit.Recorder,
	store *PositionStore,
	pair kraken.InstrumentPair,
	decision types.Decision,
) *Position {
	errnie.Info("creating position for: " + pair.Symbol)
	ctx, cancel := context.WithCancel(ctx)

	position := &Position{
		ctx:            ctx,
		cancel:         cancel,
		Status:         types.INITIALIZING,
		api:            api,
		ui:             ui,
		instrument:     instrument,
		price:          price,
		balance:        balance,
		recorder:       recorder,
		store:          store,
		pair:           pair,
		seenExecutions: map[string]struct{}{},
		EntryOrder: &spot.AddOrderRequest{
			ClOrdId:   decision.ID,
			Type:      "buy",
			OrderType: "market",
			Volume:    decision.ProposedQuantity.String(),
			Pair:      pair.Symbol,
		},
		Holding: &types.Holding{
			Symbol:      pair.Symbol,
			Qty:         decimal.NewFromInt64(0),
			SellableQty: decimal.NewFromInt64(0),
			Asset:       pair.Base,
			EntryPrice:  decision.EntryPrice,
			Mark:        decision.Mark,
			Stoploss:    decision.Stoploss,
		},
	}

	return position
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
	out["positions"] = []*Position{position}
	utils.Publish(position.ui, out)
}

/*
onTicker refreshes the mark cache for this position's holding and lets the
bound stoploss regulator judge the price a sale would actually realise.
*/
func (position *Position) onTicker(ticker kraken.TickerData) {
	if position.Holding == nil {
		position.Publish()
		return
	}

	position.price.Update(&ticker)

	if position.Holding.Qty == nil || position.Holding.Qty.Sign() <= 0 {
		if ticker.Bid != nil {
			position.Holding.Mark = ticker.Bid
		}

		position.Publish()
		return
	}

	if position.Holding.Stoploss == nil {
		position.Holding.Mark = ticker.Bid
		position.Holding.PnL = position.price.PnL(position.pair, position.Holding)
		position.Holding.ReturnPct = position.price.ReturnPct(position.pair, position.Holding)
		position.Publish()
		return
	}

	previousStatus := position.Holding.Stoploss.Status
	previousLocked := position.Holding.Stoploss.Locked
	previousFloor := position.Holding.Stoploss.Floor
	previousPeak := position.Holding.Stoploss.Peak
	position.Holding.Update(ticker)

	stoploss := position.Holding.Stoploss
	changed := previousStatus != stoploss.Status ||
		previousLocked != stoploss.Locked ||
		previousFloor.Cmp(stoploss.Floor) != 0 ||
		previousPeak.Cmp(stoploss.Peak) != 0

	if changed && position.store != nil {
		if err := position.store.Save(stoploss); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"position: failed to persist stoploss transition",
				err,
			))
		}
	}

	if stoploss.Status == types.TRIGGERED && position.ExitOrder == nil {
		position.Exit()
	}

	position.Publish()
}

func (position *Position) onExecution(message kraken.Execution) bool {
	for _, execution := range message.Data {
		if position.ExitOrder != nil &&
			execution.ClientOrderID == position.ExitOrder.ClOrdId &&
			execution.OrderStatus == "filled" {
			if position.store != nil {
				errnie.Error(position.store.Delete(position.pair.Symbol))
			}

			errnie.Error(position.Close())
			return true
		}

		if position.EntryOrder == nil ||
			execution.ClientOrderID != position.EntryOrder.ClOrdId ||
			execution.CumQty == nil || execution.CumQty.Sign() <= 0 ||
			execution.CumCost == nil || execution.CumCost.Sign() <= 0 ||
			execution.FeeUsdEquiv == nil || execution.FeeUsdEquiv.Sign() < 0 {
			continue
		}

		position.Status = types.MarketStatuses[execution.OrderStatus]
		position.Holding.Status = position.Status
		position.Holding.EntryAt = &execution.Timestamp
		position.Holding.EntryPrice = execution.CumCost.Div(execution.CumQty)
		position.Holding.EntryFee = execution.FeeUsdEquiv
		position.Holding.Qty = execution.CumQty
		position.Holding.SellableQty = execution.CumQty

		if err := position.Holding.Stoploss.RebindFill(
			position.Holding.EntryPrice,
		); err != nil {
			position.Status = types.ERROR
			position.Holding.Status = types.ERROR
			position.Holding.Stoploss.Status = types.ERROR
			errnie.Error(err)
			continue
		}

		position.Holding.Stoploss.Update(position.Holding.Mark)
		position.Holding.PnL = position.price.PnL(position.pair, position.Holding)
		position.Holding.ReturnPct = position.price.ReturnPct(position.pair, position.Holding)

		if position.store != nil {
			errnie.Error(position.store.Save(position.Holding.Stoploss))
		}
	}

	return false
}

/*
Enter submits a market buy for its quantity and returns the transport error so
Desk cannot publish a false entry-submitted lifecycle.
*/
func (position *Position) Enter() (*Position, error) {
	result, err := position.api.AddOrder(position.EntryOrder)

	if err != nil {
		position.Status = types.ERROR
		position.Holding.Status = types.ERROR
		position.Holding.Stoploss.Status = types.ERROR

		return position, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	position.EntryOrderResult = &result

	if position.Status != types.OPEN && position.Status != types.CLOSED {
		position.Status = types.PENDING
		position.Holding.Status = types.PENDING
	}

	position.Publish()
	return position, nil
}

func (position *Position) Exit() (*Position, error) {
	position.ExitOrder = &spot.AddOrderRequest{
		ClOrdId:   position.EntryOrder.ClOrdId + "-exit",
		Type:      "sell",
		OrderType: "market",
		Volume:    position.Holding.Qty.String(),
		Pair:      position.pair.Symbol,
	}

	result, err := position.api.AddOrder(position.ExitOrder)

	if err != nil {
		position.Status = types.ERROR
		position.Holding.Status = types.ERROR
		position.Holding.Stoploss.Status = types.ERROR

		return position, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market exit order",
			err,
		))
	}

	position.ExitOrderResult = &result
	position.Status = types.PENDING
	position.Holding.Status = types.PENDING

	position.Publish()
	return position, nil
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
