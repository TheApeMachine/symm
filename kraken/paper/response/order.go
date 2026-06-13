package response

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
)

var errMissingFillBook = errors.New("paper orders: pair catalog required for market fill")

type marketFillQuote struct {
	fillPrice float64
	exitPrice float64
}

type Orders struct {
	ctx              context.Context
	cancel           context.CancelFunc
	err              error
	pool             *qpool.Q[any]
	isActive         atomic.Bool
	model            map[string]trading.OrderUpdate
	restingTriggered map[string]restingTriggeredOrder
	executions       map[string]user.Execution
	pendingExec      []user.Execution
	observers        []types.Socket
	fillHandler      *Balances
	catalog          *PairCatalog
	bookDepthLevels  int
}

func NewOrders(
	ctx context.Context,
	pool *qpool.Q[any],
	catalog *PairCatalog,
) (*Orders, error) {
	ctx, cancel := context.WithCancel(ctx)
	marketConfig, marketErr := config.LoadMarketConfig()

	if marketErr != nil {
		cancel()
		return nil, marketErr
	}

	return &Orders{
		ctx:              ctx,
		cancel:           cancel,
		err:              nil,
		pool:             pool,
		model:            make(map[string]trading.OrderUpdate),
		restingTriggered: make(map[string]restingTriggeredOrder),
		executions:       make(map[string]user.Execution),
		observers:        make([]types.Socket, 0),
		catalog:          catalog,
		bookDepthLevels:  marketConfig.BookDepthLevels,
	}, nil
}

func (orders *Orders) Send(message *qpool.QValue[any]) *types.SocketMessage {
	frame, ok := message.Value.(types.KrakenMessage)

	if !ok {
		return nil
	}

	switch frame.Method {
	case "subscribe":
		orders.isActive.Store(true)
	case "unsubscribe":
		orders.isActive.Store(false)
	case trading.MethodAddOrder:
		var params trading.AddParams

		switch typed := frame.Params.(type) {
		case trading.AddParams:
			params = typed
		case *trading.AddParams:
			params = *typed
		default:
			return nil
		}

		if params.ClOrdID == "" {
			return nil
		}

		// Live Kraken rests these natively; the paper book has no quote loop
		// yet, so accepting them would park an exit that never fires. Refuse
		// loudly instead of silently absorbing protective orders.
		if isTriggeredOrderType(params.OrderType) {
			orders.parkTriggeredOrder(params)
			break
		}

		orders.model[params.ClOrdID] = trading.OrderUpdate{
			OrderID: params.ClOrdID,
		}

		if params.OrderType == trading.Market && params.OrderQty > 0 {
			orders.fillMarket(params)
		}
	case trading.MethodCancelOrder:
		var params trading.CancelParams

		switch typed := frame.Params.(type) {
		case trading.CancelParams:
			params = typed
		case *trading.CancelParams:
			params = *typed
		default:
			return nil
		}

		for _, orderID := range params.OrderID {
			delete(orders.model, orderID)
		}
	case trading.MethodAmendOrder:
		var params trading.AmendParams

		switch typed := frame.Params.(type) {
		case trading.AmendParams:
			params = typed
		case *trading.AmendParams:
			params = *typed
		default:
			return nil
		}

		if params.OrderID == "" {
			return nil
		}

		orders.model[params.OrderID] = trading.OrderUpdate{
			OrderID: params.OrderID,
		}
	}

	updates := make([]trading.OrderUpdate, 0, len(orders.model))

	for _, update := range orders.model {
		updates = append(updates, update)
	}

	data, err := sonic.Marshal(updates)

	if err != nil {
		return nil
	}

	out := &types.SocketMessage{
		Channel: "orders",
		Success: &[]bool{true}[0],
		Data:    data,
	}

	for _, observer := range orders.observers {
		observer.Send(&qpool.QValue[any]{Value: out})
	}

	return out
}

func (orders *Orders) fillMarket(params trading.AddParams) {
	if orders.fillHandler == nil {
		return
	}

	quote, marketErr := orders.marketFillQuote(
		params,
		orders.exitQuantity(params),
	)
	fillPrice := quote.fillPrice

	if marketErr != nil || fillPrice <= 0 {
		fillPrice = params.LimitPrice
	}

	if fillPrice <= 0 {
		delete(orders.model, params.ClOrdID)
		return
	}

	execution, fillErr := orders.fillHandler.ApplyFill(params, fillPrice)

	if fillErr != nil {
		delete(orders.model, params.ClOrdID)
		return
	}

	if params.Side == trading.Buy && quote.exitPrice > 0 {
		orders.fillHandler.UpdateTicker(&market.TickerUpdate{
			Symbol: params.Symbol,
			Bid:    quote.exitPrice,
		})
	}

	orders.executions[execution.ExecID] = execution
	orders.pendingExec = append(orders.pendingExec, execution)
	delete(orders.model, params.ClOrdID)

	balancePayload, err := orders.fillHandler.ModelJSON()

	if err != nil {
		return
	}

	balanceMessage := &types.SocketMessage{
		Channel: "balances",
		Success: &[]bool{true}[0],
		Data:    balancePayload,
	}

	for _, observer := range orders.observers {
		observer.Send(&qpool.QValue[any]{Value: balanceMessage})
	}
}

func (orders *Orders) marketFillQuote(
	params trading.AddParams,
	exitQuantity float64,
) (marketFillQuote, error) {
	if orders.catalog == nil {
		return marketFillQuote{}, errMissingFillBook
	}

	count := orders.bookDepthLevels

	if count <= 0 {
		return marketFillQuote{}, fmt.Errorf("paper orders: book depth must be positive")
	}

	book, bookErr := orders.catalog.DepthForSymbol(params.Symbol, count)

	if bookErr != nil {
		return marketFillQuote{}, bookErr
	}

	fillLevels := book.Asks
	exitLevels := book.Bids

	if params.Side == trading.Sell {
		fillLevels = book.Bids
		exitLevels = book.Asks
	}

	fillPrice, fillErr := depthVWAP(fillLevels, params.OrderQty)

	if fillErr != nil {
		return marketFillQuote{}, fillErr
	}

	exitPrice, _ := depthVWAP(exitLevels, exitQuantity)

	return marketFillQuote{
		fillPrice: fillPrice,
		exitPrice: exitPrice,
	}, nil
}

func (orders *Orders) exitQuantity(params trading.AddParams) float64 {
	if orders.fillHandler == nil || params.Side != trading.Buy {
		return params.OrderQty
	}

	wallet := orders.fillHandler.Wallet()
	base := baseAsset(params.Symbol)

	if base == "" {
		return params.OrderQty
	}

	return params.OrderQty + wallet.Inventory[base]
}

func depthVWAP(levels [][]any, quantity float64) (float64, error) {
	if quantity <= 0 {
		return 0, fmt.Errorf("paper orders: quantity must be positive")
	}

	remaining := quantity
	cost := 0.0
	filled := 0.0

	for _, level := range levels {
		if remaining <= 0 || len(level) < 2 {
			break
		}

		price, priceErr := depthFloat(level[0])

		if priceErr != nil {
			return 0, fmt.Errorf("paper orders: depth price: %w", priceErr)
		}

		qty, qtyErr := depthFloat(level[1])

		if qtyErr != nil {
			return 0, fmt.Errorf("paper orders: depth qty: %w", qtyErr)
		}

		take := qty

		if take > remaining {
			take = remaining
		}

		cost += take * price
		filled += take
		remaining -= take
	}

	if filled <= 0 {
		return 0, fmt.Errorf("paper orders: insufficient depth")
	}

	return cost / filled, nil
}

func depthFloat(value any) (float64, error) {
	switch typed := value.(type) {
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)

		if err != nil {
			return 0, err
		}

		return parsed, nil
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, fmt.Errorf("non-finite depth value")
		}

		return typed, nil
	default:
		return 0, fmt.Errorf("unsupported depth value %T", value)
	}
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

func (orders *Orders) Observe(sockets ...types.Socket) {
	for _, socket := range sockets {
		if balances, ok := socket.(*Balances); ok {
			orders.fillHandler = balances
		}

		orders.observers = append(orders.observers, socket)
	}
}

func (orders *Orders) DrainExecutions() []user.Execution {
	if len(orders.pendingExec) == 0 {
		return nil
	}

	rows := append([]user.Execution(nil), orders.pendingExec...)
	orders.pendingExec = orders.pendingExec[:0]

	return rows
}

func (orders *Orders) Wallet() user.Balances {
	if orders.fillHandler == nil {
		return user.Balances{}
	}

	return orders.fillHandler.Wallet()
}
