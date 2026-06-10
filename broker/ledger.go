package broker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
)

type inventory struct {
	quantity float64
	avgEntry float64
}

type quoteRow struct {
	mark      float64
	bid       float64
	ask       float64
	updatedAt time.Time
}

/*
Ledger tracks quote cash, open inventory, and marks for sizing and playbook
holding conditions.
*/
type Ledger struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q[any]
	bus         *internal.Bus
	mu          sync.RWMutex
	quoteCash   float64
	positions   map[string]inventory
	marks       map[string]float64
	quotes      map[string]quoteRow
	seenExecIDs map[string]struct{}
	holdings    logic.Holdings
}

func NewLedger(ctx context.Context, pool *qpool.Q[any]) *Ledger {
	ctx, cancel := context.WithCancel(ctx)

	quote := viper.GetString("market.quote_currency")

	return &Ledger{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		bus: internal.NewBus(
			ctx,
			pool,
			[]string{"ui"},
			[]internal.Subscription{
				internal.Subscribe("raw", "ledger"),
			},
		),
		quoteCash: viper.GetFloat64(
			"trading.paper.wallet." + quoteKey(quote),
		),
		positions:   make(map[string]inventory),
		marks:       make(map[string]float64),
		quotes:      make(map[string]quoteRow),
		seenExecIDs: make(map[string]struct{}),
	}
}

func quoteKey(quote string) string {
	switch quote {
	case "USD", "usd":
		return "usd"
	case "EUR", "eur":
		return "eur"
	default:
		return quote
	}
}

func (ledger *Ledger) Holdings() *logic.Holdings {
	ledger.mu.RLock()
	defer ledger.mu.RUnlock()

	return &ledger.holdings
}

func (ledger *Ledger) Tick() error {
	for {
		if errnie.Error(ledger.ctx.Err()) != nil {
			return ledger.ctx.Err()
		}

		message, err := ledger.bus.Receive("raw")

		if errnie.Error(err) != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}

			continue
		}

		if message == nil {
			continue
		}

		switch message.Type {
		case "balances":
			balances, ok := message.Value.(user.Balances)

			if !ok {
				errnie.Error(errors.New("ledger: invalid balances"))
				continue
			}

			ledger.applyBalances(balances)
		case "ticker":
			updates, ok := message.Value.(*krakenmarket.TickerUpdates)

			if !ok {
				continue
			}

			ledger.applyTicker(*updates)
		case "executions":
			updates, ok := message.Value.([]user.Execution)

			if !ok {
				errnie.Error(errors.New("ledger: invalid executions"))
				continue
			}

			for _, execution := range updates {
				ledger.applyExecution(execution)
			}
		}
	}
}

func (ledger *Ledger) applyBalances(balances user.Balances) {
	quote := viper.GetString("market.quote_currency")

	ledger.mu.Lock()
	ledger.quoteCash = quoteCashFromBalances(balances, quote)
	ledger.mu.Unlock()

	if reconcileErr := ledger.reconcileCash(balances); reconcileErr != nil {
		errnie.Error(reconcileErr)
	}

	ledger.publishPositions()
}

func (ledger *Ledger) reconcileCash(balances user.Balances) error {
	quote := viper.GetString("market.quote_currency")
	expected := quoteCashFromBalances(balances, quote)

	ledger.mu.RLock()
	actual := ledger.quoteCash
	ledger.mu.RUnlock()

	if expected <= 0 && actual <= 0 {
		return nil
	}

	delta := expected - actual

	if delta < -0.01 || delta > 0.01 {
		return fmt.Errorf(
			"ledger: quote cash mismatch expected %.8f actual %.8f",
			expected,
			actual,
		)
	}

	return nil
}

func (ledger *Ledger) applyTicker(updates krakenmarket.TickerUpdates) {
	ledger.mu.Lock()

	for _, update := range updates {
		if update == nil || update.Last <= 0 {
			continue
		}

		ledger.marks[update.Symbol] = update.Last

		quote := ledger.quotes[update.Symbol]
		quote.mark = update.Last
		quote.bid = update.Bid
		quote.ask = update.Ask

		if !update.Timestamp.IsZero() {
			quote.updatedAt = update.Timestamp
		} else {
			quote.updatedAt = time.Now().UTC()
		}

		ledger.quotes[update.Symbol] = quote
	}

	ledger.mu.Unlock()
}

func (ledger *Ledger) applyExecution(execution user.Execution) {
	if execution.LastQty <= 0 || execution.LastPrice <= 0 || execution.Symbol == "" {
		return
	}

	ledger.mu.Lock()

	if execution.ExecID != "" {
		if _, seen := ledger.seenExecIDs[execution.ExecID]; seen {
			ledger.mu.Unlock()
			return
		}

		ledger.seenExecIDs[execution.ExecID] = struct{}{}
	}

	switch trading.Side(execution.Side) {
	case trading.Buy:
		ledger.applyBuy(execution.Symbol, execution.LastQty, execution.LastPrice, executionFee(execution))
	case trading.Sell:
		ledger.applySell(execution.Symbol, execution.LastQty, execution.LastPrice, executionFee(execution))
	}

	ledger.mu.Unlock()
	ledger.publishPositions()
}

func (ledger *Ledger) applyBuy(symbol string, quantity float64, price float64, fee float64) {
	cost := quantity*price + fee
	ledger.quoteCash -= cost

	position := ledger.positions[symbol]
	totalQty := position.quantity + quantity

	if totalQty > 0 {
		position.avgEntry = (position.avgEntry*position.quantity + price*quantity) / totalQty
	}

	position.quantity = totalQty
	ledger.positions[symbol] = position
	ledger.holdings.SetQuantity(symbol, position.quantity)
}

func (ledger *Ledger) applySell(symbol string, quantity float64, price float64, fee float64) {
	proceeds := quantity*price - fee
	ledger.quoteCash += proceeds

	position := ledger.positions[symbol]
	position.quantity -= quantity

	if position.quantity <= 0 {
		delete(ledger.positions, symbol)
		ledger.holdings.SetQuantity(symbol, 0)
		return
	}

	ledger.positions[symbol] = position
	ledger.holdings.SetQuantity(symbol, position.quantity)
}

func (ledger *Ledger) QuoteCash() float64 {
	ledger.mu.RLock()
	defer ledger.mu.RUnlock()

	return ledger.quoteCash
}

func (ledger *Ledger) Mark(symbol string) (float64, bool) {
	ledger.mu.RLock()
	defer ledger.mu.RUnlock()

	mark, ok := ledger.marks[symbol]

	return mark, ok && mark > 0
}

/*
Quote returns the latest executable quote snapshot for one symbol.
*/
func (ledger *Ledger) Quote(symbol string) (QuoteSnapshot, bool) {
	ledger.mu.RLock()
	defer ledger.mu.RUnlock()

	quote, ok := ledger.quotes[symbol]

	if !ok || quote.mark <= 0 {
		return QuoteSnapshot{}, false
	}

	return QuoteSnapshot{
		Symbol:    symbol,
		Mark:      quote.mark,
		Bid:       quote.bid,
		Ask:       quote.ask,
		UpdatedAt: quote.updatedAt,
	}, true
}

func (ledger *Ledger) HeldQuantity(symbol string) float64 {
	ledger.mu.RLock()
	defer ledger.mu.RUnlock()

	return ledger.positions[symbol].quantity
}

func (ledger *Ledger) OpenCount() int {
	ledger.mu.RLock()
	defer ledger.mu.RUnlock()

	return ledger.holdings.OpenCount()
}

func (ledger *Ledger) publishPositions() {
	ledger.mu.RLock()

	rows := make([]map[string]any, 0, len(ledger.positions))

	for symbol, position := range ledger.positions {
		if position.quantity <= 0 {
			continue
		}

		rows = append(rows, map[string]any{
			"symbol":    symbol,
			"qty":       position.quantity,
			"avg_entry": position.avgEntry,
		})
	}

	ledger.mu.RUnlock()

	errnie.Error(ledger.bus.Send("ui", "positions", map[string]any{
		"event":     "positions",
		"positions": rows,
	}))

	for _, row := range rows {
		symbol, _ := row["symbol"].(string)
		mark, ok := ledger.Mark(symbol)

		if !ok {
			continue
		}

		errnie.Error(ledger.bus.Send("ui", "mark", map[string]any{
			"event":  "mark",
			"symbol": symbol,
			"price":  mark,
		}))
	}
}

func (ledger *Ledger) Close() error {
	ledger.cancel()
	return nil
}

func quoteCashFromBalances(balances user.Balances, quote string) float64 {
	for _, asset := range balances.Asset {
		name := asset.Asset

		if name == quote || name == "Z"+quote {
			return asset.Balance
		}
	}

	return 0
}

func executionFee(execution user.Execution) float64 {
	fee := 0.0

	for _, line := range execution.Fees {
		fee += line.Qty
	}

	if fee == 0 {
		fee = execution.FeeUsdEquiv
	}

	return fee
}
