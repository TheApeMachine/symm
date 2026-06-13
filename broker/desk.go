package broker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
	symmmarket "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/observability"
	"github.com/theapemachine/symm/rawbus"
)

type Desk struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q[any]
	bus           *internal.Bus
	actions       *sync.Map
	fills         *sync.Map
	stops         *sync.Map
	quotes        *sync.Map
	touchRegistry *symmmarket.TouchRegistry
	positions     *PositionMonitor
	exitConfig    *ExitConfigStream
	riskGate      PreTradeRiskGate
	metrics       *observability.OperationalMetrics
	marginEnabled bool
	tradingConfig config.TradingConfig
}

func NewDesk(
	ctx context.Context,
	pool *qpool.Q[any],
	touchRegistry *symmmarket.TouchRegistry,
) *Desk {
	ctx, cancel := context.WithCancel(ctx)
	marketConfig, marketErr := config.LoadMarketConfig()

	if marketErr != nil {
		errnie.Error(marketErr)
		cancel()

		return nil
	}

	tradingConfig, tradingErr := config.LoadTradingConfig()

	if tradingErr != nil {
		errnie.Error(tradingErr)
		cancel()

		return nil
	}

	riskGate, riskGateErr := NewPreTradeRiskGate(tradingConfig)

	if riskGateErr != nil {
		errnie.Error(riskGateErr)
		cancel()

		return nil
	}

	if touchRegistry == nil {
		errnie.Error(errors.New("broker: touch registry is required"))
		cancel()

		return nil
	}

	exitConfig, exitConfigErr := config.LoadExitConfig()

	if exitConfigErr != nil {
		errnie.Error(exitConfigErr)
	}

	exitConfigStream, streamErr := NewExitConfigStream(exitConfig)

	if streamErr != nil {
		errnie.Error(streamErr)
		cancel()

		return nil
	}

	bus := internal.NewBus(
		ctx,
		pool,
		[]internal.Channel{
			internal.ChannelRaw,
			internal.ChannelKrakenPrivate,
			internal.ChannelUI,
			internal.ChannelAudit,
		},
		[]internal.Subscription{
			internal.Subscribe(internal.ChannelRaw, "desk"),
		},
	)

	return &Desk{
		ctx:           ctx,
		cancel:        cancel,
		pool:          pool,
		bus:           bus,
		actions:       &sync.Map{},
		fills:         &sync.Map{},
		stops:         &sync.Map{},
		quotes:        &sync.Map{},
		touchRegistry: touchRegistry,
		positions:     NewPositionMonitor(),
		exitConfig:    exitConfigStream,
		riskGate:      riskGate,
		metrics:       observability.Shared(),
		marginEnabled: marketConfig.MarginEnabled,
		tradingConfig: tradingConfig,
	}
}

func (desk *Desk) Tick() error {
	for {
		select {
		case <-desk.ctx.Done():
			return desk.ctx.Err()
		default:
		}

		message, err := desk.bus.Receive(internal.ChannelRaw)

		if internal.IsShutdown(err) {
			return err
		}

		if internal.ReportError(err) != nil || message == nil {
			continue
		}

		switch rawbus.TypeFrom(message.Type) {
		case rawbus.TypeTicker:
			tickers, ok := message.Value.(*market.TickerUpdates)

			if !ok || tickers == nil {
				errnie.Error(errors.New("desk: invalid tickers"))
				continue
			}

			for _, ticker := range *tickers {
				desk.onTicker(ticker)
			}
		case rawbus.TypeBook:
			books, ok := message.Value.(*market.BookUpdates)

			if !ok || books == nil {
				errnie.Error(errors.New("desk: invalid books"))
				continue
			}

			for _, book := range *books {
				desk.onBook(book)
			}
		case rawbus.TypeTrade:
			trades, ok := message.Value.(*market.TradeUpdates)

			if !ok || trades == nil {
				errnie.Error(errors.New("desk: invalid trades"))
				continue
			}

			for _, trade := range *trades {
				desk.onTrade(trade)
			}
		case rawbus.TypeOrder:
			action, err := rawbus.DecodeAction(message)

			if err != nil {
				errnie.Error(err)
				continue
			}

			if action == nil {
				continue
			}

			desk.onAction(action)
		case rawbus.TypeExecutions:
			updates, err := rawbus.DecodeExecutions(message)

			if err != nil {
				errnie.Error(err)
				continue
			}

			for _, execution := range updates {
				desk.onExecution(execution)
			}
		case rawbus.TypeBalances:
			balances, ok := message.Value.(user.Balances)

			if !ok {
				errnie.Error(errors.New("desk: invalid balances"))
				continue
			}

			changed := desk.positions.ApplyBalance(balances)

			if desk.syncStops(balances) {
				changed = true
			}

			if changed {
				desk.publishPositions()
			}
		}
	}
}

func (desk *Desk) onTicker(ticker *market.TickerUpdate) {
	if ticker == nil || ticker.Symbol == "" {
		return
	}

	desk.recordTickerAge(ticker)
	desk.syncTouchQuote(ticker.Symbol)

	snapshotTicker := desk.touchTicker(ticker.Symbol)

	if snapshotTicker == nil {
		return
	}

	if desk.touchPositionPrices(snapshotTicker) {
		desk.publishPositions()
	}

	desk.evaluateStop(snapshotTicker)
}

/*
evaluateStop checks the protective stop for the ticker's symbol against the
latest price and submits a market exit when the level is breached. It is driven
from every price source (ticker, book, trade) so that illiquid symbols whose
ticker stream dries up still get their stops enforced from book/trade updates.
*/
func (desk *Desk) evaluateStop(ticker *market.TickerUpdate) {
	if ticker == nil || ticker.Symbol == "" {
		return
	}

	raw, ok := desk.stops.Load(ticker.Symbol)

	if !ok {
		return
	}

	stopLoss, stopOK := raw.(*StopLoss)

	if !stopOK || stopLoss == nil {
		return
	}

	triggered, evaluateErr := stopLoss.Evaluate(ticker)

	if errnie.Error(evaluateErr) != nil {
		return
	}

	if !triggered {
		return
	}

	quantity := stopLoss.Quantity
	triggeredAt := time.Now().UTC()

	stopLoss.MarkTriggered(triggeredAt)
	desk.recordStopTriggered(stopLoss.Symbol, triggeredAt)

	if sendErr := desk.sendMarketOrder(trading.Sell, stopLoss.Symbol, quantity); errnie.Error(sendErr) != nil {
		stopLoss.MarkNeedsRepair()
		desk.recordStopNeedsRepair(stopLoss.Symbol, sendErr.Error())
		return
	}

	stopLoss.MarkExitSubmitted()
	desk.recordStopExitSubmitted(stopLoss.Symbol, triggeredAt)
}

func (desk *Desk) onBook(book *market.BookUpdate) {
	if desk == nil || book == nil || book.Symbol == "" {
		return
	}

	observedAt := time.Now().UTC()
	desk.recordBookAge(book, observedAt)
	desk.syncTouchQuote(book.Symbol)

	snapshotTicker := desk.touchTicker(book.Symbol)

	if snapshotTicker == nil {
		return
	}

	if desk.touchPositionPrices(snapshotTicker) {
		desk.publishPositions()
	}

	desk.evaluateStop(snapshotTicker)
}

func (desk *Desk) onTrade(trade *market.TradeUpdate) {
	if desk == nil || trade == nil || trade.Symbol == "" {
		return
	}

	observedAt := time.Now().UTC()
	desk.recordTradeAge(trade, observedAt)
	desk.syncTouchQuote(trade.Symbol)

	snapshotTicker := desk.touchTicker(trade.Symbol)

	if snapshotTicker == nil {
		return
	}

	if desk.touchPositionPrices(snapshotTicker) {
		desk.publishPositions()
	}

	desk.evaluateStop(snapshotTicker)
}

func (desk *Desk) syncTouchQuote(symbol string) {
	if desk == nil || desk.touchRegistry == nil || symbol == "" {
		return
	}

	touch, touchOK := desk.touchRegistry.Load(symbol, time.Now().UTC())

	if !touchOK {
		return
	}

	desk.persistQuote(quoteFromTouch(touch))
}

func (desk *Desk) touchTicker(symbol string) *market.TickerUpdate {
	if desk == nil || desk.touchRegistry == nil || symbol == "" {
		return nil
	}

	touch, touchOK := desk.touchRegistry.Load(symbol, time.Now().UTC())

	if !touchOK {
		return nil
	}

	snapshot := quoteFromTouch(touch)

	return quoteSnapshotTicker(snapshot)
}

func (desk *Desk) onExecution(execution user.Execution) {
	orderKey := execution.ClOrdID

	if orderKey == "" {
		orderKey = execution.OrderID
	}

	raw, ok := desk.actions.Load(orderKey)

	if !ok {
		return
	}

	action, actionOK := raw.(*logic.Action)

	if !actionOK || action == nil {
		return
	}

	desk.recordOrderExecution(orderKey, execution)

	terminal := isTerminalExecutionStatus(execution.OrderStatus)

	if execution.ExecType != "trade" && execution.OrderStatus != "filled" {
		if isRejectedExecutionStatus(execution.OrderStatus) {
			desk.markStopNeedsRepair(action)
		}

		if terminal {
			desk.actions.Delete(orderKey)
			desk.clearFilledQuantity(orderKey)
		}

		return
	}

	fillQty := desk.executionFillDelta(orderKey, execution, action)

	if fillQty <= 0 {
		if terminal {
			desk.actions.Delete(orderKey)
			desk.clearFilledQuantity(orderKey)
		}

		return
	}

	if terminal {
		desk.actions.Delete(orderKey)
		desk.clearFilledQuantity(orderKey)
	}

	fillPrice := execution.LastPrice

	if fillPrice <= 0 {
		fillPrice = execution.AvgPrice
	}

	entryPrice := economicEntryPrice(execution, fillQty, fillPrice)

	if action.Type.IsExit() || execution.Side == string(trading.Sell) {
		desk.applyStopExitFill(execution.Symbol, fillQty)

		if desk.positions.Reduce(execution.Symbol, fillQty) {
			desk.publishPositions()
		}

		return
	}

	if fillPrice <= 0 || fillQty <= 0 {
		return
	}

	stopLoss, stopErr := desk.entryStop(
		execution.Symbol,
		fillQty,
		entryPrice,
	)

	if errnie.Error(stopErr) != nil {
		return
	}

	desk.stops.Store(execution.Symbol, stopLoss)

	if desk.positions.ApplyStop(stopLoss) {
		desk.publishPositions()
	}
}

func (desk *Desk) Close() error {
	desk.cancel()

	if desk.exitConfig != nil {
		return desk.exitConfig.Close()
	}

	return nil
}

func (desk *Desk) syncStops(balances user.Balances) bool {
	if desk == nil || desk.stops == nil {
		return false
	}

	changed := false
	currency := normalizedCurrency(balances.Currency, desk.positions.currency)
	inventory := balanceInventory(balances, currency)
	seen := make(map[string]bool)

	for base, quantity := range inventory {
		if quantity <= 0 {
			continue
		}

		symbol := base + "/" + currency
		seen[symbol] = true

		entryPrice := balances.AvgEntry[base]

		if entryPrice <= 0 {
			continue
		}

		if desk.syncStopForAsset(symbol, quantity, entryPrice) {
			changed = true
		}
	}

	desk.stops.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok {
			return true
		}

		if !seen[symbol] {
			desk.stops.Delete(symbol)
			changed = true
		}

		return true
	})

	return changed
}

func (desk *Desk) syncStopForAsset(
	symbol string,
	quantity float64,
	entryPrice float64,
) bool {
	raw, exists := desk.stops.Load(symbol)

	if !exists {
		stopLoss, err := desk.entryStop(symbol, quantity, entryPrice)
		if errnie.Error(err) != nil || stopLoss == nil {
			return false
		}

		desk.stops.Store(symbol, stopLoss)
		return desk.positions.ApplyStop(stopLoss)
	}

	stopLoss, ok := raw.(*StopLoss)

	if !ok || stopLoss == nil {
		return false
	}

	stopChanged := false

	if stopLoss.Quantity != quantity {
		stopLoss.Quantity = quantity
		stopChanged = true
	}

	if stopLoss.EntryPrice != entryPrice {
		stopLoss.EntryPrice = entryPrice
		offset := stopLoss.Offset

		if offset <= 0 {
			offset = DeriveTrailOffset(0, 0)
		}

		stopLoss.HardStopPrice = entryPrice * (1 - DeriveMaxInitialRisk(offset, 0))

		if stopLoss.PeakPrice < entryPrice {
			stopLoss.PeakPrice = entryPrice
		}

		trailStop := stopLoss.PeakPrice * (1 - offset)
		stopLoss.StopPrice = effectiveStopPrice(
			entryPrice,
			trailStop,
			stopLoss.HardStopPrice,
		)
		stopChanged = true
	}

	if !stopChanged {
		return false
	}

	return desk.positions.ApplyStop(stopLoss)
}
