package broker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
)

type Desk struct {
	ctx       context.Context
	cancel    context.CancelFunc
	pool      *qpool.Q[any]
	bus       *internal.Bus
	ledger    *Ledger
	treeStats *logic.TreeStats
	orders    *sync.Map
}

func NewDesk(
	ctx context.Context,
	pool *qpool.Q[any],
	ledger *Ledger,
	treeStats *logic.TreeStats,
) *Desk {
	ctx, cancel := context.WithCancel(ctx)

	return &Desk{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		bus: internal.NewBus(
			ctx,
			pool,
			[]string{"kraken:private", "ui"},
			[]string{"raw"},
		),
		ledger:    ledger,
		treeStats: treeStats,
		orders:    &sync.Map{},
	}
}

func (desk *Desk) Tick() error {
	for {
		message, err := desk.bus.Receive("raw")

		if errnie.Error(err) != nil || message == nil {
			continue
		}

		switch message.Type {
		case "order":
			action, ok := message.Value.(*logic.Action)

			if !ok {
				errnie.Error(errors.New("desk: invalid order action"))
				continue
			}

			if errnie.Error(desk.submitAction(action)) != nil {
				continue
			}
		case "orders":
			updates, ok := message.Value.([]trading.OrderUpdate)

			if !ok {
				errnie.Error(errors.New("desk: invalid orders"))
				continue
			}

			for _, update := range updates {
				desk.orders.Delete(update.OrderID)
			}
		case "executions":
			updates, ok := message.Value.([]user.Execution)

			if !ok {
				errnie.Error(errors.New("desk: invalid executions"))
				continue
			}

			for _, execution := range updates {
				if execution.ClOrdID != "" {
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
		}
	}
}

func (desk *Desk) submitAction(action *logic.Action) error {
	if action == nil {
		return errnie.Error(errors.New("desk: nil action"))
	}

	orderType, typeErr := action.Type.KrakenOrderType()

	if typeErr != nil {
		desk.publishDecision(action, "rejected", typeErr.Error())
		return errnie.Error(typeErr)
	}

	heldQty := desk.ledger.HeldQuantity(action.Symbol)

	if action.Type.IsExit() && heldQty <= 0 {
		desk.publishDecision(action, "rejected", "no open position")
		return errnie.Error(ErrNoPosition)
	}

	if !action.Type.IsExit() && desk.ledger.OpenCount() >= viper.GetInt("trading.max_concurrent_positions") {
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

	quantity, sizeErr := SizeOrder(action, desk.ledger.QuoteCash(), heldQty, mark)

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
	limitPrice := action.Price

	if limitPrice <= 0 {
		limitPrice = mark
	}

	params := &trading.AddParams{
		OrderType:  orderType,
		Side:       action.Side,
		Symbol:     action.Symbol,
		LimitPrice: limitPrice,
		OrderQty:   quantity,
		ClOrdID:    clOrdID,
		Token:      token,
	}

	if !action.Type.IsExit() {
		params.EntryQueuedAt = time.Now()
	}

	frame := types.KrakenMessage{
		Method: trading.MethodAddOrder,
		Params: params,
		ReqID:  time.Now().UnixNano(),
	}

	desk.orders.Store(clOrdID, frame)
	desk.publishDecision(action, "submitted", "")

	return errnie.Error(desk.bus.Send("kraken:private", "orders", frame))
}

func (desk *Desk) publishDecision(
	action *logic.Action,
	verdict string,
	reason string,
) {
	if action == nil {
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

	errnie.Error(desk.bus.Send("ui", "decision", map[string]any{
		"event":    "decision",
		"type":     action.Type.String(),
		"symbol":   action.Symbol,
		"key":      action.BranchKey,
		"verdict":  verdict,
		"reason":   reason,
		"chart":    "decision_tree",
		"decision": true,
	}))
}

func (desk *Desk) CheckOrder(orderID string) error {
	frame, ok := desk.orders.Load(orderID)

	if !ok {
		return errnie.Error(fmt.Errorf("desk: order not found: %s", orderID))
	}

	return errnie.Error(desk.bus.Send("kraken:private", "orders", frame))
}

func (desk *Desk) Close() error {
	desk.cancel()
	return nil
}
