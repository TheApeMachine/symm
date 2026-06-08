package broker

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

type Desk struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	pool   *qpool.Q[any]
	bus    *internal.Bus
}

func NewDesk(ctx context.Context, pool *qpool.Q[any]) (*Desk, error) {
	ctx, cancel := context.WithCancel(ctx)

	return &Desk{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		bus: internal.NewBus(
			ctx,
			pool,
			[]string{"kraken:private"},
			[]string{},
		),
	}, nil
}

func (desk *Desk) AddOrder(action logic.Action) error {
	order := trading.AddParams{
		OrderType:  trading.Limit,
		Side:       trading.Side(action.Side),
		Symbol:     action.Symbol,
		LimitPrice: action.Price,
		OrderQty:   action.Quantity,
		ClOrdID:    uuid.New().String(),
	}

	switch action.Type {
	case logic.ActionLimit:
		order.OrderType = trading.Limit
	case logic.ActionMarket:
		order.OrderType = trading.Market
	case logic.ActionIceberg:
		order.OrderType = trading.Iceberg
	case logic.ActionStopLoss:
		order.OrderType = trading.StopLoss
	case logic.ActionStopLossLimit:
		order.OrderType = trading.StopLossLimit
	case logic.ActionTakeProfit:
		order.OrderType = trading.TakeProfit
	case logic.ActionTakeProfitLimit:
		order.OrderType = trading.TakeProfitLimit
	case logic.ActionTrailingStop:
		order.OrderType = trading.TrailingStop
	case logic.ActionTrailingStopLimit:
		order.OrderType = trading.TrailingStopLimit
	case logic.ActionSettlePosition:
		order.OrderType = trading.Market
	}

	return errnie.Error(
		desk.bus.Send("kraken:private", "orders", order),
	)
}

func (desk *Desk) Close() error {
	desk.cancel()
	return desk.err
}
