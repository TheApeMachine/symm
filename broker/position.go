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
	intentFlat         = ""
	intentEntryPending = "entry_pending"
	intentOpen         = "open"
	// IntentReducePending is a partial sell awaiting fill acknowledgment.
	IntentReducePending = "reduce_pending"
	// IntentExitPending is a full exit sell awaiting fill acknowledgment.
	IntentExitPending = "exit_pending"
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
		intent == IntentExitPending ||
		intent == IntentReducePending
}

/*
PendingWire projects outstanding order identity for recovery checkpoints.
*/
func (position *Position) PendingWire() types.PendingOrderWire {
	if position == nil || position.pair == nil {
		return types.PendingOrderWire{}
	}

	intent, _ := position.intent.Load().(string)
	side := "buy"

	if intent == IntentExitPending || intent == IntentReducePending {
		side = "sell"
	}

	return types.PendingOrderWire{
		Symbol:        position.pair.Symbol,
		Side:          side,
		OrderID:       position.orderID,
		Intent:        intent,
		ReservationID: position.claim.ID(),
	}
}

/*
RestorePending re-arms an outstanding order intent on a recovered lot without
touching the venue. Boot reconcile calls this when a durable pending wire still
matches a resting exchange order, so Trader treats the lot as in-flight and does
not re-Enter or re-Exit an order the venue already holds.
*/
func (position *Position) RestorePending(wire types.PendingOrderWire) {
	if position == nil || wire.Intent == "" {
		return
	}

	position.orderID = wire.OrderID
	position.intent.Store(wire.Intent)
	position.status = types.PENDING

	if wire.ReservationID != "" {
		position.claim.Bind(position.balance, wire.ReservationID)
	}
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
		wasSell := position.requestWasSell()
		position.intent.Store(intentFlat)
		position.claim.Release()
		position.request = nil

		if wasSell {
			position.status = types.OPEN
			return
		}

		position.status = types.ERROR
		return
	}

	position.orderID = orderAck.Result.OrderID
	position.status = types.PENDING
}

func (position *Position) ExecutionAck(buf []byte) {
	execution := kraken.NewExecution(buf)

	if errnie.Error(kraken.Validate(execution)) != nil {
		// Malformed private frames must not fault every subscribed Position —
		// only ignore until a valid envelope names this order.
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
		position.noteIntent(status, data)
		position.reconcileInventory(holding)

		if position.balance != nil {
			position.balance.Publish()
		}

		if holding.Status == types.CLOSED || holding.Status == types.CANCELED {
			position.terminal()
		}
	}
}

/*
requestWasSell reports whether the outstanding request is an exit/reduce.
*/
func (position *Position) requestWasSell() bool {
	if position == nil || position.request == nil {
		return false
	}

	return position.request.Params.Side == "sell"
}

/*
orderTerminal reports whether the execution print ends the outstanding order.
*/
func orderTerminal(data kraken.ExecutionData) bool {
	return data.OrderStatus == "filled" ||
		data.OrderStatus == "canceled" ||
		data.ExecType == "canceled"
}

/*
noteIntent advances order intent only on terminal prints. Partial fills keep
entry/exit/reduce pending so Trader cannot overwrite an active order.
*/
func (position *Position) noteIntent(
	status types.Status,
	data kraken.ExecutionData,
) {
	switch status {
	case types.OPEN:
		if !orderTerminal(data) {
			return
		}

		if data.Side == "buy" && data.OrderStatus == "filled" {
			position.claim.Consume()
		}

		if data.Side == "buy" &&
			(data.OrderStatus == "canceled" || data.ExecType == "canceled") {
			position.claim.Release()
		}

		position.intent.Store(intentOpen)
		position.request = nil
		position.orderID = ""
	case types.CLOSED, types.CANCELED, types.ERROR:
		position.intent.Store(intentFlat)
		position.request = nil
		position.orderID = ""
	}
}

/*
reconcileInventory closes a lot only when the exchange Balance row is zero.
Sell fills never invent that close — wallet sync is the inventory authority.
*/
func (position *Position) reconcileInventory(holding *types.Holding) {
	if position == nil || holding == nil || position.balance == nil {
		return
	}

	if holding.Asset == "" {
		return
	}

	row, err := position.balance.Get(holding.Asset)

	if err != nil || row == nil {
		return
	}

	if row.Balance == nil {
		return
	}

	if row.Balance.Sign() > 0 {
		holding.Qty = row.Balance.Copy()

		if row.Available != nil {
			holding.SellableQty = row.Available.Copy()
		}

		holding.Status = types.OPEN
		position.status = types.OPEN
		return
	}

	holding.Status = types.CLOSED
	holding.Qty = decimal.NewFromFloat64(0)
	holding.SellableQty = decimal.NewFromFloat64(0)
	position.status = types.CLOSED
	position.intent.Store(intentFlat)
}

/*
bindEntryStop latches the Position regulator after a buy fill. The adverse band
is market-derived: paid entry fee, one-way exit fee at the same rate, and live
half-spread — so touch width alone cannot breach a fee-thin stop.
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

	trail := position.EntryTrail(holding)
	position.BindStop(holding.EntryPrice.Float64(), trail)
	holding.Stoploss = position.stoploss
}

/*
EntryTrail is the fill-time survival band in return space: round-trip fee plus
half the visible touch spread when the book is warm. Fee schedule Fraction is
used when the fill did not yet carry EntryFee.
*/
func (position *Position) EntryTrail(holding *types.Holding) float64 {
	if position == nil || holding == nil || position.price == nil {
		return 0
	}

	feeTrail := 0.0

	if holding.EntryFee != nil && holding.Qty != nil && holding.Qty.Sign() > 0 {
		notional := position.price.Mul(holding.EntryPrice, holding.Qty)

		if notional != nil && notional.Sign() > 0 {
			feeTrail = position.price.Div(holding.EntryFee, notional).Float64()
		}
	}

	// Exit will pay approximately the same taker fraction again.
	if feeTrail <= 0 {
		if fraction, err := position.price.Fraction(holding.Symbol); err == nil &&
			fraction != nil && fraction.Sign() > 0 {
			feeTrail = fraction.Float64()
		}
	}

	trail := feeTrail * 2

	if halfSpread := position.halfSpread(holding.Symbol); halfSpread > 0 {
		trail += halfSpread
	}

	return trail
}

/*
halfSpread returns half the touch-to-mid width in return space when Bid/Ask are
warm and positive; otherwise zero so EntryTrail stays fee-only.
*/
func (position *Position) halfSpread(symbol string) float64 {
	if position == nil || position.price == nil || symbol == "" {
		return 0
	}

	ticker, err := position.price.Get(symbol)

	if err != nil || ticker == nil || ticker.Bid == nil || ticker.Ask == nil {
		return 0
	}

	if ticker.Bid.Sign() <= 0 || ticker.Ask.Sign() <= 0 {
		return 0
	}

	mid := position.price.Div(ticker.Bid.Add(ticker.Ask), two)

	if mid == nil || mid.Sign() <= 0 {
		return 0
	}

	width := ticker.Ask.Sub(ticker.Bid)

	if width == nil || width.Sign() <= 0 {
		return 0
	}

	return position.price.Div(width, mid).Float64() / 2
}

/*
fillStatus maps an execution print onto inventory status. Buys and sells that
trade stay OPEN until Balance.syncWallet reports zero total Balance — Available
alone never closes a lot, and executions never invent a flat inventory state.
*/
func (position *Position) fillStatus(
	holding *types.Holding,
	data kraken.ExecutionData,
) types.Status {
	if data.OrderStatus == "canceled" || data.ExecType == "canceled" {
		if data.Side == "sell" || position.requestWasSell() {
			return types.OPEN
		}

		position.claim.Release()

		return types.CANCELED
	}

	if data.ExecType == "trade" || data.OrderStatus == "filled" {
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
		holding.Qty,
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
quantity reduces. Outstanding exit/reduce intents are not re-armed. Missing
wallet inventory flattens a ghost shell; zero Available with positive Balance
only refuses the sell (inventory is reserved). Venue rejection restores
managing state and never closes owned inventory.
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
		// No exchange row at all — local shell is a ghost, not reserved inventory.
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
		if holding.Qty != nil && holding.Qty.Sign() > 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"no sellable "+holding.Asset+" available for "+position.pair.Symbol,
				nil,
			))
		}

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
		size,
		holding.Symbol,
	)
	position.status = types.PENDING

	if quantity != nil {
		position.intent.Store(IntentReducePending)
	} else {
		position.intent.Store(IntentExitPending)
	}

	if err := position.api.AddOrder(position.request); err != nil {
		position.status = types.OPEN
		position.intent.Store(intentFlat)
		// Keep request for last-attempt sizing audit; Pending reads intent only.
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
