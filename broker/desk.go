package broker

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Desk is the broker Actor entrypoint and persistent position owner. It consumes
one market ticker and one account execution subscription, then routes each
message to the positions it owns so closed lots leave no abandoned fan-out
subscriber behind.
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
	recorder      *audit.Recorder
	recovery      *Recovery
	positions     *sync.Map
	positionsMu   sync.Mutex
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
	recorder *audit.Recorder,
	ui chan []byte,
) *Desk {
	ctx, cancel := context.WithCancel(ctx)

	viper.SetDefault("trading.slots.normal", 2)
	viper.SetDefault("trading.slots.reserved", 2)

	desk := &Desk{
		ctx:    ctx,
		cancel: cancel,
		status: types.READY,
		api:    api,
		subscriptions: map[string]*types.Subscription[any]{
			"ticker":     api.Subscribe("ticker", types.NewSubscription[any]()),
			"executions": api.Subscribe("executions", types.NewSubscription[any]()),
		},
		ui:           ui,
		instrument:   instrument,
		price:        price,
		balance:      balance,
		recorder:     recorder,
		positions:    &sync.Map{},
		maxPositions: viper.GetViper().GetInt("trading.slots.normal"),
		maxReserved:  viper.GetViper().GetInt("trading.slots.reserved"),
	}

	desk.recovery = NewRecovery(
		ctx, api, ui, instrument, price, balance, recorder, desk.positions,
	)

	if err := desk.recovery.Recover(); err != nil {
		desk.status = types.ERROR

		errnie.Error(errnie.Err(
			errnie.Internal,
			"desk: failed to recover account positions",
			err,
		))

		return desk
	}

	desk.run()
	return desk
}

func (desk *Desk) run() {
	go func() {
		balanceRefreshing := &atomic.Bool{}

		for {
			select {
			case <-desk.ctx.Done():
				return
			case message := <-desk.subscriptions["ticker"].Channel:
				ticker, ok := message.(*kraken.Ticker)

				if !ok || ticker == nil {
					continue
				}

				for _, tickerData := range ticker.Data {
					found, ok := desk.positions.Load(tickerData.Symbol)

					if ok && found != nil {
						position, ok := found.(*Position)

						if ok && position != nil {
							position.onTicker(tickerData)
						}
					}
				}

				if balanceRefreshing.CompareAndSwap(false, true) {
					go func() {
						defer balanceRefreshing.Store(false)
						desk.balance.Update()
					}()
				}
			case message := <-desk.subscriptions["executions"].Channel:
				execution, ok := message.(*kraken.Execution)

				if !ok || execution == nil {
					continue
				}

				desk.positions.Range(func(key, value any) bool {
					position, ok := value.(*Position)

					if ok && position != nil {
						position.onExecution(*execution)
					}

					return true
				})
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

/*
Balance exposes the desk's balance so callers outside the broker package can
read the current wallet state without accessing the unexported field.
*/
func (desk *Desk) Balance() *Balance {
	return desk.balance
}

/*
Instrument exposes the desk's instrument registry so callers outside the broker
package can reach pair metadata without accessing the unexported field.
*/
func (desk *Desk) Instrument() *Instrument {
	return desk.instrument
}

/*
Recovery exposes the desk's recovery handler.
*/
func (desk *Desk) Recovery() *Recovery {
	return desk.recovery
}

func (desk *Desk) OpenPositions() int {
	count := 0

	for position := range desk.Positions() {
		if position.Status != types.CLOSED {
			count++
		}
	}

	return count
}

/*
Holding reports how many lots the desk still carries for one symbol.

OpenPositions counts slots, which includes a lot whose order is still working.
This counts inventory, so it answers the only question an observer outside the
desk can ask about a symbol without guessing: is anything still on the book for
it, or has the position been run all the way out.
*/
func (desk *Desk) Holding(symbol string) int {
	held := 0

	for position := range desk.Positions() {
		if position.Status == types.CLOSED {
			continue
		}

		held++
	}

	return held
}

/*
PublishEquity reports what the desk is worth if every open lot were closed now.

Cash alone understates the account while positions are open. Unrealized is the
profit/loss only; equity is cash plus the basis committed to open positions plus
that profit/loss.
*/
func (desk *Desk) PublishEquity() {
	tradeBalance, err := desk.api.TradeBalance()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"desk: could not fetch trade balance",
			err,
		))

		return
	}

	utils.Publish(desk.ui, datura.NewMap("equity", datura.NewMap(
		"cash", desk.balance.Cash(),
		"unrealized", tradeBalance.UnrealizedPnL,
		"equity", tradeBalance.Equity,
	)))
}

func (desk *Desk) Positions() iter.Seq[*Position] {
	return func(yield func(*Position) bool) {
		desk.positions.Range(func(key, value any) bool {
			position, ok := value.(*Position)

			if !ok || position == nil {
				return true
			}

			return yield(position)
		})
	}
}

/*
PublishPositions sends all current non-terminal positions to the UI channel
so newly connected clients can see open positions immediately.
*/
func (desk *Desk) PublishPositions() {
	for position := range desk.Positions() {
		position.Publish()
	}
}

/*
Execute turns an arbitrated decision round into desk-owned order work. Entries
are recorded before submission so the next arbitration round sees committed
capacity even while the venue acknowledgement or fill is still pending.
*/
func (desk *Desk) Execute(decision types.Decision) (err error) {
	switch decision.Action {
	case types.ActionEnter:
		if decision.ProposedQuantity == nil || decision.ProposedQuantity.Sign() <= 0 ||
			decision.Stoploss == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"desk: sized quantity and strategy stoploss required for entry",
				nil,
			))
		}

		desk.positions.LoadOrStore(decision.Symbol, NewPosition(
			desk.ctx,
			desk.api,
			desk.ui,
			desk.instrument,
			desk.price,
			desk.balance,
			desk.recorder,
			desk.instrument.Pair(decision.Symbol),
			decision,
		))
	case types.ActionExit:
		err = errnie.Err(
			errnie.NotAcceptable,
			"desk: strategy exits are disabled; only a triggered stoploss may submit a sell",
			nil,
		)
	}

	return errnie.Error(err)
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
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"desk: could not cancel position",
				err,
			))
		}

		return true
	})

	return nil
}
