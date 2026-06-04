package broker

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/spf13/viper"
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
		OrderType: orderType,
		Side:      action.Side,
		Symbol:    action.Symbol,
		OrderQty:  action.Quantity,
		ClOrdID:   clOrdID,
		Triggers:  triggersFor(action),
	}

	// A protective exit rests at the level carried in Triggers; a plain entry/exit
	// posts its limit/reference price.
	if !perspectives.IsProtectiveExit(action.Type) {
		addParams.LimitPrice = action.Price
	}

	if perspectives.IsMakerAction(action.Type) {
		addParams.PostOnly = true
	}

	if addErr := trading.NewOrderClient(desk.ctx, desk.pool).AddOrder(addParams); addErr != nil {
		return addErr
	}

	return nil
}

/*
triggersFor builds the Kraken trigger params for a protective exit, or nil for an
ordinary order. Stop/take rest at a fixed level measured from the entry price
(action.Price); a trailing stop carries a negative-percent offset and Kraken (or the
paper emulator) trails it from the market price at placement. The per-node offset
overrides the account default. Returns nil for entries and immediate settles.
*/
func triggersFor(action perspectives.Action) *trading.Triggers {
	if !perspectives.IsProtectiveExit(action.Type) {
		return nil
	}

	offset := perspectives.TriggerOffset(action.Offset, globalTriggerOffset(action.Type))

	if perspectives.IsTrailingExit(action.Type) {
		return &trading.Triggers{Reference: "last", PriceType: "pct", Price: -offset * 100}
	}

	return &trading.Triggers{
		Reference: "last",
		Price:     perspectives.ProtectiveLevel(action.Type, action.Price, 0, offset),
	}
}

// globalTriggerOffset is the account-default trigger distance for an exit type,
// read from config (percent) when a playbook node carries no per-node offset.
func globalTriggerOffset(action perspectives.ActionType) float64 {
	switch action {
	case perspectives.ActionStopLoss, perspectives.ActionStopLossLimit:
		return viperPercentDefault("trading.exit.stop_loss_pct", 2.0)
	case perspectives.ActionTakeProfit, perspectives.ActionTakeProfitLimit:
		return viperPercentDefault("trading.exit.take_profit_pct", 3.0)
	case perspectives.ActionTrailingStop, perspectives.ActionTrailingStopLimit:
		return viperPercentDefault("trading.exit.trailing_pct", 1.5)
	default:
		return 0
	}
}

func viperPercentDefault(key string, fallbackPercent float64) float64 {
	percent := viper.GetFloat64(key)

	if percent <= 0 {
		percent = fallbackPercent
	}

	return percent / 100
}

var clOrdCounter atomic.Uint64

func (desk *Desk) NextClOrdID() string {
	return fmt.Sprintf("s%016x", clOrdCounter.Add(1))
}

func (desk *Desk) Close() error {
	desk.cancel()

	return desk.err
}
