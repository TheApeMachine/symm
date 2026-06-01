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

func (desk *Desk) AddOrder(action perspectives.Action) error {
	return errnie.Error(desk.orders.AddOrder(trading.AddParams{
		OrderType:  trading.Limit,
		Side:       action.Side,
		Symbol:     action.Symbol,
		LimitPrice: action.Price,
		OrderQty:   action.Quantity,
	}))
}

func (desk *Desk) NextClOrdID() string {
	return fmt.Sprintf("s%016x", uint64(time.Now().UnixNano()))
}

func (desk *Desk) Close() error {
	desk.cancel()

	return desk.err
}
