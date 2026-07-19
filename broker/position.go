package broker

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

const (
	intentFlat          = ""
	intentEntryPending  = "entry_pending"
	intentOpen          = "open"
	intentReducePending = "reduce_pending"
	intentExitPending   = "exit_pending"
)

type Position struct {
	status      types.Status
	closed      atomic.Bool
	intent      atomic.Value
	api         *websocket.API
	instrument  *Instrument
	price       *Price
	balance     *Balance
	pair        *kraken.InstrumentPair
	request     *kraken.MarketOrder
	order       *spot.Order
	orderID     string
	claim       Claim
	stoploss    *types.Stoploss
	tickers     []*kraken.TickerData
	executions  []*kraken.Execution
	seenExec    sync.Map
	onTerminal  func(symbol string)
	onOrder     func([]byte)
	onExecution func([]byte)
	subOrder    uint64
	subExec     uint64
}

/*
NewPosition constructs a position manager and registers its channel handlers.
Subscription ids are retained so Unsubscribe can drop them when the lot closes
instead of leaving orphaned On handlers on the shared ticker channel.
*/
func NewPosition(
	api *websocket.API,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	pair *kraken.InstrumentPair,
) *Position {
	position := &Position{
		status:     types.INITIALIZING,
		api:        api,
		instrument: instrument,
		price:      price,
		balance:    balance,
		pair:       pair,
		tickers:    make([]*kraken.TickerData, 0),
		order: &spot.Order{
			Description: &spot.OrderDescription{
				Pair:      pair.Symbol,
				Type:      "enter",
				OrderType: "market",
			},
			Volume: decimal.NewFromFloat64(0),
			Price:  decimal.NewFromFloat64(0),
		},
	}

	position.intent.Store(intentFlat)
	position.onOrder = position.OrderAck
	position.onExecution = position.ExecutionAck
	position.subscribe()

	return position
}

/*
Pending reports whether an order is outstanding for this lot.
*/
func (position *Position) Pending() bool {
	if position == nil {
		return false
	}

	intent, _ := position.intent.Load().(string)

	return intent == intentEntryPending ||
		intent == intentExitPending ||
		intent == intentReducePending
}

func (position *Position) Status() types.Status {
	return position.status
}

/*
Stop returns the regulator owned by this Position.
*/
func (position *Position) Stop() *types.Stoploss {
	if position == nil {
		return nil
	}

	return position.stoploss
}

/*
TakeStop installs the lot's Stoploss on this Position and keeps the same
pointer on the Balance holding for wire/recovery.
*/
func (position *Position) TakeStop(holding *types.Holding) {
	if position == nil || holding == nil {
		return
	}

	if holding.Stoploss == nil {
		holding.Stoploss = types.NewStoploss(context.Background())
	}

	position.stoploss = holding.Stoploss
}

/*
ObserveMark ratchets the Position stop from a live mark print.
*/
func (position *Position) ObserveMark(mark float64) {
	if position == nil || position.stoploss == nil {
		return
	}

	position.stoploss.ObserveMark(mark)
}

/*
BindStop latches the Position regulator at entry.
*/
func (position *Position) BindStop(entry, trail float64) {
	if position == nil {
		return
	}

	if position.stoploss == nil {
		position.stoploss = types.NewStoploss(context.Background())
	}

	position.stoploss.Bind(entry, trail)
}

/*
subscribe registers order/execution handlers once. Marks arrive via Desk from
the single Price ticker decode path.
*/
func (position *Position) subscribe() {
	if position == nil || position.api == nil || position.subOrder != 0 {
		return
	}

	position.subOrder = position.api.On("add_order", position.onOrder)
	position.subExec = position.api.On("executions", position.onExecution)
}

/*
unsubscribe drops this position's channel handlers. Safe to call more than once.
*/
func (position *Position) unsubscribe() {
	if position == nil || position.api == nil {
		return
	}

	if position.subOrder != 0 {
		position.api.Unsubscribe("add_order", position.subOrder)
		position.subOrder = 0
	}

	if position.subExec != 0 {
		position.api.Unsubscribe("executions", position.subExec)
		position.subExec = 0
	}
}

/*
Close unsubscribes channel handlers once. Desk.evict owns map removal so this
method never re-enters through onTerminal.
*/
func (position *Position) Close() {
	if position == nil || !position.closed.CompareAndSwap(false, true) {
		return
	}

	position.unsubscribe()
}

func (position *Position) OrderAck(buf []byte) {
	orderAck := kraken.NewOrderResponse(buf)

	if position.request == nil || orderAck.ReqID != position.request.ReqID {
		return
	}

	if errnie.Error(kraken.Validate(orderAck)) != nil {
		position.status = types.ERROR
		position.intent.Store(intentFlat)
		position.claim.Release()
		return
	}

	position.orderID = orderAck.Result.OrderID
	position.status = types.PENDING
}

func (position *Position) ExecutionAck(buf []byte) {
	execution := kraken.NewExecution(buf)

	if errnie.Error(kraken.Validate(execution)) != nil {
		position.status = types.ERROR
		position.claim.Release()
		return
	}

	for _, data := range execution.Data {
		if data.OrderID != position.orderID {
			continue
		}

		if data.ExecID != "" {
			if _, seen := position.seenExec.LoadOrStore(data.ExecID, struct{}{}); seen {
				continue
			}
		}

		value, ok := position.balance.holdings.Load(data.Symbol)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"holding not found for "+data.Symbol,
				nil,
			))

			continue
		}

		holding := value.(*types.Holding)
		position.TakeStop(holding)
		position.price.Fill(position.pair, holding, data)
		position.bindEntryStop(holding, data)
		position.executions = append(position.executions, execution)

		status := position.fillStatus(holding, data)
		holding.Status = status
		position.status = status
		position.noteIntent(status)

		if position.balance != nil {
			position.balance.Publish()
		}

		if holding.Status == types.CLOSED || holding.Status == types.CANCELED {
			position.terminal()
		}
	}
}

func (position *Position) noteIntent(status types.Status) {
	switch status {
	case types.OPEN:
		position.intent.Store(intentOpen)

		if position.request != nil && position.request.Params.Side == "buy" {
			position.claim.Consume()
		}
	case types.CLOSED, types.CANCELED, types.ERROR:
		position.intent.Store(intentFlat)
	}
}

/*
bindEntryStop latches the Position regulator after a buy fill using paid fee
width in return space.
*/
func (position *Position) bindEntryStop(
	holding *types.Holding,
	data kraken.ExecutionData,
) {
	if position == nil || holding == nil || data.Side != "buy" {
		return
	}

	if holding.EntryPrice == nil || holding.EntryPrice.Sign() <= 0 {
		return
	}

	trail := 0.0

	if holding.EntryFee != nil && holding.Qty != nil && holding.Qty.Sign() > 0 &&
		position.price != nil {
		notional := position.price.Mul(holding.EntryPrice, holding.Qty)

		if notional != nil && notional.Sign() > 0 {
			trail = position.price.Div(holding.EntryFee, notional).Float64()
		}
	}

	position.BindStop(holding.EntryPrice.Float64(), trail)
	holding.Stoploss = position.stoploss
}

/*
fillStatus maps an execution print onto inventory status. Buys that trade stay
open; a sell only closes when remaining qty is exhausted — partial reduces must
keep the Desk shell so paper remainders stay sellable.
*/
func (position *Position) fillStatus(
	holding *types.Holding,
	data kraken.ExecutionData,
) types.Status {
	if data.OrderStatus == "canceled" || data.ExecType == "canceled" {
		if position.request != nil && position.request.Params.Side == "sell" {
			return types.OPEN
		}

		position.claim.Release()

		return types.CANCELED
	}

	if data.Side == "sell" &&
		(data.OrderStatus == "filled" || data.ExecType == "trade") {
		if holding == nil || holding.Qty == nil || holding.Qty.Sign() <= 0 {
			return types.CLOSED
		}

		return types.OPEN
	}

	if data.ExecType == "trade" {
		return types.OPEN
	}

	return types.MarketStatuses[data.OrderStatus]
}

func (position *Position) Enter() error {
	holding, err := position.balance.Holding(position.pair.Symbol)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get holding for "+position.pair.Symbol,
			err,
		))
	}

	amount, err := position.price.Taker(position.pair, holding.Qty)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to calculate taker cost: "+err.Error(),
			err,
		))
	}

	if position.claim.ID() != "" {
		if !position.claim.Funded(amount) {
			return errnie.Error(ErrInsufficientReservation())
		}
	} else if !position.balance.Available(amount) {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"insufficient balance",
			nil,
		))
	}

	position.request = kraken.NewMarketOrder(
		"buy",
		holding.Qty.Float64(),
		holding.Symbol,
	)

	position.status = types.PENDING
	position.intent.Store(intentEntryPending)

	if err := position.api.AddOrder(position.request); err != nil {
		position.status = types.ERROR
		position.intent.Store(intentFlat)
		position.claim.Release()

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	return nil
}

/*
Exit submits a market sell for the full filled quantity.
*/
func (position *Position) Exit() error {
	return position.Sell(nil)
}

/*
Sell submits a market sell. Nil quantity sells the full live lot; a positive
quantity reduces. Outstanding exit/reduce intents are not re-armed. Missing or
zero wallet Available flattens the lot without contacting the venue; a rejected
AddOrder clears the shell the same way Enter cleans up a failed buy so Decide
cannot hammer paper/live with the same ghost size every tick.
*/
func (position *Position) Sell(quantity *decimal.Decimal) error {
	if position.Pending() {
		return nil
	}

	holding, err := position.balance.Holding(position.pair.Symbol)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get holding for "+position.pair.Symbol,
			err,
		))
	}

	size := holding.Qty

	if quantity != nil {
		size = quantity
	}

	if size == nil || size.Sign() <= 0 {
		position.terminal()

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"sell quantity must be positive for "+position.pair.Symbol,
			nil,
		))
	}

	available, availableErr := position.balance.AssetAvailable(holding.Asset)

	if availableErr != nil {
		// Missing wallet row means the lot is not sellable — flatten the ghost
		// shell instead of sending holding.Qty into a venue that already has zero.
		position.balance.closeHolding(position.pair.Symbol)
		position.terminal()
		_ = position.balance.Resync()

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"no wallet availability to sell "+position.pair.Symbol,
			availableErr,
		))
	}

	if available.Sign() <= 0 {
		position.balance.closeHolding(position.pair.Symbol)
		position.terminal()

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"no sellable "+holding.Asset+" available for "+position.pair.Symbol,
			nil,
		))
	}

	if size.Sub(available).Sign() > 0 {
		size = available
	}

	position.request = kraken.NewMarketOrder(
		"sell",
		size.Float64(),
		holding.Symbol,
	)
	position.status = types.PENDING

	if quantity != nil {
		position.intent.Store(intentReducePending)
	} else {
		position.intent.Store(intentExitPending)
	}

	if err := position.api.AddOrder(position.request); err != nil {
		// Match Enter: venue rejection clears the lot so Decide cannot re-arm
		// the same sell every tick against paper/live balances that already
		// report flat (stale local Available must not keep a ghost OPEN shell).
		position.status = types.ERROR
		position.intent.Store(intentFlat)
		position.balance.closeHolding(position.pair.Symbol)
		position.terminal()
		_ = position.balance.Resync()

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	return nil
}

/*
terminal asks Desk to evict this lot, or drops the holding locally when no Desk
callback is wired (unit tests and orphaned positions).
*/
func (position *Position) terminal() {
	if position.onTerminal != nil && position.pair != nil {
		position.onTerminal(position.pair.Symbol)
		return
	}

	if position.balance != nil && position.pair != nil {
		position.balance.closeHolding(position.pair.Symbol)
	}

	position.Close()
}
