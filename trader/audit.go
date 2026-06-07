package trader

import (
	"strings"
	"time"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

func (crypto *Crypto) quoteAuditFields(action reasoning.Action) map[string]any {
	if crypto.quotes == nil {
		return nil
	}

	quote, ok := crypto.quotes.Snapshot(action.Symbol)

	if !ok {
		return nil
	}

	quantity := action.Quantity

	if quantity <= 0 {
		quantity = 1
	}

	return broker.QuoteAuditFields(quote, action.Side, quantity, time.Now().UTC())
}

func (crypto *Crypto) mergeQuoteAudit(frame map[string]any, action reasoning.Action) {
	for key, value := range crypto.quoteAuditFields(action) {
		frame[key] = value
	}
}

func preflightGateFromReason(reason string) string {
	reason = strings.ToLower(reason)

	switch {
	case strings.Contains(reason, "spread"):
		return "spread"
	case strings.Contains(reason, "stale quote"):
		return "stale_quote"
	case strings.Contains(reason, "slippage"):
		return "slippage"
	case strings.Contains(reason, "depth"):
		return "depth"
	case strings.Contains(reason, "no quote"):
		return "no_quote"
	default:
		return ""
	}
}

func (crypto *Crypto) writeFillAudit(
	action reasoning.Action,
	price float64,
	quantity float64,
	fee float64,
	verdict string,
) error {
	if crypto.audit == nil {
		return nil
	}

	frame := map[string]any{
		"audit_event": "fill",
		"symbol":      action.Symbol,
		"type":        action.Type.String(),
		"side":        string(action.Side),
		"verdict":     verdict,
		"price":       price,
		"quantity":    quantity,
		"fee":         fee,
	}

	crypto.mergeQuoteAudit(frame, reasoning.Action{
		Symbol:   action.Symbol,
		Side:     action.Side,
		Quantity: quantity,
	})

	return crypto.audit.Write(frame)
}

func (crypto *Crypto) writePositionOpenAudit(
	symbol string,
	side trading.Side,
	entryPrice float64,
	quantity float64,
	fee float64,
	actionType reasoning.ActionType,
) error {
	if crypto.audit == nil {
		return nil
	}

	frame := map[string]any{
		"audit_event": "position_open",
		"symbol":      symbol,
		"side":        string(side),
		"entry_price": entryPrice,
		"quantity":    quantity,
		"fee":         fee,
		"action_type": actionType.String(),
	}

	crypto.mergeQuoteAudit(frame, reasoning.Action{
		Symbol:   symbol,
		Side:     side,
		Quantity: quantity,
	})

	return crypto.audit.Write(frame)
}
