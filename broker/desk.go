package broker

import (
	"context"
	"iter"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Desk is the broker Actor entrypoint. It subscribes market topics for
price cache updates and account topics not routed to positions.
Position and Stoploss subscribe directly to the market ticker, account
executions, and account add_order topics so they react to live
websocket messages without Desk absorbing the routing logic.
*/
type Desk struct {
	ctx           context.Context
	cancel        context.CancelFunc
	status        types.Status
	api           *websocket.API
	subscriptions map[string]*types.Subscription[any]
	ui            chan []byte
	instrument    *Instrument
	price         *Price
	balance       *Balance
	positions     *sync.Map
	maxPositions  int
	maxReserved   int
}

/*
NewDesk constructs the serial broker owner.
*/
func NewDesk(
	ctx context.Context,
	api *websocket.API,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	ui chan []byte,
) *Desk {
	ctx, cancel := context.WithCancel(ctx)

	desk := &Desk{
		ctx:    ctx,
		cancel: cancel,
		status: types.READY,
		api:    api,
		subscriptions: map[string]*types.Subscription[any]{
			"ticker": api.Subscribe("ticker", types.NewSubscription[any]()),
		},
		ui:         ui,
		instrument: instrument,
		price:      price,
		balance:    balance,
		positions:  &sync.Map{},
	}

	desk.run()
	return desk
}

func (desk *Desk) run() {
	go func() {
		for {
			select {
			case <-desk.ctx.Done():
				return
			case message := <-desk.subscriptions["ticker"].Channel:
				ticker, ok := message.(*kraken.Ticker)

				if !ok || ticker == nil {
					continue
				}
			}
		}
	}()
}

/*
Status reports desk readiness.
*/
func (desk *Desk) Status() types.Status {
	return desk.status
}

func (desk *Desk) OpenPositions() int {
	count := 0

	desk.positions.Range(func(key, value any) bool {
		position, ok := value.(*Position)

		if !ok || position == nil {
			return true
		}

		if position.Status != types.CLOSED {
			count++
		}

		return true
	})

	return count
}

func (desk *Desk) Positions() iter.Seq[Position] {
	return func(yield func(Position) bool) {
		desk.positions.Range(func(key, value any) bool {
			position, ok := value.(*Position)

			if !ok || position == nil {
				return true
			}

			return yield(*position)
		})
	}
}

func (desk *Desk) OpenSlots(opportunity bool) int {
	switch opportunity {
	case true:
		return (desk.maxPositions + desk.maxReserved) - desk.OpenPositions()
	default:
		return desk.maxPositions - desk.OpenPositions()
	}
}

/*
Close is retained for boot shutdown symmetry.
*/
func (desk *Desk) Close() error {
	desk.cancel()
	return nil
}

func (desk *Desk) Cancel() error {
	desk.positions.Range(func(key, value any) bool {
		position, ok := value.(*Position)

		if !ok || position == nil {
			return true
		}

		if err := position.Close(); err != nil {
			errnie.Error(err)
		}

		return true
	})

	return nil
}
