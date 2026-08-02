package broker

import (
	"context"
	"errors"
	"iter"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Slot budget used when the trading config supplies none, matching the shipped
configuration so an unconfigured desk still trades the way a configured one
does.
*/
const (
	defaultNormalSlots   = 2
	defaultReservedSlots = 2
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

	/*
		Without a configured budget the desk has no slots at all and refuses
		every entry, so an unset or absent config falls back to a working desk
		rather than a silently closed one.
	*/
	maxPositions := viper.GetInt("trading.slots.normal")

	if maxPositions < 1 {
		maxPositions = defaultNormalSlots
	}

	maxReserved := viper.GetInt("trading.slots.reserved")

	if maxReserved < 0 {
		maxReserved = defaultReservedSlots
	}

	desk := &Desk{
		ctx:    ctx,
		cancel: cancel,
		status: types.READY,
		api:    api,
		subscriptions: map[string]*types.Subscription[any]{
			"ticker": api.Subscribe("ticker", types.NewSubscription[any]()),
		},
		ui:           ui,
		instrument:   instrument,
		price:        price,
		balance:      balance,
		positions:    &sync.Map{},
		maxPositions: maxPositions,
		maxReserved:  maxReserved,
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

/*
Price exposes the desk's price surface so the market data path can keep it
current. Every executable calculation the broker makes reads from it, so it
has to see the same ticks the thesis does.
*/
func (desk *Desk) Price() *Price {
	return desk.price
}

func (desk *Desk) OpenPositions() int {
	count := 0

	desk.positions.Range(func(key, value any) bool {
		position, ok := value.(*Position)

		if !ok || position == nil {
			return true
		}

		position.mu.RLock()
		status := position.Status
		position.mu.RUnlock()

		if status != types.CLOSED {
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

			return yield(position.snapshot())
		})
	}
}

/*
Execute turns an arbitrated decision round into desk-owned order work. Entries
are recorded before submission so the next arbitration round sees committed
capacity even while the venue acknowledgement or fill is still pending.
*/
func (desk *Desk) Execute(decisions []types.Decision) (err error) {
	for _, decision := range decisions {
		switch decision.Action {
		case types.ActionEnter:
			err = errors.Join(err, desk.enter(decision))
		case types.ActionExit:
			err = errors.Join(err, desk.exit(decision))
		}
	}

	return errnie.Error(err)
}

func (desk *Desk) enter(decision types.Decision) error {
	if decision.ID == "" || decision.ProposedQuantity == nil || decision.ProposedQuantity.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"desk: entry decision is missing an identifier or executable quantity",
			nil,
		))
	}

	for position := range desk.Positions() {
		if position.Status != types.CLOSED && position.Holding != nil && position.Holding.Symbol == decision.Symbol {
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"desk: symbol already has an active position",
				nil,
			))
		}
	}

	pair, err := desk.instrument.Pair(decision.Symbol)

	if err != nil {
		return errnie.Error(err)
	}

	position := NewPosition(
		desk.ctx,
		desk.api,
		desk.ui,
		desk.instrument,
		desk.price,
		desk.balance,
		pair,
		decision.ProposedQuantity,
	)
	position.ID = decision.ID
	position.EntryOrder.ClOrdId = decision.ID
	position.Holding.IsOpportunity = decision.Opportunity
	position.Holding.ReservationID = decision.ReservationID
	desk.positions.Store(position.ID, position)

	if _, err := position.Enter(); err != nil {
		desk.positions.Delete(position.ID)

		if releaseErr := desk.balance.Release(decision.ProposedNotional); releaseErr != nil {
			err = errors.Join(err, releaseErr)
		}

		return errnie.Error(err)
	}

	return nil
}

func (desk *Desk) exit(decision types.Decision) error {
	for position := range desk.Positions() {
		if position.Status == types.CLOSED || position.Holding == nil || position.Holding.Symbol != decision.Symbol {
			continue
		}

		stored, ok := desk.positions.Load(position.ID)

		if !ok {
			continue
		}

		return stored.(*Position).Exit()
	}

	return errnie.Error(errnie.Err(
		errnie.NotFound,
		"desk: active position not found for exit",
		nil,
	))
}

/*
MaxPositions is the number of concurrent positions the desk works under normal
conditions, before any reserve is drawn on.
*/
func (desk *Desk) MaxPositions() int {
	return desk.maxPositions
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
