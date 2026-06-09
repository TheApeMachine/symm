package broker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
)

type Desk struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	pool   *qpool.Q[any]
	bus    *internal.Bus
	orders *sync.Map
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
			[]string{"raw"},
		),
		orders: &sync.Map{},
	}
}

func (desk *Desk) Tick() error {
	for {
		message, err := desk.bus.Receive("raw")

		if errnie.Error(err) != nil || message == nil {
			continue
		}

		switch message.Type {
		case "order":
			action, ok := message.Value.(*logic.Action)

			if !ok {
				errnie.Error(errors.New("desk: invalid order action"))
				continue
			}

			if errnie.Error(desk.addOrder(action)) != nil {
				continue
			}
		case "orders":
			updates, ok := message.Value.([]trading.OrderUpdate)

			if !ok {
				errnie.Error(errors.New("desk: invalid orders"))
				continue
			}

			for _, update := range updates {
				desk.orders.Delete(update.OrderID)
			}
		case "executions":
			updates, ok := message.Value.([]user.Execution)

			if !ok {
				errnie.Error(errors.New("desk: invalid executions"))
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

func (desk *Desk) addOrder(action *logic.Action) error {
	token, err := types.NewToken(desk.ctx)

	if err != nil {
		return errnie.Error(err)
	}

	clOrdID := uuid.New().String()
	frame := types.KrakenMessage{
		Method: trading.MethodAddOrder,
		Params: &trading.AddParams{
			OrderType:  trading.OrderType(action.Type),
			Side:       trading.Side(action.Side),
			Symbol:     action.Symbol,
			LimitPrice: action.Price,
			OrderQty:   action.Quantity,
			ClOrdID:    clOrdID,
			Token:      token,
		},
		ReqID: time.Now().UnixNano(),
	}

	desk.orders.Store(clOrdID, frame)

	return errnie.Error(desk.bus.Send("kraken:private", "orders", frame))
}

func (desk *Desk) CheckOrder(orderID string) error {
	frame, ok := desk.orders.Load(orderID)

	if !ok {
		return errnie.Error(fmt.Errorf("desk: order not found: %s", orderID))
	}

	return errnie.Error(desk.bus.Send("kraken:private", "orders", frame))
}

func (desk *Desk) Close() error {
	desk.cancel()
	return desk.err
}
