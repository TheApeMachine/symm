package broker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/rawbus"
)

type Desk struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q[any]
	bus           *internal.Bus
	actions       *sync.Map
	stops         *sync.Map
	marginEnabled bool
}

func NewDesk(
	ctx context.Context, pool *qpool.Q[any],
) *Desk {
	ctx, cancel := context.WithCancel(ctx)
	marketConfig, _ := config.LoadMarketConfig()
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
		stops:         &sync.Map{},
		marginEnabled: marketConfig.MarginEnabled,
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
		case rawbus.TypeOrder, rawbus.TypeActions:
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
		}
	}
}

func (desk *Desk) onTicker(ticker *market.TickerUpdate) {
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

	stopLoss.WidenOffsetFromTicker(ticker)

	if _, ratchetErr := stopLoss.Ratchet(ticker); errnie.Error(ratchetErr) != nil {
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

	desk.stops.Delete(ticker.Symbol)
	stopLoss.Close()

	desk.sendMarketOrder(trading.Sell, stopLoss.Symbol, quantity)
}

func (desk *Desk) onAction(action *logic.Action) {
	if action.Type.IsExit() {
		desk.stops.Delete(action.Symbol)
	}

	clOrdID := uuid.New().String()

	orderType, err := krakenOrderType(action, desk.marginEnabled)

	if err != nil {
		errnie.Error(err)
		return
	}

	params := trading.AddParams{
		ClOrdID:    clOrdID,
		Symbol:     action.Symbol,
		Side:       action.Side,
		OrderQty:   action.Quantity,
		LimitPrice: action.Price,
		OrderType:  orderType,
	}

	if action.Offset > 0 && isTriggeredOrderType(orderType) {
		params.Triggers = &trading.Triggers{
			Price:     action.Offset,
			PriceType: "pct",
		}
	}

	if !action.Type.IsExit() {
		params.EntryQueuedAt = time.Now().UTC()
	}

	desk.actions.Store(clOrdID, action)

	errnie.Error(desk.bus.Send(internal.ChannelKrakenPrivate, "orders", types.KrakenMessage{
		Method: trading.MethodAddOrder,
		Params: params,
		ReqID:  time.Now().UnixNano(),
	}))
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

	desk.actions.Delete(orderKey)

	if execution.ExecType != "trade" && execution.OrderStatus != "filled" {
		return
	}

	fillPrice := execution.LastPrice

	if fillPrice <= 0 {
		fillPrice = execution.AvgPrice
	}

	fillQty := execution.LastQty

	if fillQty <= 0 {
		fillQty = execution.CumQty
	}

	if fillQty <= 0 {
		fillQty = action.Quantity
	}

	if action.Type.IsExit() || execution.Side == string(trading.Sell) {
		desk.stops.Delete(execution.Symbol)

		return
	}

	if fillPrice <= 0 || fillQty <= 0 {
		return
	}

	stopLoss, stopErr := NewStopLoss(
		execution.Symbol,
		fillQty,
		fillPrice,
		0,
	)

	if errnie.Error(stopErr) != nil {
		return
	}

	desk.stops.Store(execution.Symbol, stopLoss)
}

func (desk *Desk) sendMarketOrder(
	side trading.Side, symbol string, quantity float64,
) {
	if symbol == "" || quantity <= 0 {
		return
	}

	action := &logic.Action{
		Type:     logic.ActionMarket,
		Side:     side,
		Symbol:   symbol,
		Quantity: quantity,
	}

	errnie.Error(rawbus.Send(desk.bus, rawbus.TypeOrder, action))

	clOrdID := uuid.New().String()

	desk.actions.Store(clOrdID, action)

	errnie.Error(desk.bus.Send(internal.ChannelKrakenPrivate, "orders", types.KrakenMessage{
		Method: trading.MethodAddOrder,
		Params: trading.AddParams{
			ClOrdID:   clOrdID,
			Symbol:    symbol,
			Side:      side,
			OrderQty:  quantity,
			OrderType: trading.Market,
		},
		ReqID: time.Now().UnixNano(),
	}))
}

func krakenOrderType(action *logic.Action, marginEnabled bool) (trading.OrderType, error) {
	switch action.Type {
	case logic.ActionMarket:
		return trading.Market, nil
	case logic.ActionLimit:
		return trading.Limit, nil
	case logic.ActionIceberg:
		return trading.Iceberg, nil
	case logic.ActionStopLoss:
		return trading.StopLoss, nil
	case logic.ActionStopLossLimit:
		return trading.StopLossLimit, nil
	case logic.ActionTakeProfit:
		return trading.TakeProfit, nil
	case logic.ActionTakeProfitLimit:
		return trading.TakeProfitLimit, nil
	case logic.ActionTrailingStop:
		return trading.TrailingStop, nil
	case logic.ActionTrailingStopLimit:
		return trading.TrailingStopLimit, nil
	case logic.ActionSettlePosition:
		orderType := trading.SettlePosition

		if !marginEnabled {
			return trading.Market, nil
		}

		return orderType, nil
	default:
		return "", fmt.Errorf("broker: unknown action type %q", action.Type)
	}
}

func (desk *Desk) Close() error {
	desk.cancel()
	return nil
}

func isTriggeredOrderType(orderType trading.OrderType) bool {
	switch orderType {
	case trading.StopLoss, trading.StopLossLimit,
		trading.TakeProfit, trading.TakeProfitLimit,
		trading.TrailingStop, trading.TrailingStopLimit:
		return true
	default:
		return false
	}
}
