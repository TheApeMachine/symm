package broker

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

type Desk struct {
	ctx    context.Context
	cancel context.CancelFunc
	pool   *qpool.Q
	quotes *QuoteCache
	stress *StressCache
	err    error
}

func NewDesk(ctx context.Context, pool *qpool.Q) (*Desk, error) {
	ctx, cancel := context.WithCancel(ctx)

	desk := &Desk{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		quotes: EnsureQuoteCache(ctx, pool),
		stress: EnsureStressCache(ctx, pool),
	}

	return desk, nil
}

func (desk *Desk) Halted() bool {
	return false
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

	stress := desk.stress.Snapshot(action.Symbol)

	if perspectives.IsEntryAction(action.Type) {
		if stress.RejectsDiscretionaryEntry() {
			return fmt.Errorf(
				"preflight: toxic regime blocks discretionary entry for %s",
				action.Symbol,
			)
		}

		if stress.DeskRegimeForStress() == DeskRegimeRestricted {
			if orderType != trading.Limit {
				return fmt.Errorf(
					"preflight: restricted desk rejects aggressive entry during turbulence for %s",
					action.Symbol,
				)
			}

			if !perspectives.IsMakerAction(action.Type) {
				return fmt.Errorf(
					"preflight: restricted desk requires post-only limits for %s",
					action.Symbol,
				)
			}
		}
	}

	request := PreflightRequest{
		Quote:      quote,
		Side:       action.Side,
		Quantity:   action.Quantity,
		OrderType:  orderType,
		ActionType: action.Type,
		Stress:     stress,
	}

	if err := PreflightGates(request); err != nil {
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

	addErr := trading.NewOrderClient(desk.ctx, desk.pool).AddOrder(addParams)

	if addErr != nil {
		return err
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
