package broker

import (
	"context"
	"slices"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Desk is the broker Actor entrypoint. It alone Subscribes market and account
topics and fans typed frames into Price, Balance, Instrument, and Positions.
*/
type Desk struct {
	*types.Actor
	status       types.Status
	api          *websocket.API
	instrument   *Instrument
	price        *Price
	balance      *Balance
	positions    *sync.Map
	maxPositions int
	maxReserved  int
}

func NewDesk(
	ctx context.Context,
	api *websocket.API,
	instrument *Instrument,
	price *Price,
	balance *Balance,
) *Desk {
	desk := &Desk{
		status:       types.INITIALIZING,
		api:          api,
		instrument:   instrument,
		price:        price,
		balance:      balance,
		positions:    &sync.Map{},
		maxPositions: viper.GetViper().GetInt("trading.slots.normal"),
		maxReserved:  viper.GetViper().GetInt("trading.slots.reserved"),
	}

	desk.Actor = types.NewActor(ctx, map[string]types.Handler{
		"ticker":     {Topic: "ticker", Fn: desk.onTicker},
		"instrument": {Topic: "instrument", Fn: desk.instrument.On},
		"balances":   {Topic: "balances", Fn: desk.onBalances},
		"executions": {Topic: "executions", Fn: desk.onExecutions},
		"add_order":  {Topic: "add_order", Fn: desk.onOrder},
	})

	return desk
}

/*
Initialize attaches Desk to the market and account Actors, subscribes to
executions, and publishes the initial balance frame.
*/
func (desk *Desk) Initialize(market *types.Actor, account *types.Actor) error {
	errnie.Info("initializing desk")

	topics := []types.Topic{
		{Name: "ticker", Actor: market},
		{Name: "instrument", Actor: market},
	}

	if account != nil {
		topics = append(topics,
			types.Topic{Name: "balances", Actor: account},
			types.Topic{Name: "executions", Actor: account},
			types.Topic{Name: "add_order", Actor: account},
		)
	}

	desk.Actor.Initialize(topics...)

	if desk.api != nil {
		if err := desk.api.SubscribeExecutions(); err != nil {
			desk.status = types.ERROR

			return errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to subscribe to executions",
				err,
			))
		}
	}

	desk.status = types.READY

	if desk.balance != nil {
		desk.balance.Publish()
	}

	return nil
}

func (desk *Desk) Status() types.Status {
	return desk.status
}

func (desk *Desk) onTicker(message any) any {
	ticker := message.(*kraken.Ticker)

	if errnie.Error(kraken.Validate(ticker)) != nil {
		return nil
	}

	desk.price.TickerAck(ticker)

	for index := range ticker.Data {
		item := ticker.Data[index]
		position, ok := desk.Position(item.Symbol)

		if !ok || position == nil {
			continue
		}

		position.Mark(item.Symbol)
	}

	return nil
}

func (desk *Desk) onBalances(message any) any {
	desk.balance.BalanceAck(message.([]byte))
	return nil
}

func (desk *Desk) onExecutions(message any) any {
	raw := message.([]byte)

	desk.positions.Range(func(_, value any) bool {
		value.(*Position).ExecutionAck(raw)
		return true
	})

	return nil
}

func (desk *Desk) onOrder(message any) any {
	raw := message.([]byte)

	desk.positions.Range(func(_, value any) bool {
		value.(*Position).OrderAck(raw)
		return true
	})

	return nil
}

/*
Update checks all positions and removes any that have been
closed or errored.
*/
func (desk *Desk) Update() {
	desk.positions.Range(func(_, value any) bool {
		position := value.(*Position)

		if slices.Contains([]types.Status{
			types.CLOSED, types.ERROR, types.CANCELED,
		}, position.Status()) {
			position.Close()
			desk.positions.Delete(position.pair.Symbol)
			return true
		}

		// Wallet Balance is inventory authority; Position status can lag a
		// syncWallet flatten until the next execution print.
		if desk.balance == nil || position.pair == nil {
			return true
		}

		holding, err := desk.balance.Holding(position.pair.Symbol)

		if err != nil || holding.Status != types.CLOSED {
			return true
		}

		position.status = types.CLOSED
		desk.positions.Delete(position.pair.Symbol)

		return true
	})
}

/*
OpenPositions counts open inventory that occupies a slot. Wallet holdings are
the source of truth — position shells can lag adoptOpen or disappear on ERROR
while paper balances remain. Update+AdoptOpen run so slot math sees fresh shells;
tick publishing should use HoldingCount instead to avoid this work every cut.
*/
func (desk *Desk) OpenPositions() int {
	desk.Update()
	return desk.HoldingCount()
}

/*
HoldingCount returns the number of wallet lots without Update or adopt work so
the tick publish path stays cheap under a hot quote stream.
*/
func (desk *Desk) HoldingCount() int {
	if desk == nil || desk.balance == nil {
		return 0
	}

	count := 0

	for range desk.balance.Holdings() {
		count++
	}

	return count
}

func (desk *Desk) Position(symbol string) (*Position, bool) {
	value, ok := desk.positions.Load(symbol)

	if !ok {
		return nil, false
	}

	return value.(*Position), true
}

func (desk *Desk) Buy(
	holding *types.Holding,
	opportunity bool,
) (*Position, error) {
	pair, err := desk.instrument.Pair(holding.Symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"desk: instrument pair unavailable for "+holding.Symbol,
			err,
		))
	}

	position := NewPosition(
		desk.api, desk.instrument, desk.price, desk.balance, pair,
	)

	if err := position.Enter(holding); err != nil {
		return nil, err
	}

	desk.positions.Store(holding.Symbol, position)

	return position, nil
}

/*
Sell exits the full desk-owned lot for symbol.
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

	return position.Exit()
}

func (desk *Desk) Close() error {
	return nil
}

func (desk *Desk) HasSlot(opportunity bool) bool {
	if !opportunity {
		return desk.OpenPositions() < desk.maxPositions
	}

	return desk.OpenPositions() < desk.MaxSlots(opportunity)
}

func (desk *Desk) MaxSlots(withReserved bool) int {
	if withReserved {
		return desk.maxPositions + desk.maxReserved
	}

	return desk.maxPositions
}
