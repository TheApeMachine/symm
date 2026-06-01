package broker

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
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

func (desk *Desk) AddOrder(
	pair string, side trading.Side, price float64, quantity float64,
) error {
	return errnie.Error(desk.orders.Add(&trading.OrderRequest{
		Pair:      pair,
		Type:      string(side),
		Ordertype: string(trading.Limit),
		Volume:    fmt.Sprintf("%f", quantity),
		Price:     fmt.Sprintf("%f", price),
		ClOrdID:   desk.NextClOrdID(),
	}))
}

func (desk *Desk) NextClOrdID() string {
	return fmt.Sprintf("desk-%d", time.Now().UnixNano())
}

func (desk *Desk) Close() error {
	desk.cancel()
	return desk.err
}
