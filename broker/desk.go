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

	raw, ok := desk.stops.Load(ticker.Symbol)

	if !ok {
		if desk.positions.ApplyTicker(ticker) {
			desk.publishPositions()
		}

		return
	}

	stopLoss, stopOK := raw.(*StopLoss)

	if !stopOK || stopLoss == nil {
		return
	}

	stopLoss.WidenOffsetFromTicker(ticker, desk.exitConfig.Load())

	if _, ratchetErr := stopLoss.Ratchet(ticker); errnie.Error(ratchetErr) != nil {
		return
	}

	if desk.positions.ApplyStopTicker(stopLoss, ticker) {
		desk.publishPositions()
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
