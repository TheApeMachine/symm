package broker

import (
	"context"
	"slices"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
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

	desk.Actor = types.NewActor(ctx, "desk", map[string]types.Handler{
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

/*
onInstrument forwards the instrument tick then adopts newly visible open lots.
*/
func (desk *Desk) onInstrument(message any) any {
	desk.instrument.On(message)
	return nil
}

/*
onTicker refreshes the price cache, lets each open position mark itself, and
lets the bound stoploss regulate off the same live bid path before a wallet
publish goes out.
*/
func (desk *Desk) onTicker(message any) any {
	ticker, ok := message.(*kraken.Ticker)

	if !ok {
		ticker = kraken.NewTicker(message.([]byte))
	}

	desk.price.TickerAck(ticker)

	for _, row := range ticker.Data {
		position, exists := desk.positions[row.Symbol]

		if !exists || position == nil || position.holding == nil {
			continue
		}

		if err := desk.price.Mark(&position.pair, position.holding); err != nil {
			errnie.Error(err)
		}

		if position.holding.Stoploss != nil {
			position.holding.Stoploss.Update(position.holding.Mark)
		}
	}

	if err := desk.balance.Publish(); err != nil {
		errnie.Error(err)
	}

	return nil
}

/*
onBalances applies the wallet frame, indexes fill history once, and adopts any
wallet-backed lots so restart positions are immediately managed by Desk.
*/
func (desk *Desk) onBalances(message any) any {
	desk.balance.BalanceAck(message.([]byte))
	desk.loadFillHistory()

	seen := make(map[string]struct{})

	for lot := range desk.balance.Lots() {
		if lot.Status != types.OPEN || lot.Qty == nil || lot.Qty.Sign() <= 0 {
			continue
		}

		pair, err := desk.instrument.Pair(lot.Symbol)

		if err != nil {
			errnie.Error(err)
			continue
		}

		seen[lot.Symbol] = struct{}{}

		if fills := desk.fillsBySymbol[lot.Symbol]; len(fills) > 0 {
			entry := fills[len(fills)-1]

			desk.positions[lot.Symbol] = NewPosition(
				desk.ctx,
				desk.api,
				desk.balance.ui,
				desk.instrument,
				desk.price,
				desk.balance,
				pair,
				entry.Qty,
			)
		}
	}

	if err := desk.balance.Publish(); err != nil {
		errnie.Error(err)
	}

	return nil
}

/*
onOrder correlates a venue acknowledgement to the position that submitted the
request. The order id is needed for later execution frames, which may arrive
after the ack and only carry venue identity.
*/
func (desk *Desk) onOrder(message any) any {
	order, ok := message.(*kraken.OrderResponse)

	if !ok {
		order = kraken.NewOrderResponse(message.([]byte))
	}

	position := desk.byReqID[order.ReqID]

	if position == nil {
		return nil
	}

	position.onOrder(order)
	desk.byOrderID[order.Result.OrderID] = position
	return nil
}

/*
onExecutions routes private fill frames to their owning position by venue order
id, then lets Balance publish the enriched holding that powers live cards and
wallet equity.
*/
func (desk *Desk) onExecutions(message any) any {
	execution, ok := message.(*kraken.Execution)

	if !ok {
		execution = kraken.NewExecution(message.([]byte))
	}

	for _, row := range execution.Data {
		position := desk.byOrderID[row.OrderID]

		if position == nil {
			continue
		}

		position.onExecutions(&kraken.Execution{
			Channel:  execution.Channel,
			Type:     execution.Type,
			Data:     []kraken.ExecutionData{row},
			Sequence: execution.Sequence,
		})
	}

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

	if position.entryOrder != nil {
		desk.byReqID[position.entryOrder.ReqID] = position
	}

	return nil
}
