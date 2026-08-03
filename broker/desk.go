package broker

import (
	"context"
	"errors"
	"iter"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
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
		positions:    &sync.Map{},
		maxPositions: viper.GetViper().GetInt("trading.slots.normal"),
		maxReserved:  viper.GetViper().GetInt("trading.slots.reserved"),
	}

	if err := desk.recover(); err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"desk: failed to recover account positions",
			err,
		))
	}

	desk.run()
	return desk
}

func (desk *Desk) run() {
	go func() {
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
				// statement of what is actually held.
				desk.balance.Update()
				desk.publishEquity()
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
recover adopts the wallet as the account's open inventory.

Holding a coin is the position — there is no separate lot ledger to rebuild.
Every non-quote asset the wallet carries is therefore an open holding at the
quantity the wallet states, and trade history is consulted for one thing only:
what was paid for the amount currently held. That basis is the average price
across the most recent buys covering the held quantity, which is all a mark can
be measured against.
*/
func (desk *Desk) recover() error {
	balances, err := desk.api.Balance()

	if err != nil {
		return errnie.Error(err)
	}

	history, err := desk.api.TradeBalance()

	if err != nil {
		return errnie.Error(err)
	}

	quote := desk.api.Normalizer().Name(viper.GetString("market.quote_currency"))

	for asset, amount := range balances {
		asset = desk.api.Normalizer().Name(asset)

		// The quote currency is the cash side of every pair, not a lot.
		if asset == "" || asset == quote || amount == nil || amount.Sign() <= 0 {
			continue
		}

		symbol := asset + "/" + quote

		pair, err := desk.instrument.Pair(symbol)

		if err != nil {
			errnie.Error(err)
			continue
		}

		histories := make([]spot.Trade, 0)

		// Venues spell a pair without a separator and in any case, so the
		// trade is matched against the instrument's own base and quote.
		venue := strings.ToUpper(pair.Base + pair.Quote)

		for _, trade := range history.Trades {
			traded := strings.ToUpper(strings.ReplaceAll(trade.Pair, "/", ""))

			if traded != venue {
				continue
			}

			histories = append(histories, trade)
		}

		if len(histories) == 0 {
			continue
		}

		// Sort by timestamp ascending
		sort.Slice(histories, func(i, j int) bool {
			return histories[i].Time.Cmp(histories[j].Time) < 0
		})

		// Buys after the last sell are the ones that bought what is held now.
		opening := histories

		for index := len(histories) - 1; index >= 0; index-- {
			if strings.EqualFold(histories[index].Type, "sell") {
				opening = histories[index+1:]
				break
			}
		}

		if len(opening) == 0 {
			continue
		}

		// The wallet reports an amount of the asset, not an order size, so
		// the venue's lot rules decide how much of it is actually sellable.
		quantity, err := desk.api.Normalizer().FormatSize(symbol, amount)

		if err != nil || quantity == nil || quantity.Sign() <= 0 {
			errnie.Error(err)
			continue
		}

		entryAt := time.Unix(opening[0].Time.Int64(), 0).UTC()
		entryCost := decimal.NewFromInt64(0)
		entryFee := decimal.NewFromInt64(0)

		for _, trade := range opening {
			if trade.Cost == nil || trade.Volume == nil {
				continue
			}

			entryCost = entryCost.Add(trade.Cost)
			entryFee = entryFee.Add(trade.Fee)
		}

		if entryCost.Sign() <= 0 || entryFee.Sign() <= 0 {
			continue
		}

		entryPrice := entryCost.Div(quantity)

		position := NewPosition(
			desk.ctx,
			desk.api,
			desk.ui,
			desk.instrument,
			desk.price,
			desk.balance,
			pair,
			types.Decision{
				ID:               "recovered:" + symbol,
				ProposedQuantity: quantity,
				EntryPrice:       entryPrice,
				EntryFee:         entryFee,
				Mark:             entryPrice,
			},
		)

		position.ID = "recovered:" + symbol
		position.EntryOrder.ClOrdId = position.ID
		position.Status = types.OPEN

		// The holding built by NewPosition owns the lot's stoploss and its
		// cancel context, so recovery fills that one in rather than swapping
		// in a fresh struct that would leave the lot unprotected and its
		// context orphaned.
		position.Holding.Asset = asset
		position.Holding.Qty = quantity
		position.Holding.SellableQty = quantity.Copy()
		position.Holding.EntryAt = &entryAt
		position.Holding.Status = types.OPEN

		desk.positions.Store(symbol, position)
		position.Publish()
	}

	return nil
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

func (desk *Desk) OpenPositions() int {
	count := 0

	desk.positions.Range(func(key, value any) bool {
		position, ok := value.(*Position)

		if !ok || position == nil {
			return true
		}

		position.mu.RLock()
		status := position.Status
		position.mu.RUnlock()

		if status != types.CLOSED {
			count++
		}

		return true
	})

	return count
}

/*
publishEquity reports what the desk is worth if every open lot were closed now.

Cash alone understates the account while positions are open, so the unrealized
total is published beside it along with their sum. Every lot's PnL is already
net of the fees to get in and out, which makes equity the balance the wallet
would actually settle at rather than a gross mark-to-market.
*/
func (desk *Desk) publishEquity() {
	if desk == nil || desk.ui == nil || desk.balance == nil {
		return
	}

	cash, err := desk.balance.Cash()

	if err != nil || cash == nil {
		return
	}

	unrealized := decimal.NewFromInt64(0)

	for position := range desk.Positions() {
		if position.Status != types.OPEN || position.Holding == nil {
			continue
		}

		if position.Holding.PnL == nil {
			continue
		}

		unrealized = unrealized.Add(position.Holding.PnL)
	}

	out := datura.NewMap()
	out["equity"] = datura.NewMap(
		"cash", cash,
		"unrealized", unrealized,
		"equity", cash.Add(unrealized),
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

			return yield(position.snapshot())
		})
	}
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
			err = errors.Join(err, desk.exit(decision))
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
			position.mu.RLock()
			status := position.Status
			position.mu.RUnlock()

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

	entryValue := decimal.ExactMul(tick.Ask, decision.ProposedQuantity)
	entryFee := decimal.ExactMul(entryValue, feeRate)

	if entryValue == nil || entryFee == nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"desk: could not estimate entry basis for "+decision.Symbol,
			nil,
		))
	}

	decision.EntryPrice = tick.Ask.Copy()
	decision.EntryFee = entryFee
	decision.Mark = decision.EntryPrice.Copy()

	position := NewPosition(
		desk.ctx,
		desk.api,
		desk.ui,
		desk.instrument,
		desk.price,
		desk.balance,
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

func (desk *Desk) exit(decision types.Decision) error {
	value, ok := desk.positions.Load(decision.Symbol)

	if ok {
		if position, ok := value.(*Position); ok && position != nil {
			position.mu.RLock()
			status := position.Status
			position.mu.RUnlock()

			if status != types.CLOSED {
				return position.Exit(decision.ID)
			}
		}
	}

	return errnie.Error(errnie.Err(
		errnie.NotFound,
		"desk: active position not found for exit",
		nil,
	))
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
