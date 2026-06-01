package broker

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

type Desk struct {
	ctx    context.Context
	cancel context.CancelFunc
	pool   *qpool.Q
	orders *trading.Client
	err    error
}

func NewDesk(ctx context.Context, pool *qpool.Q) (*Desk, error) {
	ctx, cancel := context.WithCancel(ctx)

	orders := errnie.Does(func() (*trading.Client, error) {
		return trading.NewOrder(ctx)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	desk := &Desk{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		orders: orders,
	}

	return desk, errnie.Error(errnie.Require(map[string]any{
		"ctx":    ctx,
		"cancel": cancel,
		"pool":   pool,
		"orders": orders,
	}))
}

/*
Submit maps one perspective action to a Kraken WebSocket v2 add_order frame.
*/
func (desk *Desk) Submit(action perspectives.Action) error {
	if action.Type == perspectives.ActionNone {
		return nil
	}

	params, err := desk.paramsFromAction(action)

	if err != nil {
		return err
	}

	return errnie.Error(desk.orders.AddOrder(params))
}

func (desk *Desk) paramsFromAction(action perspectives.Action) (trading.AddParams, error) {
	side := action.Side
	orderType := trading.Market
	limitPrice := 0.0

	var triggers *trading.Triggers

	switch action.Type {
	case perspectives.ActionEnter:
		if side == "" {
			side = trading.Buy
		}

		if action.Price > 0 {
			orderType = trading.Limit
			limitPrice = action.Price
		}
	case perspectives.ActionExit, perspectives.ActionShort:
		if side == "" {
			side = trading.Sell
		}

		if action.Price > 0 {
			orderType = trading.Limit
			limitPrice = action.Price
		}
	case perspectives.ActionStopLoss:
		if side == "" {
			side = trading.Sell
		}

		if action.Price <= 0 {
			return trading.AddParams{}, fmt.Errorf(
				"stop loss requires a trigger price for %s", action.Symbol,
			)
		}

		orderType = trading.StopLoss
		triggers = &trading.Triggers{
			Reference: "last",
			Price:     action.Price,
			PriceType: "static",
		}
	case perspectives.ActionTakeProfit:
		if side == "" {
			side = trading.Sell
		}

		if action.Price <= 0 {
			return trading.AddParams{}, fmt.Errorf(
				"take profit requires a trigger price for %s", action.Symbol,
			)
		}

		orderType = trading.TakeProfit
		triggers = &trading.Triggers{
			Reference: "last",
			Price:     action.Price,
			PriceType: "static",
		}
	default:
		return trading.AddParams{}, fmt.Errorf("unsupported action type %d", action.Type)
	}

	return trading.AddParams{
		OrderType:  orderType,
		Side:       side,
		Symbol:     action.Symbol,
		OrderQty:   action.Quantity,
		LimitPrice: limitPrice,
		ClOrdID:    desk.NextClOrdID(),
		Triggers:   triggers,
	}, nil
}

func (desk *Desk) NextClOrdID() string {
	return fmt.Sprintf("s%016x", uint64(time.Now().UnixNano()))
}

func (desk *Desk) Close() error {
	desk.cancel()

	return desk.err
}
