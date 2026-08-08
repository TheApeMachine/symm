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
	store         *PositionStore
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
	store *PositionStore,
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
		store:        store,
		positions:    &sync.Map{},
		maxPositions: viper.GetViper().GetInt("trading.slots.normal"),
		maxReserved:  viper.GetViper().GetInt("trading.slots.reserved"),
	}

	desk.recovery = NewRecovery(
		ctx, api, ui, instrument, price, balance, recorder, store, desk.positions,
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

				/*
					Cash and equity are refreshed together because they answer the
					same question at the same instant: what the account holds, and
					what it would settle at if every open lot were closed now. The
					in-flight guard already paces both against the ticker rate, so
					the account readout stays live without adding venue traffic of
					its own.
				*/
				if balanceRefreshing.CompareAndSwap(false, true) {
					go func() {
						defer balanceRefreshing.Store(false)
						desk.balance.Update()
						desk.PublishEquity()
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
						if position.onExecution(*execution) {
							desk.positions.CompareAndDelete(key, position)
						}
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
			decision.Stoploss == nil || decision.Forecast == nil || desk.price == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"desk: quantity, forecast, price, and strategy stoploss required for entry",
				nil,
			))
		}

		if err := decision.Forecast.Validate(); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"desk: valid forecast required for entry",
				err,
			))
		}

		desk.positionsMu.Lock()
		found, loaded := desk.positions.Load(decision.Symbol)

		if loaded {
			position, valid := found.(*Position)

			if valid && position != nil && position.Status != types.CLOSED {
				desk.positionsMu.Unlock()
				return nil
			}
		}

		if desk.OpenSlots(decision.Opportunity) <= 0 {
			desk.positionsMu.Unlock()
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"desk: position capacity exhausted for requested allocation",
				nil,
			))
		}

		economics, err := desk.price.EntryEconomics(
			decision.Symbol,
			decision.ProposedQuantity,
			decision.Forecast.ExpectedReturn,
		)

		if err != nil {
			desk.positionsMu.Unlock()
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"desk: entry is no longer executable",
				err,
			))
		}

		if economics.NetReturn.Sign() <= 0 {
			desk.positionsMu.Unlock()
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"desk: forecast no longer clears current spread and taker fees",
				nil,
			))
		}

		decision.ExpectedReturn = economics.ExpectedReturn
		decision.ExpectedFees = economics.ExpectedFees
		decision.ExpectedSpread = economics.ExpectedSpread
		decision.ExpectedImpact = economics.ExpectedImpact

		position := NewPosition(
			desk.ctx,
			desk.api,
			desk.ui,
			desk.instrument,
			desk.price,
			desk.balance,
			desk.recorder,
			desk.store,
			desk.instrument.Pair(decision.Symbol),
			decision,
		)
		desk.positions.Store(decision.Symbol, position)
		desk.positionsMu.Unlock()

		_, err = position.Enter()

		if err != nil {
			desk.positions.CompareAndDelete(decision.Symbol, position)
			return err
		}
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

/*
OpenSlots reports normal capacity unless the decision has independently
qualified for the reserve lane. It never reports historical over-capacity as
new capacity.
*/
func (desk *Desk) OpenSlots(opportunity bool) int {
	occupied := desk.occupiedPositions()

	if opportunity {
		return max(0, (desk.maxPositions+desk.maxReserved)-occupied)
	}

	return max(0, desk.maxPositions-occupied)
}

/*
occupiedPositions counts committed desk entries without reading mutable order
status. Entries are stored before submission and removed after failure or close,
so map membership is the race-free risk-slot boundary.
*/
func (desk *Desk) occupiedPositions() int {
	occupied := 0
	desk.positions.Range(func(_, value any) bool {
		position, valid := value.(*Position)

		if valid && position != nil {
			occupied++
		}

		return true
	})

	return occupied
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
