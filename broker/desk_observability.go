package broker

import (
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/observability"
)

func (desk *Desk) ensureActionIDs(action *logic.Action) {
	if action == nil {
		return
	}

	if action.ActionID == "" {
		action.ActionID = uuid.New().String()
	}

	if action.DecisionID == "" {
		action.DecisionID = action.ActionID
	}
}

func (desk *Desk) recordRiskReject(action *logic.Action, riskErr error) {
	if desk == nil || desk.metrics == nil || riskErr == nil {
		return
	}

	symbol := ""

	if action != nil {
		symbol = action.Symbol
	}

	desk.metrics.RecordRiskReject(symbol, riskErr.Error(), time.Now().UTC())
}

func (desk *Desk) recordOrderSubmitted(
	action *logic.Action,
	clOrdID string,
	submitLatency time.Duration,
	observedAt time.Time,
) {
	if desk == nil || desk.metrics == nil || action == nil {
		return
	}

	desk.metrics.RecordOrderSubmitted(
		observability.OrderCorrelation{
			DecisionID: action.DecisionID,
			ActionID:   action.ActionID,
			ClOrdID:    clOrdID,
			Symbol:     action.Symbol,
		},
		submitLatency,
		pendingNotional(action),
		observedAt,
	)
}

func (desk *Desk) recordOrderExecution(
	clOrdID string,
	execution user.Execution,
) {
	if desk == nil || desk.metrics == nil {
		return
	}

	desk.metrics.RecordOrderExecution(
		clOrdID,
		execution.OrderID,
		execution.ExecID,
		execution.OrderStatus,
		execution.ExecType,
		time.Now().UTC(),
	)
}

func (desk *Desk) recordTickerAge(ticker *market.TickerUpdate) {
	if desk == nil || desk.metrics == nil || ticker == nil {
		return
	}

	desk.metrics.RecordMarketDataAge(
		"ticker",
		ticker.Symbol,
		ticker.Timestamp,
		time.Now().UTC(),
	)
}

func (desk *Desk) recordBookAge(book *market.BookUpdate, observedAt time.Time) {
	if desk == nil || desk.metrics == nil || book == nil {
		return
	}

	desk.metrics.RecordMarketDataAge(
		"book",
		book.Symbol,
		book.Timestamp,
		observedAt,
	)
}

func (desk *Desk) recordTradeAge(trade *market.TradeUpdate, observedAt time.Time) {
	if desk == nil || desk.metrics == nil || trade == nil {
		return
	}

	desk.metrics.RecordMarketDataAge(
		"trade",
		trade.Symbol,
		trade.Timestamp,
		observedAt,
	)
}

func (desk *Desk) recordStopTriggered(symbol string, observedAt time.Time) {
	if desk == nil || desk.metrics == nil {
		return
	}

	desk.metrics.RecordStopTriggered(symbol, observedAt)
}

func (desk *Desk) recordStopExitSubmitted(
	symbol string,
	triggeredAt time.Time,
) {
	if desk == nil || desk.metrics == nil {
		return
	}

	desk.metrics.RecordStopExitSubmitted(symbol, triggeredAt, time.Now().UTC())
}

func (desk *Desk) recordStopExitFilled(
	symbol string,
	triggeredAt time.Time,
) {
	if desk == nil || desk.metrics == nil {
		return
	}

	desk.metrics.RecordStopExitFilled(symbol, triggeredAt, time.Now().UTC())
}

func (desk *Desk) recordStopNeedsRepair(symbol string, reason string) {
	if desk == nil || desk.metrics == nil {
		return
	}

	desk.metrics.RecordStopNeedsRepair(symbol, reason, time.Now().UTC())
}

func (desk *Desk) recordExposure(frame PositionMonitorFrame) {
	if desk == nil || desk.metrics == nil {
		return
	}

	desk.metrics.RecordExposure(
		frame.Currency,
		frame.OpenPositions,
		frame.ExitValue,
		desk.pendingExposure(),
		frame.ExitBalance,
		time.Now().UTC(),
	)
}

func (desk *Desk) pendingExposure() float64 {
	if desk == nil || desk.actions == nil {
		return 0
	}

	total := 0.0

	desk.actions.Range(func(_ any, value any) bool {
		action, ok := value.(*logic.Action)

		if !ok || action == nil {
			return true
		}

		total += pendingNotional(action)

		return true
	})

	return total
}

func pendingNotional(action *logic.Action) float64 {
	if action == nil || action.Price <= 0 || action.Quantity <= 0 {
		return 0
	}

	return action.Price * action.Quantity
}
