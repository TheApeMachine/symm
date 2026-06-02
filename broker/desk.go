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
		return trading.NewOrder(ctx, pool)
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

func (desk *Desk) Halted() bool {
	if desk.orders == nil {
		return true
	}

	return desk.orders.Halted()
}

func (desk *Desk) AddOrder(action perspectives.Action) error {
	if desk.Halted() {
		return errnie.Error(fmt.Errorf("order circuit breaker tripped"))
	}

	orderType, err := perspectives.OrderTypeFromActionType(action.Type)

	if err != nil {
		return errnie.Error(err)
	}

	clOrdID := desk.NextClOrdID()

	addParams := trading.AddParams{
		OrderType:  orderType,
		Side:       action.Side,
		Symbol:     action.Symbol,
		LimitPrice: action.Price,
		OrderQty:   action.Quantity,
		ClOrdID:    clOrdID,
		Triggers:   &trading.Triggers{},
	}

	if perspectives.IsMakerAction(action.Type) {
		addParams.PostOnly = true
	}

	resultCh, err := desk.orders.AddOrder(addParams)

	if err != nil {
		return errnie.Error(err)
	}

	defer desk.orders.ReleaseOrderResult(resultCh)

	var result trading.OrderResult

	select {
	case result = <-resultCh:
	case <-desk.ctx.Done():
		return errnie.Error(fmt.Errorf("order cancelled: %w", desk.ctx.Err()))
	case <-time.After(trading.AckTimeout()):
		return errnie.Error(fmt.Errorf("order %s ack timeout", clOrdID))
	}

	if !result.Success {
		if result.Error != "" {
			return errnie.Error(fmt.Errorf(
				"order %s rejected: %s",
				result.ClOrdID,
				result.Error,
			))
		}

		return errnie.Error(fmt.Errorf("order %s rejected", result.ClOrdID))
	}

	return nil
}

func (desk *Desk) NextClOrdID() string {
	return fmt.Sprintf("s%016x", uint64(time.Now().UnixNano()))
}

func (desk *Desk) Close() error {
	desk.cancel()

	return desk.err
}
