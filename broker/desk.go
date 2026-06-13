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
	quoteBooks    *sync.Map
	positions     *PositionMonitor
	exitConfig    *ExitConfigStream
	riskGate      PreTradeRiskGate
	metrics       *observability.OperationalMetrics
	marginEnabled bool
	tradingModel  string
}

func NewDesk(
	ctx context.Context, pool *qpool.Q[any],
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
		quoteBooks:    &sync.Map{},
		positions:     NewPositionMonitor(),
		exitConfig:    exitConfigStream,
		riskGate:      riskGate,
		metrics:       observability.Shared(),
		marginEnabled: marketConfig.MarginEnabled,
		tradingModel:  tradingConfig.Model,
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

			if desk.positions.ApplyBalance(balances) {
				desk.publishPositions()
			}
		}
	}
}

func (desk *Desk) onTicker(ticker *market.TickerUpdate) {
	if ticker == nil || ticker.Symbol == "" {
		return
	}

	desk.storeQuote(ticker)
	desk.recordTickerAge(ticker)

	if desk.touchPositionPrices(ticker) {
		desk.publishPositions()
	}

	desk.evaluateStop(ticker)
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
	bookState := desk.symbolBook(book.Symbol)
	bookState.applyBookUpdate(book)
	desk.recordBookAge(book, observedAt)

	snapshot, ok := bookState.quoteSnapshot(book.Symbol, observedAt)

	if !ok {
		return
	}

	desk.persistQuote(snapshot)

	snapshotTicker := quoteSnapshotTicker(snapshot)

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
	bookState := desk.symbolBook(trade.Symbol)
	bookState.applyTrade(trade)
	desk.recordTradeAge(trade, observedAt)

	snapshot, ok := bookState.quoteSnapshot(trade.Symbol, observedAt)

	if !ok {
		quote, quoteErr := desk.loadQuote(trade.Symbol, observedAt)

		if quoteErr != nil {
			return
		}

		snapshot = quote

		if trade.Price > 0 {
			snapshot.Last = trade.Price
		}

		snapshot.ObservedAt = observedAt
	}

	desk.persistQuote(snapshot)

	snapshotTicker := quoteSnapshotTicker(snapshot)

	if desk.touchPositionPrices(snapshotTicker) {
		desk.publishPositions()
	}

	desk.evaluateStop(snapshotTicker)
}

func (desk *Desk) symbolBook(symbol string) *symbolBook {
	raw, ok := desk.quoteBooks.Load(symbol)

	if ok {
		book, bookOK := raw.(*symbolBook)

		if bookOK {
			return book
		}
	}

	book := newSymbolBook()
	actual, _ := desk.quoteBooks.LoadOrStore(symbol, book)
	loaded, loadOK := actual.(*symbolBook)

	if loadOK {
		return loaded
	}

	return book
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
		fillPrice,
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
