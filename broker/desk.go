package broker

import (
	"context"
	"slices"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
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
	*types.Actor
	ctx           context.Context
	cancel        context.CancelFunc
	status        types.Status
	api           *websocket.API
	instrument    *Instrument
	price         *Price
	balance       *Balance
	positions     map[string]*Position
	maxPositions  int
	maxReserved   int
	market        *types.Actor
	account       *types.Actor
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
	trading config.TradingConfig,
) *Desk {
	ctx, cancel := context.WithCancel(ctx)

	desk := &Desk{
		ctx:          ctx,
		cancel:       cancel,
		status:       types.INITIALIZING,
		api:          api,
		instrument:   instrument,
		price:        price,
		balance:      balance,
		positions:    make(map[string]*Position),
		maxPositions: trading.SlotsNormal,
		maxReserved:  trading.SlotsReserved,
	}

	desk.Actor = types.NewActor(ctx, "desk", map[string]types.Handler{
		"ticker":     {Topic: "ticker", Fn: desk.onTicker},
		"instrument": {Topic: "instrument", Fn: desk.onInstrument},
		"balances":   {Topic: "balances", Fn: desk.onBalances},
	})

	return desk
}

/*
Initialize attaches Desk to the market and account Actors, subscribes
to shared topics, and publishes the initial balance frame. Position
subscribes to market ticker and account executions/add_order topics
directly so it handles its own lifecycle without Desk absorbing it.
*/
func (desk *Desk) Initialize(market *types.Actor, account *types.Actor) error {
	errnie.Info("initializing desk")

	desk.market = market
	desk.account = account

	topics := []types.Topic{
		{Name: "ticker", Actor: market},
		{Name: "instrument", Actor: market},
	}

	if account != nil {
		topics = append(topics,
			types.Topic{Name: "balances", Actor: account},
		)
	}

	desk.Actor.Initialize(topics...)

	if desk.api != nil {
		if err := desk.api.SubscribeExecutions(); err != nil {
			_ = desk.transitionStatus(types.ERROR)

			return errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to subscribe to executions",
				err,
			))
		}
	}

	if err := desk.transitionStatus(types.READY); err != nil {
		return err
	}

	if desk.balance != nil {
		if err := desk.balance.Publish(); err != nil {
			errnie.Error(err)
		}
	}

	return nil
}

/*
transitionStatus applies one canonical desk lifecycle edge and fails loud on
illegal transitions.
*/
func (desk *Desk) transitionStatus(next types.Status) error {
	status, err := types.Transition(desk.status, next)

	if err != nil {
		return errnie.Error(err)
	}

	desk.status = status
	return nil
}

/*
Status reports desk readiness.
*/
func (desk *Desk) Status() types.Status {
	return desk.status
}

/*
onInstrument forwards the instrument tick then adopts newly visible open lots.
*/
func (desk *Desk) onInstrument(message any) any {
	desk.instrument.On(message)
	return nil
}

/*
onTicker refreshes the price cache and publishes the balance
frame. Position subscribes to the ticker topic directly so it
handles its own mark and stoploss updates without Desk
absorbing that routing logic.
*/
func (desk *Desk) onTicker(message any) any {
	ticker, ok := message.(*kraken.Ticker)

	if !ok {
		ticker = kraken.NewTicker(message.([]byte))
	}

	desk.price.TickerAck(ticker)

	if err := desk.balance.Publish(); err != nil {
		errnie.Error(err)
	}

	return nil
}

/*
onBalances applies the wallet frame and adopts any
wallet-backed lots so restart positions are immediately managed by Desk.
*/
func (desk *Desk) onBalances(message any) any {
	desk.balance.BalanceAck(message.([]byte))

	for lot := range desk.balance.Lots() {
		if lot.Status != types.OPEN || lot.Qty == nil || lot.Qty.Sign() <= 0 {
			continue
		}

		pair, err := desk.instrument.Pair(lot.Symbol)

		if err != nil {
			errnie.Error(err)
			continue
		}

		desk.positions[lot.Symbol] = NewPosition(
			desk.ctx,
			desk.api,
			desk.balance.ui,
			desk.instrument,
			desk.price,
			desk.balance,
			pair,
			lot.Qty,
			desk.market,
			desk.account,
		)
	}

	if err := desk.balance.Publish(); err != nil {
		errnie.Error(err)
	}

	return nil
}

/*
Update removes closed or errored positions from the open map.
*/
func (desk *Desk) Update() {
	for symbol, position := range desk.positions {
		if position.holding.Status == types.CLOSED || slices.Contains([]types.Status{
			types.CLOSED, types.ERROR, types.CANCELED,
		}, position.Status()) {
			position.status = types.CLOSED
			position.holding.Status = types.CLOSED
			delete(desk.positions, symbol)
		}
	}
}

/*
OpenPositions counts live wallet lots only.
*/
func (desk *Desk) OpenPositions() int {
	desk.Update()
	return len(desk.positions)
}

/*
Position returns the open lot shell for symbol.
*/
func (desk *Desk) Position(symbol string) (*Position, bool) {
	position, ok := desk.positions[symbol]
	return position, ok
}

/*
Balance returns the composed Balance owner.
*/
func (desk *Desk) Balance() *Balance {
	if desk == nil {
		return nil
	}

	return desk.balance
}

/*
Close is retained for boot shutdown symmetry.
*/
func (desk *Desk) Close() error {
	return nil
}

/*
HasSlot reports whether an additional lot may open under slot capacity.
*/
func (desk *Desk) HasSlot(opportunity bool) bool {
	occupied := desk.OpenPositions()

	if !opportunity {
		return occupied < desk.maxPositions
	}

	return occupied < desk.MaxSlots(opportunity)
}

/*
MaxSlots returns normal capacity, optionally including reserved opportunity slots.
*/
func (desk *Desk) MaxSlots(withReserved bool) int {
	if withReserved {
		return desk.maxPositions + desk.maxReserved
	}

	return desk.maxPositions
}

func (desk *Desk) Buy(
	symbol string,
	qty *decimal.Decimal,
	opportunity bool,
) (*Position, error) {
	pair, err := desk.instrument.Pair(symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"desk: no instrument for "+symbol,
			err,
		))
	}

	desk.positions[symbol] = NewPosition(
		desk.ctx,
		desk.api,
		desk.balance.ui,
		desk.instrument,
		desk.price,
		desk.balance,
		pair,
		qty,
		desk.market,
		desk.account,
	).Enter()

	return desk.positions[symbol], nil
}

/*
Sell exits the full desk-owned sellable lot for symbol.
*/
func (desk *Desk) Sell(symbol string) error {
	position, ok := desk.Position(symbol)

	if !ok {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"desk: no open position for "+symbol,
			nil,
		))
	}

	if err := position.Exit(); err != nil {
		return err
	}

	return nil
}
