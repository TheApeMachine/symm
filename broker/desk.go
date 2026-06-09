package broker

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/logic"
)

type Desk struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	pool   *qpool.Q[any]
	bus    *internal.Bus
}

func NewDesk(ctx context.Context, pool *qpool.Q[any]) *Desk {
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
	}
}

func (desk *Desk) AddOrder(action *logic.Action) error {
	frame, err := types.NewKrakenMessage(
		trading.MethodAddOrder,
		trading.AddParams{
			OrderType:  trading.OrderType(action.Type),
			Side:       trading.Side(action.Side),
			Symbol:     action.Symbol,
			LimitPrice: action.Price,
			OrderQty:   action.Quantity,
			ClOrdID:    uuid.New().String(),
		},
		time.Now().UnixNano(),
	)

	if errnie.Error(err) != nil {
		return err
	}

	return errnie.Error(desk.bus.Send("kraken:private", "orders", frame))
}

func (desk *Desk) Close() error {
	desk.cancel()
	return desk.err
}
