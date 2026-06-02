package broker

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

type Desk struct {
	ctx    context.Context
	cancel context.CancelFunc
	pool   *qpool.Q
	orders *trading.Client
	quotes *QuoteCache
	err    error
}

func NewDesk(ctx context.Context, pool *qpool.Q) (*Desk, error) {
	ctx, cancel := context.WithCancel(ctx)

	orders, err := trading.NewOrder(ctx, pool)

	if err != nil {
		cancel()
		return nil, fmt.Errorf("broker desk: order client: %w", err)
	}

	desk := &Desk{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		orders: orders,
		quotes: EnsureQuoteCache(ctx, pool),
	}

	return desk, nil
}

func (desk *Desk) Halted() bool {
	if desk.orders == nil {
		return true
	}

	return desk.orders.Halted()
}

func (desk *Desk) AddOrder(action perspectives.Action) error {
	if desk.Halted() {
		return fmt.Errorf("order circuit breaker tripped")
	}

	orderType, err := perspectives.OrderTypeFromActionType(action.Type)

	if err != nil {
		return err
	}

	quote, ok := desk.quotes.Snapshot(action.Symbol)

	if !ok {
		return fmt.Errorf("preflight: no quote for %s", action.Symbol)
	}

	if err := PreflightGates(quote, action.Side, action.Quantity, orderType); err != nil {
		return err
	}

	if orderType == trading.Limit && perspectives.IsMakerAction(action.Type) {
		if WouldCrossPostOnly(quote, action.Side, action.Price) {
			return fmt.Errorf(
				"preflight: post-only limit would cross for %s",
				action.Symbol,
			)
		}
	}

	clOrdID := desk.NextClOrdID()

	addParams := trading.AddParams{
		OrderType:  orderType,
		Side:       action.Side,
		Symbol:     action.Symbol,
		LimitPrice: action.Price,
		OrderQty:   action.Quantity,
		ClOrdID:    clOrdID,
		Triggers:   &trading.Triggers{},
	}

	if perspectives.IsMakerAction(action.Type) {
		addParams.PostOnly = true
	}

	resultCh, err := desk.orders.AddOrder(addParams)

	if err != nil {
		return err
	}

	defer desk.orders.ReleaseOrderResult(resultCh)

	var result trading.OrderResult

	select {
	case result = <-resultCh:
	case <-desk.ctx.Done():
		return fmt.Errorf("order cancelled: %w", desk.ctx.Err())
	case <-time.After(trading.AckTimeout()):
		return fmt.Errorf("order %s ack timeout", clOrdID)
	}

	if !result.Success {
		if result.Error != "" {
			return fmt.Errorf(
				"order %s rejected: %s",
				result.ClOrdID,
				result.Error,
			)
		}

		return fmt.Errorf("order %s rejected", result.ClOrdID)
	}

	return nil
}

var clOrdCounter atomic.Uint64

func (desk *Desk) NextClOrdID() string {
	return fmt.Sprintf("s%016x", clOrdCounter.Add(1))
}

func (desk *Desk) Close() error {
	desk.cancel()

	return desk.err
}
