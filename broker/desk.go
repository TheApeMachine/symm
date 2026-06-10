package broker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type Desk struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q[any]
	bus         *internal.Bus
	ledger      *Ledger
	treeStats   *logic.TreeStats
	orders      *OrderRegistry
	audit       *audit.Writer
	deadLetter  *DeadLetter
	gate        PreTradeGate
	instruments *krakenmarket.InstrumentRegistry
	regime      *market.RegimeClassifier
}

func NewDesk(
	ctx context.Context,
	pool *qpool.Q[any],
	ledger *Ledger,
	treeStats *logic.TreeStats,
	auditWriter *audit.Writer,
	instruments *krakenmarket.InstrumentRegistry,
	regime *market.RegimeClassifier,
) *Desk {
	ctx, cancel := context.WithCancel(ctx)

	return &Desk{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		bus: internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelKrakenPrivate, internal.ChannelUI},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelRaw, "desk"),
			},
		),
		ledger:      ledger,
		treeStats:   treeStats,
		orders:      NewOrderRegistry(),
		audit:       auditWriter,
		deadLetter:  NewDeadLetter(auditWriter),
		instruments: instruments,
		regime:      regime,
	}
}

func (desk *Desk) Tick() error {
	for {
		if errnie.Error(desk.ctx.Err()) != nil {
			return desk.ctx.Err()
		}

		for _, clOrdID := range desk.orders.RejectStaleEntries() {
			desk.orders.Delete(clOrdID)
		}

		message, err := desk.bus.Receive(internal.ChannelRaw)

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
		case "order":
			action, ok := message.Value.(*logic.Action)

			if !ok {
				desk.deadLetter.Record("order", "invalid action payload", map[string]any{
					"message_type": message.Type,
				})
				errnie.Error(errors.New("desk: invalid order action"))
				continue
			}

			if submitErr := desk.submitAction(action); submitErr != nil {
				desk.deadLetter.Record("order", submitErr.Error(), desk.actionDetail(action))
				errnie.Error(submitErr)
				continue
			}
		case "orders":
			updates, ok := message.Value.([]trading.OrderUpdate)

			if !ok {
				desk.deadLetter.Record("orders", "invalid payload", nil)
				errnie.Error(errors.New("desk: invalid orders"))
				continue
			}

			for _, update := range updates {
				desk.orders.Delete(update.OrderID)
			}
		case "executions":
			updates, ok := message.Value.([]user.Execution)

			if !ok {
				desk.deadLetter.Record("executions", "invalid payload", nil)
				errnie.Error(errors.New("desk: invalid executions"))
				continue
			}

			for _, execution := range updates {
				if execution.ClOrdID != "" {
					desk.orders.MarkFilled(execution.ClOrdID)
					desk.orders.Delete(execution.ClOrdID)
				}

				if execution.ExecType == "trade" && execution.LastQty > 0 {
					desk.publishDecision(
						&logic.Action{
							Type:      logic.ActionMarket,
							Symbol:    execution.Symbol,
							BranchKey: execution.ClOrdID,
						},
						"filled",
						"",
					)
				}
			}
		default:
			desk.deadLetter.Record("bus", "unknown message type", map[string]any{
				"message_type": message.Type,
			})
		}
	}
}

func (desk *Desk) actionDetail(action *logic.Action) map[string]any {
	if action == nil {
		return nil
	}

	return map[string]any{
		"symbol":   action.Symbol,
		"type":     action.Type.String(),
		"side":     string(action.Side),
		"fraction": action.Fraction,
	}
}

func (desk *Desk) submitAction(action *logic.Action) error {
	if action == nil {
		return errnie.Error(errors.New("desk: nil action"))
	}

	risk := CurrentRiskContext()
	heldQty := desk.ledger.HeldQuantity(action.Symbol)

	if action.Type.IsExit() && heldQty <= 0 {
		desk.publishDecision(action, "rejected", "no open position")
		return errnie.Error(ErrNoPosition)
	}

	if !action.Type.IsExit() && desk.ledger.OpenCount() >= risk.MaxConcurrentPositions {
		if heldQty <= 0 {
			desk.publishDecision(action, "rejected", "max concurrent positions")

			return errnie.Error(errors.New("desk: max concurrent positions"))
		}
	}

	mark, markOK := desk.ledger.Mark(action.Symbol)

	if !markOK {
		desk.publishDecision(action, "rejected", "no mark price")
		return errnie.Error(ErrNoMark)
	}

	quote, quoteOK := desk.ledger.Quote(action.Symbol)

	if !quoteOK {
		desk.publishDecision(action, "rejected", "no quote")
		return errnie.Error(ErrNoMark)
	}

	if gateErr := desk.gate.CheckEntry(action, risk, quote); gateErr != nil {
		desk.publishDecision(action, "rejected", gateErr.Error())
		return errnie.Error(gateErr)
	}

	constraints, hasConstraints := desk.instrumentConstraints(action.Symbol)

	if !action.Type.IsExit() && !hasConstraints {
		desk.publishDecision(action, "rejected", "no instrument constraints")
		return errnie.Error(fmt.Errorf("desk: no instrument constraints for %s", action.Symbol))
	}

	var constraintsPtr *krakenmarket.InstrumentConstraints

	if hasConstraints {
		constraintsPtr = &constraints
	}

	entryScale := 1.0

	if !action.Type.IsExit() && desk.regime != nil {
		entryScale = risk.EntryScaleForRegime(desk.regime.MarketMean())
	}

	quantity, sizeErr := SizeOrder(
		action,
		risk,
		entryScale,
		desk.ledger.QuoteCash(),
		heldQty,
		mark,
		constraintsPtr,
	)

	if sizeErr != nil {
		desk.publishDecision(action, "rejected", sizeErr.Error())
		return errnie.Error(sizeErr)
	}

	token, tokenErr := types.NewToken(desk.ctx)

	if tokenErr != nil {
		desk.publishDecision(action, "rejected", tokenErr.Error())
		return errnie.Error(tokenErr)
	}

	clOrdID := uuid.New().String()

	params, buildErr := BuildAddOrder(
		action,
		OrderContext{Mark: mark},
		quantity,
		clOrdID,
		token,
		constraintsPtr,
	)

	if buildErr != nil {
		desk.publishDecision(action, "rejected", buildErr.Error())
		return errnie.Error(buildErr)
	}

	if !action.Type.IsExit() {
		params.EntryQueuedAt = time.Now().UTC()

		if transitErr := RejectStaleEntry(params); transitErr != nil {
			desk.publishDecision(action, "rejected", transitErr.Error())
			return errnie.Error(transitErr)
		}
	}

	frame := types.KrakenMessage{
		Method: trading.MethodAddOrder,
		Params: params,
		ReqID:  time.Now().UnixNano(),
	}

	desk.orders.Store(clOrdID, frame)
	desk.publishDecision(action, "submitted", "")

	return errnie.Error(desk.bus.Send(internal.ChannelKrakenPrivate, "orders", frame))
}

func (desk *Desk) instrumentConstraints(symbol string) (krakenmarket.InstrumentConstraints, bool) {
	if desk.instruments == nil {
		return krakenmarket.InstrumentConstraints{}, false
	}

	return desk.instruments.Constraints(symbol)
}

func (desk *Desk) publishDecision(
	action *logic.Action,
	verdict string,
	reason string,
) {
	if action == nil {
		desk.deadLetter.Record("decision", "nil action", nil)
		return
	}

	if desk.treeStats != nil {
		desk.treeStats.RecordAction(
			action.Symbol,
			&logic.Evaluation{Action: action, Key: action.BranchKey},
			verdict,
			reason,
		)
	}

	errnie.Error(desk.bus.Send(internal.ChannelUI, "decision", map[string]any{
		"event":    "decision",
		"type":     action.Type.String(),
		"symbol":   action.Symbol,
		"key":      action.BranchKey,
		"verdict":  verdict,
		"reason":   reason,
		"chart":    "decision_tree",
		"decision": true,
	}))

	desk.recordDeskDecision(action, verdict, reason)
}

func (desk *Desk) recordDeskDecision(
	action *logic.Action,
	verdict string,
	reason string,
) {
	if desk.audit == nil || action == nil {
		return
	}

	frame := audit.DeskDecisionFrame{
		RecordedAt: time.Now().UTC(),
		Symbol:     action.Symbol,
		ActionType: action.Type.String(),
		Side:       string(action.Side),
		BranchKey:  action.BranchKey,
		Verdict:    verdict,
		Reason:     reason,
	}

	if verdict == "rejected" && reason != "" {
		errnie.Error(desk.audit.EnqueueDedupedFrame(frame))

		return
	}

	errnie.Error(desk.audit.EnqueueFrame(frame))
}

func (desk *Desk) CheckOrder(orderID string) error {
	frame, ok := desk.orders.Load(orderID)

	if !ok {
		return errnie.Error(fmt.Errorf("desk: order not found: %s", orderID))
	}

	return errnie.Error(desk.bus.Send(internal.ChannelKrakenPrivate, "orders", frame))
}

func (desk *Desk) Close() error {
	desk.cancel()
	return nil
}
