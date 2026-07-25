package broker

import (
	"context"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Desk is the broker Actor entrypoint. It alone Subscribes market and account
topics and owns plain position maps plus decode-once account routing indexes.
*/
type Desk struct {
	*types.Actor
	status        types.Status
	api           *websocket.API
	instrument    *Instrument
	price         *Price
	balance       *Balance
	positions     map[string]*Position
	byReqID       map[int64]*Position
	byOrderID     map[string]*Position
	fillsBySymbol map[string][]Fill
	historyReady  bool
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
	trading config.TradingConfig,
) *Desk {
	desk := &Desk{
		status:       types.INITIALIZING,
		api:          api,
		instrument:   instrument,
		price:        price,
		balance:      balance,
		positions:    make(map[string]*Position),
		byReqID:      make(map[int64]*Position),
		byOrderID:    make(map[string]*Position),
		maxPositions: trading.SlotsNormal,
		maxReserved:  trading.SlotsReserved,
	}

	desk.Actor = types.NewActor(ctx, map[string]types.Handler{
		"ticker":     {Topic: "ticker", Fn: desk.onTicker},
		"instrument": {Topic: "instrument", Fn: desk.onInstrument},
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

func (desk *Desk) onTicker(message any) any {
	ticker := message.(*kraken.Ticker)

	if errnie.Error(kraken.Validate(ticker)) != nil {
		return nil
	}

	desk.price.TickerAck(ticker)

	marked := false

	for index := range ticker.Data {
		item := ticker.Data[index]
		position, ok := desk.positions[item.Symbol]

		if !ok {
			continue
		}

		position.Mark(item.Symbol)
		marked = true
	}

	if marked {
		if err := desk.balance.Publish(); err != nil {
			errnie.Error(err)
		}
	}

	return nil
}

/*
onInstrument forwards the instrument tick then adopts newly visible open lots.
*/
func (desk *Desk) onInstrument(message any) any {
	desk.instrument.On(message)
	desk.AdoptOpen()

	return nil
}

func (desk *Desk) onBalances(message any) any {
	desk.balance.BalanceAck(message.([]byte))
	desk.AdoptOpen()

	return nil
}

/*
AdoptOpen creates Position shells for wallet lots that already exist at startup
or arrive via balance sync, seeds entry economics from trade history, and marks
so the UI never shows Entry 0 / NaN% for a venue-backed lot.
*/
func (desk *Desk) AdoptOpen() {
	if desk.balance == nil || desk.instrument == nil {
		return
	}

	desk.loadFillHistory()
	published := false

	for holding := range desk.balance.Holdings() {
		symbol := holding.Symbol

		if _, exists := desk.positions[symbol]; !exists {
			pair, err := desk.instrument.Pair(symbol)

			if err != nil {
				continue
			}

			position := NewPosition(
				desk.api, desk.instrument, desk.price, desk.balance, pair,
			)

			if err := position.setStatus(types.OPEN); err != nil {
				errnie.Error(err)

				continue
			}

			desk.positions[symbol] = position
		}

		if err := desk.balance.Update(symbol, func(lot *types.Holding) error {
			if desk.seedEconomics(lot) {
				published = true
			}

			return nil
		}); err != nil {
			errnie.Error(err)

			continue
		}

		if position, exists := desk.positions[symbol]; exists {
			position.Mark(symbol)
		}
	}

	if published {
		if err := desk.balance.Publish(); err != nil {
			errnie.Error(err)
		}
	}
}

/*
loadFillHistory pulls venue trade history once and indexes fills by symbol so
restarted inventory can recover entry price and fees.
*/
func (desk *Desk) loadFillHistory() {
	if desk.historyReady || desk.api == nil {
		return
	}

	history, err := desk.api.TradesHistory()

	if err != nil {
		errnie.Error(err)

		return
	}

	if history == nil {
		return
	}

	desk.fillsBySymbol = make(map[string][]Fill, len(history.Result.Trades))

	for execID, trade := range history.Result.Trades {
		symbol := desk.api.Name(trade.Pair)

		if symbol == "" {
			continue
		}

		fill := Fill{
			ExecID: execID,
			Side:   strings.ToLower(trade.Type),
			Qty:    trade.Volume,
			Price:  trade.Price,
			Fee:    trade.Fee,
		}

		desk.fillsBySymbol[symbol] = append(desk.fillsBySymbol[symbol], fill)
	}

	desk.historyReady = true
}

/*
seedEconomics derives EntryPrice/EntryFee from indexed fills when the wallet
lot arrived without a live Enter path.
*/
func (desk *Desk) seedEconomics(holding *types.Holding) bool {
	if holding == nil || desk.price == nil {
		return false
	}

	if holding.EntryPrice != nil && holding.EntryPrice.Sign() > 0 &&
		holding.EntryFee != nil {
		return false
	}

	fills := desk.fillsBySymbol[holding.Symbol]

	if len(fills) == 0 {
		return false
	}

	desk.price.deriveEconomics(holding, fills)

	return holding.EntryPrice != nil && holding.EntryPrice.Sign() > 0
}

/*
Update removes closed or errored positions from the open map.
*/
func (desk *Desk) Update() {
	for symbol, position := range desk.positions {
		if slices.Contains([]types.Status{
			types.CLOSED, types.ERROR, types.CANCELED,
		}, position.Status()) {
			position.Close()
			desk.forget(symbol, position)

			continue
		}

		if desk.balance == nil {
			continue
		}

		holding, err := desk.balance.Holding(position.Symbol())

		if err != nil || holding.Status != types.CLOSED {
			continue
		}

		if closeErr := position.setStatus(types.CLOSED); closeErr != nil {
			errnie.Error(closeErr)
		}

		desk.forget(symbol, position)
	}
}

func (desk *Desk) forget(symbol string, position *Position) {
	delete(desk.positions, symbol)

	if position.request != nil {
		delete(desk.byReqID, position.request.ReqID)
	}

	if position.orderID != "" {
		delete(desk.byOrderID, position.orderID)
	}
}

/*
OpenPositions counts open inventory plus pending slot reservations.
*/
func (desk *Desk) OpenPositions() int {
	desk.Update()

	reserved := 0

	if desk.balance != nil {
		reserved = desk.balance.ReservedSlots()
	}

	return desk.HoldingCount() + reserved
}

/*
HoldingCount returns the number of wallet lots without Update work.
*/
func (desk *Desk) HoldingCount() int {
	if desk.balance == nil {
		return 0
	}

	count := 0

	for range desk.balance.Holdings() {
		count++
	}

	return count
}

/*
Position returns the open lot shell for symbol.
*/
func (desk *Desk) Position(symbol string) (*Position, bool) {
	position, ok := desk.positions[symbol]

	return position, ok
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
