package broker

import (
	"context"
	"errors"
	"iter"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
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

	if err := desk.recover(); err != nil {
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
				tickers, ok := message.(*kraken.Ticker)

				if !ok || tickers == nil {
					continue
				}

				// Positions are keyed by symbol, so each row goes straight to
				// the one lot it concerns.
				for _, ticker := range tickers.Data {
					desk.price.Update(&ticker)

					value, ok := desk.positions.Load(ticker.Symbol)

					if !ok {
						continue
					}

					position, ok := value.(*Position)

					if ok && position != nil {
						position.onTicker(ticker)
					}
				}

				// The wallet is re-read from REST because it is the only
				// statement of what is actually held. Refresh asynchronously
				// so the ticker-processing path is never blocked, and guard
				// against overlapping REST calls.
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
						position.onExecution(execution)
					}

					return true
				})
			}
		}
	}()
}

/*
recover rebuilds the wallet's current inventory from complete fill history and
adopts any working sell before the position can submit another one.
*/
func (desk *Desk) recover() error {
	balances, err := desk.api.Balance()

	if err != nil {
		return errnie.Error(err)
	}

	history, err := desk.api.TradesHistory()

	if err != nil {
		return errnie.Error(err)
	}

	working, err := desk.api.OpenOrders()

	if err != nil {
		return errnie.Error(err)
	}

	if err := desk.cancelRecoveredBuys(working.Open); err != nil {
		return err
	}

	quote := desk.api.Normalizer().Name(viper.GetString("market.quote_currency"))

	for asset, amount := range balances {
		asset = desk.api.Normalizer().Name(asset)

		if asset == "" || asset == quote || amount == nil || amount.Sign() <= 0 {
			continue
		}

		if err := desk.recoverAsset(asset, quote, amount, history.Trades, working.Open); err != nil {
			return err
		}
	}

	for orderID, order := range working.Open {
		if order.Description != nil && strings.EqualFold(order.Description.Type, "sell") {
			return errnie.Error(errnie.Err(
				errnie.Conflict,
				"desk: working sell "+orderID+" has no reconciled wallet inventory",
				nil,
			))
		}
	}

	return nil
}

func (desk *Desk) cancelRecoveredBuys(orders map[string]spot.Order) error {
	for orderID, order := range orders {
		if order.Description == nil || !strings.EqualFold(order.Description.Type, "buy") {
			continue
		}

		if _, err := uuid.Parse(order.ClOrdID); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Conflict,
				"desk: working buy "+orderID+" is not identifiable as a symm order",
				nil,
			))
		}

		result, err := desk.api.CancelOrder(&spot.CancelOrderRequest{TxID: orderID})

		if err != nil {
			return errnie.Error(err)
		}

		if result.Count <= 0 && !result.Pending {
			return errnie.Error(errnie.Err(
				errnie.NotFound,
				"desk: working entry "+orderID+" could not be canceled",
				nil,
			))
		}

		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"desk: canceled working entry "+orderID+"; restart after it reaches a terminal state",
			nil,
		))
	}

	return nil
}

func (desk *Desk) recoverAsset(
	asset string,
	quote string,
	amount *decimal.Decimal,
	history map[string]spot.Trade,
	working map[string]spot.Order,
) error {
	symbol := asset + "/" + quote
	pair, err := desk.instrument.Pair(symbol)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"desk: recovered wallet symbol unavailable: "+symbol,
			err,
		))
	}

	quantity, err := desk.api.Normalizer().FormatSize(symbol, amount)

	if err != nil || quantity == nil || quantity.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation, "desk: invalid recovered quantity for "+symbol, err,
		))
	}

	entryPrice, entryFee, entryAt, err := desk.recoverBasis(pair, quantity, history)

	if err != nil {
		return err
	}

	position := desk.recoveredPosition(pair, asset, quantity, entryPrice, entryFee, entryAt)
	orderID, order, err := recoveredSell(pair, working)

	if err != nil {
		return err
	}

	if order != nil {
		position.adoptExit(orderID, *order)
		delete(working, orderID)
	}

	desk.positions.Store(symbol, position)
	position.publishSnapshot()
	position.Publish()

	return nil
}

func (desk *Desk) recoverBasis(
	pair kraken.InstrumentPair,
	held *decimal.Decimal,
	history map[string]spot.Trade,
) (*decimal.Decimal, *decimal.Decimal, time.Time, error) {
	trades := make([]spot.Trade, 0)
	venue := strings.ToUpper(pair.Base + pair.Quote)

	for _, trade := range history {
		if strings.ToUpper(strings.ReplaceAll(trade.Pair, "/", "")) == venue {
			trades = append(trades, trade)
		}
	}

	sort.Slice(trades, func(left, right int) bool {
		return trades[left].Time.Cmp(trades[right].Time) < 0
	})

	quantity := decimal.NewFromInt64(0)
	cost := decimal.NewFromInt64(0)
	fee := decimal.NewFromInt64(0)
	entryAt := time.Time{}

	for _, trade := range trades {
		if trade.Volume == nil || trade.Cost == nil || trade.Time == nil {
			return nil, nil, time.Time{}, errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"desk: incomplete trade history for "+pair.Symbol,
				nil,
			))
		}

		if strings.EqualFold(trade.Type, "buy") {
			if quantity.Sign() == 0 {
				entryAt = time.Unix(trade.Time.Int64(), 0).UTC()
			}

			quantity = addAmount(quantity, trade.Volume)
			cost = addAmount(cost, trade.Cost)

			if trade.Fee != nil {
				fee = addAmount(fee, trade.Fee)
			}

			continue
		}

		if !strings.EqualFold(trade.Type, "sell") {
			return nil, nil, time.Time{}, errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"desk: unknown trade side in history for "+pair.Symbol,
				nil,
			))
		}

		if trade.Volume.Cmp(quantity) > 0 {
			return nil, nil, time.Time{}, errnie.Error(errnie.Err(
				errnie.Conflict,
				"desk: sell history exceeds recovered inventory for "+pair.Symbol,
				nil,
			))
		}

		remaining := subtractAmount(quantity, trade.Volume)

		if remaining.Sign() == 0 {
			quantity = remaining
			cost = decimal.NewFromInt64(0)
			fee = decimal.NewFromInt64(0)
			entryAt = time.Time{}
			continue
		}

		costScale := max(cost.GetScale(), int64(pair.CostPrecision), int64(feeRateScale))
		feeScale := max(fee.GetScale(), int64(pair.CostPrecision), int64(feeRateScale))
		previous := quantity
		quantity = remaining
		cost = cost.SetScale(costScale).Mul(remaining).Div(previous)
		fee = fee.SetScale(feeScale).Mul(remaining).Div(previous)
	}

	formatted, err := desk.api.Normalizer().FormatSize(pair.Symbol, quantity)

	if err != nil || formatted == nil || formatted.Cmp(held) != 0 || cost.Sign() <= 0 {
		return nil, nil, time.Time{}, errnie.Error(errnie.Err(
			errnie.Conflict,
			"desk: fill history does not reconcile with wallet inventory for "+pair.Symbol,
			err,
		))
	}

	return cost.Div(quantity), fee, entryAt, nil
}

func (desk *Desk) recoveredPosition(
	pair kraken.InstrumentPair,
	asset string,
	quantity *decimal.Decimal,
	entryPrice *decimal.Decimal,
	entryFee *decimal.Decimal,
	entryAt time.Time,
) *Position {
	position := NewPosition(
		desk.ctx, desk.api, desk.ui, desk.instrument, desk.price, desk.balance,
		desk.recorder, pair, types.Decision{
			ID:               "recovered:" + pair.Symbol,
			ProposedQuantity: quantity,
			EntryPrice:       entryPrice,
			EntryFee:         entryFee,
			Mark:             entryPrice,
			Risk:             desk.price.RiskPlan(pair),
		},
	)

	position.Status = types.OPEN
	position.entryTerminal = true
	position.Holding.Asset = asset
	position.Holding.Qty = quantity
	position.Holding.SellableQty = quantity
	position.Holding.EntryAt = &entryAt
	position.Holding.Status = types.OPEN
	position.Holding.Stoploss.BindRecovered()

	return position
}

func recoveredSell(
	pair kraken.InstrumentPair,
	orders map[string]spot.Order,
) (string, *spot.Order, error) {
	venue := strings.ToUpper(pair.Base + pair.Quote)
	orderID := ""
	var recovered *spot.Order

	for candidateID, order := range orders {
		if order.Description == nil || !strings.EqualFold(order.Description.Type, "sell") {
			continue
		}

		ordered := strings.ToUpper(strings.ReplaceAll(order.Description.Pair, "/", ""))

		if ordered != venue {
			continue
		}

		if recovered != nil {
			return "", nil, errnie.Error(errnie.Err(
				errnie.Conflict,
				"desk: multiple working sells exist for "+pair.Symbol,
				nil,
			))
		}

		copy := order
		orderID = candidateID
		recovered = &copy
	}

	return orderID, recovered, nil
}

func addAmount(left, right *decimal.Decimal) *decimal.Decimal {
	scale := max(left.GetScale(), right.GetScale())
	return left.SetScale(scale).Add(right.SetScale(scale))
}

func subtractAmount(left, right *decimal.Decimal) *decimal.Decimal {
	scale := max(left.GetScale(), right.GetScale())
	return left.SetScale(scale).Sub(right.SetScale(scale))
}

/*
isTerminal reports whether a lot can no longer trade. These are exactly the
states the UI retires a row on, so the two sides agree on what "gone" means.
*/
func isTerminal(status types.Status) bool {
	switch status {
	case types.CLOSED, types.CANCELED, types.REJECTED, types.EXPIRED,
		types.ERROR, types.FATAL:
		return true
	default:
		return false
	}
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
PublishEquity reports what the desk is worth if every open lot were closed now.

Cash alone understates the account while positions are open. Unrealized is the
profit/loss only; equity is cash plus the basis committed to open positions plus
that profit/loss.
*/
func (desk *Desk) PublishEquity() {
	if desk == nil || desk.ui == nil || desk.balance == nil {
		return
	}

	cash, err := desk.balance.Cash()

	if err != nil || cash == nil {
		return
	}

	unrealized := decimal.NewFromInt64(0)
	invested := decimal.NewFromInt64(0)

	for position := range desk.Positions() {
		if isTerminal(position.Status) || position.Holding == nil ||
			position.Holding.SellableQty == nil || position.Holding.SellableQty.Sign() <= 0 {
			continue
		}

		if position.Holding.EntryPrice != nil {
			basis := position.Holding.EntryPrice.Mul(position.Holding.SellableQty)

			if position.Holding.EntryFee != nil {
				basisScale := max(basis.GetScale(), position.Holding.EntryFee.GetScale())
				basis = basis.SetScale(basisScale).Add(position.Holding.EntryFee)
			}

			investedScale := max(invested.GetScale(), basis.GetScale())
			invested = invested.SetScale(investedScale).Add(basis)
		}

		if position.Holding.PnL != nil {
			unrealizedScale := max(
				unrealized.GetScale(),
				position.Holding.PnL.GetScale(),
			)
			unrealized = unrealized.
				SetScale(unrealizedScale).
				Add(position.Holding.PnL)
		}
	}

	equityScale := max(cash.GetScale(), invested.GetScale(), unrealized.GetScale())
	equity := cash.SetScale(equityScale).Add(invested).Add(unrealized)
	tradeBalance, err := desk.api.TradeBalance()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"desk: could not fetch trade balance",
			err,
		))
	}

	if tradeBalance.UnrealizedPnL != nil {
		unrealized = tradeBalance.UnrealizedPnL
	}

	if tradeBalance.Equity != nil {
		equity = tradeBalance.Equity
	}

	out := datura.NewMap()
	out["equity"] = datura.NewMap(
		"cash", cash,
		"invested", invested,
		"unrealized", unrealized,
		"equity", equity,
	)

	utils.Publish(desk.ui, out)
}

func (desk *Desk) Positions() iter.Seq[*Position] {
	return func(yield func(*Position) bool) {
		desk.positions.Range(func(key, value any) bool {
			position, ok := value.(*Position)

			if !ok || position == nil {
				return true
			}

			return yield(position.Snapshot())
		})
	}
}

/*
PublishPositions sends all current non-terminal positions to the UI channel
so newly connected clients can see open positions immediately.
*/
func (desk *Desk) PublishPositions() []byte {
	out := datura.NewMap()
	out["positions"] = []*Position{}

	for position := range desk.Positions() {
		status := position.Status

		if isTerminal(status) {
			continue
		}

		out["positions"] = append(out["positions"].([]*Position), position)
	}

	return out.MarshalAndFree()
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
			err = errors.Join(err, errnie.Err(
				errnie.NotAcceptable,
				"desk: strategy exits are disabled; only a triggered stoploss may submit a sell",
				nil,
			))
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

	desk.positionsMu.Lock()
	defer desk.positionsMu.Unlock()

	if value, ok := desk.positions.Load(decision.Symbol); ok {
		if position, ok := value.(*Position); ok && position != nil {
			status := position.Snapshot().Status

			if status != types.CLOSED {
				return errnie.Error(errnie.Err(
					errnie.NotAcceptable,
					"desk: symbol already has an active position",
					nil,
				))
			}
		}
	}

	if desk.OpenSlots(decision.Opportunity) <= 0 {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"desk: position capacity is exhausted",
			nil,
		))
	}

	pair, err := desk.instrument.Pair(decision.Symbol)

	if err != nil {
		return errnie.Error(err)
	}

	// A pending market buy starts with the current executable ask as its
	// provisional basis. The private fill replaces this estimate with the
	// venue's realized average price.
	tick := desk.price.Tick(pair.Symbol)

	if tick == nil || tick.Ask == nil || tick.Ask.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"desk: cannot price "+decision.Symbol+" for entry",
			nil,
		))
	}

	feeRate, err := desk.price.Fee(pair.Symbol)

	if err != nil {
		return errnie.Error(err)
	}

	entryValue := tick.Ask.Mul(decision.ProposedQuantity)
	entryFee := feeRate.Mul(entryValue)

	if entryValue == nil || entryFee == nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"desk: could not estimate entry basis for "+decision.Symbol,
			nil,
		))
	}

	decision.EntryPrice = tick.Ask
	decision.EntryFee = entryFee
	decision.Mark = tick.Bid

	/*
		An entry without stop geometry is refused rather than fitted with some.

		The desk could derive a plan here, and an earlier version did — but the
		quantity arriving with the decision was solved against whatever distance
		the allocator used, and attaching a different distance after the fact
		breaks exactly the coupling that makes a wide stop affordable. A lot
		sized for a ten-cent boundary and then defended at forty is carrying
		four times the loss it was budgeted.

		Recovered wallet inventory is the one case that legitimately has no
		plan, and it does not come through here.
	*/
	if !decision.Risk.Present {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"desk: entry for "+decision.Symbol+" carries no risk geometry",
			nil,
		))
	}

	position := NewPosition(
		desk.ctx,
		desk.api,
		desk.ui,
		desk.instrument,
		desk.price,
		desk.balance,
		desk.recorder,
		pair,
		decision,
	)

	desk.positions.Store(pair.Symbol, position)

	if _, err := position.Enter(); err != nil {
		desk.positions.Delete(pair.Symbol)

		return errnie.Error(errors.Join(
			errnie.Err(
				errnie.UnprocessableContent,
				"desk: could not enter position",
				nil,
			),
			err,
		))
	}

	return nil
}

/*
ApplyEvidence hands the strategy's reading of one symbol to the position that
holds it.

This is the whole of the strategy's write access to an open lot. The evaluator
runs on the analyzer's goroutine and everything else about a position runs on
the desk's, so the strategy states what it observed and the desk decides when
that observation meets a book — rather than the strategy reaching across and
setting stop geometry itself.

An unknown or closed symbol is not an error. The thesis judges every symbol it
has evidence for, most of which the desk holds nothing in.
*/
func (desk *Desk) ApplyEvidence(evidence types.StopEvidence) {
	value, ok := desk.positions.Load(evidence.Symbol)

	if !ok {
		return
	}

	position, ok := value.(*Position)

	if !ok || position == nil {
		return
	}

	snapshot := position.Snapshot()

	if snapshot == nil || isTerminal(snapshot.Status) {
		return
	}

	position.ApplyEvidence(evidence)
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
