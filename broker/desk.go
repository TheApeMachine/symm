package broker

import (
	"context"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Desk is the broker Actor entrypoint. It alone Subscribes market and account
topics and owns plain position maps plus decode-once account routing indexes.
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
		byReqID:      make(map[int64]*Position),
		byOrderID:    make(map[string]*Position),
		maxPositions: trading.SlotsNormal,
		maxReserved:  trading.SlotsReserved,
	}

	desk.Actor = types.NewActor(ctx, map[string]types.Handler{
		"instrument": {Topic: "instrument", Fn: desk.onInstrument},
		"balances":   {Topic: "balances", Fn: desk.onBalances},
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

func (desk *Desk) Publish() {
	desk.balance.Publish()
	desk.instrument.Publish()
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

func (desk *Desk) onBalances(message any) any {
	desk.balance.BalanceAck(message.([]byte))
	return nil
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
	holding *types.Holding,
	opportunity bool,
) (*Position, error) {
	return desk.ReserveAndSubmitEntry(holding, opportunity)
}

/*
ReserveAndSubmitEntry claims slot+cash then submits the market enter as one
Desk transition so a failed submit releases both reservations.
*/
func (desk *Desk) ReserveAndSubmitEntry(
	holding *types.Holding,
	opportunity bool,
) (*Position, error) {
	if holding.Qty == nil || holding.Qty.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "desk: enter requires positive quantity", nil,
		))
	}

	if !desk.HasSlot(opportunity) {
		return nil, errnie.Error(errnie.Err(
			errnie.Conflict, "desk: no free slot for "+holding.Symbol, nil,
		))
	}

	pair, err := desk.instrument.Pair(holding.Symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"desk: instrument pair unavailable for "+holding.Symbol,
			err,
		))
	}

	mark, err := desk.price.Last(holding.Symbol)

	if err != nil || mark == nil || mark.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"desk: mark unavailable for reservation on "+holding.Symbol,
			err,
		))
	}

	intentID := holding.Symbol + ":" + holding.Qty.String()
	cash := holding.Qty.Mul(mark)

	if _, exists := desk.balance.byID[intentID]; !exists {
		if err := desk.balance.Reserve(
			intentID, holding.Symbol, cash, true,
		); err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Internal,
				err.Error(),
				err,
			))
		}
	}

	position := NewPosition(
		desk.ctx,
		desk.api,
		desk.instrument,
		desk.price,
		desk.balance,
		pair,
		holding.Qty,
	)

	position.intentID = intentID

	if err := position.Enter(holding); err != nil {
		errnie.Error(desk.balance.Release(intentID))

		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			err.Error(),
			err,
		))
	}

	// Venue now owns the working order; drop the local claim so slot math and
	// Available track exchange state plus any still-open Allocator claims.
	errnie.Error(desk.balance.Commit(intentID))
	position.intentID = ""

	if position.entryOrder != nil {
		desk.byReqID[position.entryOrder.ReqID] = position
	}

	desk.positions[holding.Symbol] = position

	return position, nil
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

	if position.entryOrder != nil {
		desk.byReqID[position.entryOrder.ReqID] = position
	}

	return nil
}
