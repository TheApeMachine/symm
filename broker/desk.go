package broker

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

const defaultHaltCooldown = 5 * time.Minute

type Desk struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q
	quotes       *QuoteCache
	stress       *StressCache
	rules        *InstrumentRulesCache
	halted       atomic.Bool
	haltedAt     atomic.Pointer[time.Time]
	haltCooldown time.Duration
	err          error
}

func NewDesk(ctx context.Context, pool *qpool.Q) (*Desk, error) {
	return NewDeskWithCaches(ctx, pool, EnsureQuoteCache(ctx, pool), EnsureStressCache(ctx, pool))
}

/*
NewDeskWithCaches builds a desk with explicit quote and stress dependencies.
*/
func NewDeskWithCaches(
	ctx context.Context,
	pool *qpool.Q,
	quotes *QuoteCache,
	stress *StressCache,
) (*Desk, error) {
	return NewDeskWithAllCaches(ctx, pool, quotes, stress, EnsureInstrumentRulesCache(ctx, pool))
}

/*
NewDeskWithAllCaches builds a desk with explicit quote, stress, and instrument caches.
*/
func NewDeskWithAllCaches(
	ctx context.Context,
	pool *qpool.Q,
	quotes *QuoteCache,
	stress *StressCache,
	rules *InstrumentRulesCache,
) (*Desk, error) {
	if quotes == nil {
		return nil, fmt.Errorf("broker desk: quote cache is required")
	}

	if stress == nil {
		return nil, fmt.Errorf("broker desk: stress cache is required")
	}

	if rules == nil {
		return nil, fmt.Errorf("broker desk: instrument rules cache is required")
	}

	ctx, cancel := context.WithCancel(ctx)

	desk := &Desk{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		quotes:       quotes,
		stress:       stress,
		rules:        rules,
		haltCooldown: defaultHaltCooldown,
	}

	return desk, nil
}

func (desk *Desk) Halted() bool {
	if !desk.halted.Load() {
		return false
	}

	haltTime := desk.haltedAt.Load()

	if haltTime != nil && desk.haltCooldown > 0 && time.Since(*haltTime) >= desk.haltCooldown {
		desk.ResetHalt()

		return false
	}

	return true
}

/*
TripHalt trips the order circuit breaker and cancels all resting exchange orders.
*/
func (desk *Desk) TripHalt() {
	if !desk.halted.CompareAndSwap(false, true) {
		return
	}

	now := time.Now()
	desk.haltedAt.Store(&now)

	if desk.pool == nil {
		return
	}

	_ = trading.NewOrderClient(desk.ctx, desk.pool).CancelAll(trading.CancelAllParams{})
}

/*
ResetHalt clears the order circuit breaker so the desk accepts orders again.
*/
func (desk *Desk) ResetHalt() {
	desk.halted.Store(false)
	desk.haltedAt.Store(nil)
}

func (desk *Desk) AddOrder(action reasoning.Action) (reasoning.Action, error) {
	if desk.Halted() {
		return action, fmt.Errorf("order circuit breaker tripped")
	}

	orderType, err := reasoning.OrderTypeFromActionType(action.Type)

	if err != nil {
		return action, err
	}

	quote, ok := desk.quotes.Snapshot(action.Symbol)

	if !ok {
		return action, fmt.Errorf("preflight: no quote for %s", action.Symbol)
	}

	stress := desk.stress.Snapshot(action.Symbol)
	action, err = resolveAction(action, quote, stress)

	if err != nil {
		return action, err
	}

	if reasoning.IsEntryAction(action.Type) {
		action.Quantity, action.Price, err = desk.rules.PrepareEntryOrder(
			action.Symbol,
			action.Quantity,
			action.Price,
			orderType,
		)
	} else {
		action.Quantity, action.Price, err = desk.rules.PrepareOrder(
			action.Symbol,
			action.Quantity,
			action.Price,
			orderType,
		)
	}

	if err != nil {
		return action, err
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
		return action, err
	}

	if orderType == trading.Limit && reasoning.IsMakerAction(action.Type) {
		if WouldCrossPostOnly(quote, action.Side, action.Price) {
			return action, fmt.Errorf(
				"preflight: post-only limit would cross for %s",
				action.Symbol,
			)
		}
	}

	clOrdID := desk.NextClOrdID()
	triggers, err := desk.triggersFor(action, quote)

	if err != nil {
		return action, err
	}

	addParams := trading.AddParams{
		OrderType: orderType,
		Side:      action.Side,
		Symbol:    action.Symbol,
		OrderQty:  action.Quantity,
		ClOrdID:   clOrdID,
		Triggers:  triggers,
	}

	// A protective market exit rests at the level carried in Triggers; a protective
	// limit exit also needs a concrete limit price when the trigger fires. Plain
	// entries/exits post their limit/reference price directly.
	if !reasoning.IsProtectiveExit(action.Type) {
		addParams.LimitPrice = action.Price
	} else if reasoning.ExitRestsAsLimit(action.Type) &&
		!reasoning.IsTrailingExit(action.Type) &&
		triggers != nil {
		addParams.LimitPrice = triggers.Price
	}

	if reasoning.IsMakerAction(action.Type) {
		addParams.PostOnly = true
	}

	if addErr := trading.NewOrderClient(desk.ctx, desk.pool).AddOrder(addParams); addErr != nil {
		return action, addErr
	}

	return action, nil
}

/*
ResolveAction applies live quote and stress data to an action before submission.
*/
func (desk *Desk) ResolveAction(action reasoning.Action) (reasoning.Action, error) {
	quote, ok := desk.quotes.Snapshot(action.Symbol)

	if !ok {
		return action, fmt.Errorf("preflight: no quote for %s", action.Symbol)
	}

	return resolveAction(action, quote, desk.stress.Snapshot(action.Symbol))
}

func resolveAction(
	action reasoning.Action,
	quote Quote,
	stress SymbolStress,
) (reasoning.Action, error) {
	if reasoning.IsEntryAction(action.Type) {
		action.Quantity = stress.EntryQuantity(action.Quantity)
	}

	if !reasoning.IsProtectiveExit(action.Type) {
		return action, nil
	}

	offset, err := triggerOffset(action, quote)

	if err != nil {
		return action, err
	}

	action.Offset = offset

	return action, nil
}

/*
triggersFor builds the Kraken trigger params for a protective exit, or nil for an
ordinary order. Stop/take rest at a fixed level measured from the entry price
(action.Price); a trailing stop carries a negative-percent offset and Kraken (or the
paper emulator) trails it from the market price at placement. The per-node offset
overrides the account default. Returns nil for entries and immediate settles.
*/
func (desk *Desk) triggersFor(
	action reasoning.Action,
	quote Quote,
) (*trading.Triggers, error) {
	if !reasoning.IsProtectiveExit(action.Type) {
		return nil, nil
	}

	offset, err := triggerOffset(action, quote)

	if err != nil {
		return nil, err
	}

	if reasoning.IsTrailingExit(action.Type) {
		return &trading.Triggers{Reference: "last", PriceType: "pct", Price: -offset * 100}, nil
	}

	positionSide := action.Side
	if reasoning.IsExitAction(action.Type) {
		if action.Side == trading.Buy {
			positionSide = trading.Sell
		} else {
			positionSide = trading.Buy
		}
	}

	return &trading.Triggers{
		Reference: "last",
		Price:     reasoning.ProtectiveLevelForSide(positionSide, action.Type, action.Price, 0, offset),
	}, nil
}

func triggerOffset(action reasoning.Action, quote Quote) (float64, error) {
	if action.Offset > 0 && action.Offset < 1 {
		return action.Offset, nil
	}

	if reasoning.IsTrailingExit(action.Type) {
		return dynamicTrailingOffset(quote)
	}

	switch action.Type {
	case reasoning.ActionStopLoss, reasoning.ActionStopLossLimit:
		return requiredPercent("trading.exit.stop_loss_pct")
	case reasoning.ActionTakeProfit, reasoning.ActionTakeProfitLimit:
		return requiredPercent("trading.exit.take_profit_pct")
	default:
		return 0, nil
	}
}

func dynamicTrailingOffset(quote Quote) (float64, error) {
	multiple, err := market.RequiredFloat("trading.exit.trailing_volatility_multiple")

	if err != nil {
		return 0, fmt.Errorf("preflight: %w", err)
	}

	if quote.Volatility <= 0 {
		return 0, fmt.Errorf(
			"preflight: trailing stop needs realized volatility for %s",
			quote.Symbol,
		)
	}

	return quote.Volatility * multiple, nil
}

func requiredPercent(key string) (float64, error) {
	percent, err := market.RequiredFloat(key)

	if err != nil {
		return 0, fmt.Errorf("preflight: %w", err)
	}

	return percent / 100, nil
}

var clOrdCounter atomic.Uint64

func (desk *Desk) NextClOrdID() string {
	return fmt.Sprintf("s%016x", clOrdCounter.Add(1))
}

func (desk *Desk) Close() error {
	desk.cancel()

	return desk.err
}
