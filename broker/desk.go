package broker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/rawbus"
)

type Desk struct {
	ctx    context.Context
	cancel context.CancelFunc
	pool   *qpool.Q[any]
	bus    *internal.Bus
	orders *sync.Map
}

func NewDesk(
	ctx context.Context, pool *qpool.Q[any],
) *Desk {
	ctx, cancel := context.WithCancel(ctx)
	bus := internal.NewBus(
		ctx,
		pool,
		[]internal.Channel{internal.ChannelKrakenPrivate, internal.ChannelUI, internal.ChannelAudit},
		[]internal.Subscription{
			internal.Subscribe(internal.ChannelRaw, "desk"),
		},
	)

	return &Desk{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		bus:    bus,
		orders: &sync.Map{},
	}
}

func (desk *Desk) Tick() error {
	for {
		select {
		case <-desk.ctx.Done():
			return desk.ctx.Err()
		default:
		}

		message, err := desk.bus.Receive(internal.ChannelRaw)

		if internal.IsShutdown(err) {
			return err
		}

		if internal.ReportError(err) != nil || message == nil {
			continue
		}

		switch rawbus.TypeFrom(message.Type) {
		case rawbus.TypeOrder, rawbus.TypeActions:
			action, err := rawbus.DecodeAction(message)

			if err != nil {
				errnie.Error(err)
				continue
			}

			if action == nil {
				continue
			}

			clOrdID := uuid.New().String()

			var orderType trading.OrderType

			switch action.Type {
			case logic.ActionMarket:
				orderType = trading.Market
			case logic.ActionLimit:
				orderType = trading.Limit
			case logic.ActionIceberg:
				orderType = trading.Iceberg
			case logic.ActionStopLoss:
				orderType = trading.StopLoss
			case logic.ActionStopLossLimit:
				orderType = trading.StopLossLimit
			case logic.ActionTakeProfit:
				orderType = trading.TakeProfit
			case logic.ActionTakeProfitLimit:
				orderType = trading.TakeProfitLimit
			case logic.ActionTrailingStop:
				orderType = trading.TrailingStop
			case logic.ActionTrailingStopLimit:
				orderType = trading.TrailingStopLimit
			case logic.ActionSettlePosition:
				orderType = trading.SettlePosition
			default:
				errnie.Error(fmt.Errorf("broker: unknown action type %q", action.Type))
				continue
			}

			params := trading.AddParams{
				ClOrdID:    clOrdID,
				Symbol:     action.Symbol,
				Side:       action.Side,
				OrderQty:   action.Quantity,
				LimitPrice: action.Price,
				OrderType:  orderType,
			}

			if !action.Type.IsExit() {
				params.EntryQueuedAt = time.Now().UTC()
			}

			errnie.Error(desk.bus.Send(internal.ChannelKrakenPrivate, "orders", types.KrakenMessage{
				Method: trading.MethodAddOrder,
				Params: params,
				ReqID:  time.Now().UnixNano(),
			}))

			desk.orders.Store(clOrdID, action)
		case rawbus.TypeOrders:
			updates, err := rawbus.DecodeOrderUpdates(message)

			if err != nil {
				errnie.Error(err)
				continue
			}

			for _, update := range updates {
				desk.orders.Delete(update.OrderID)
			}
		case rawbus.TypeExecutions:
			updates, err := rawbus.DecodeExecutions(message)

			if err != nil {
				errnie.Error(err)
				continue
			}

			for _, execution := range updates {
				if execution.ClOrdID != "" {
					desk.orders.Delete(execution.ClOrdID)
				}
			}
		}
	}
}

func (desk *Desk) Close() error {
	desk.cancel()
	return nil
}
