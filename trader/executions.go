package trader

import (
	"fmt"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/order"
)

/*
applyFill is the single live-side write-back path for executions. It
dedupes via wallet.ApplyFill on the fill's ExecKey, so a reconnect that
replays the same execution does not double-credit inventory or
double-debit cash.

Paper fills mutate the wallet inline inside broker.{Buy,Sell}.FillPaper.
They still flow through this channel as informational frames, but the
"paper-" OrderID prefix is the marker we use to skip them -- applying
their state again would double everything. Live OrderIDs from Kraken
never carry that prefix.
*/
func (crypto *Crypto) applyFill(raw any) {
	fill, ok := raw.(order.Fill)

	if !ok {
		errnie.Error(fmt.Errorf("invalid execution payload: %T", raw))
		return
	}

	if crypto.wallet == nil {
		return
	}

	if strings.HasPrefix(fill.OrderID, "paper-") {
		return
	}

	base := symbolBase(fill.Symbol)
	cashDelta := 0.0

	switch fill.Side {
	case "buy":
		cashDelta = -fill.Qty*fill.Price - fill.Fee
	case "sell":
		cashDelta = fill.Qty*fill.Price - fill.Fee
	}

	if !crypto.wallet.ApplyFill(fill.ExecKey, fill.Side, base, fill.Qty, fill.Price, cashDelta) {
		audit("fill_dedupe", map[string]any{
			"exec_key": fill.ExecKey,
			"order_id": fill.OrderID,
			"symbol":   fill.Symbol,
		})

		return
	}
	crypto.execution.HandleFill(fill)

	audit("fill_applied", map[string]any{
		"exec_key": fill.ExecKey,
		"order_id": fill.OrderID,
		"symbol":   fill.Symbol,
		"side":     fill.Side,
		"qty":      fill.Qty,
		"price":    fill.Price,
	})
}

/*
handleOrderAck records the exchange-assigned OrderID against the
client-side cl_ord_id so subsequent Cancel / Amend can address the
exchange's identifier. Errors from the exchange are surfaced via the
audit log.
*/
func (crypto *Crypto) handleOrderAck(raw any) {
	ack, ok := raw.(*order.Ack)

	if !ok {
		errnie.Error(fmt.Errorf("invalid order ack payload: %T", raw))
		return
	}

	audit("order_ack", map[string]any{
		"method":    ack.Method,
		"req_id":    ack.ReqID,
		"success":   ack.Success,
		"error":     ack.Error,
		"order_id":  ack.Result.OrderID,
		"cl_ord_id": ack.Result.ClOrdID,
	})
	crypto.execution.HandleAck(ack)
}
