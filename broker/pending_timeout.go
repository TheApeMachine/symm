package broker

import (
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
)

func (desk *Desk) checkPendingTimeouts() {
	if desk == nil || desk.pendingByClOrdID == nil {
		return
	}

	ackTimeout := viper.GetDuration("trading.order_ack_timeout")
	transitTTL := viper.GetDuration("trading.entry.transit_ttl")

	now := time.Now().UTC()
	desk.pendingByClOrdID.Range(func(_ any, value any) bool {
		pending, ok := value.(*PendingOrder)
		if !ok || pending == nil || pending.CreatedAt.IsZero() {
			return true
		}

		if pending.LastStatus == "" && ackTimeout > 0 && now.Sub(pending.CreatedAt) >= ackTimeout {
			pending.LastStatus = "order_ack_timeout"
			desk.publishPendingDiagnostic(pending, "error", "order_ack_timeout")
			desk.submitEntryCancel(pending)
			return true
		}

		if transitTTL > 0 && entryTransitExpired(pending, now, transitTTL) {
			desk.submitEntryCancel(pending)
		}

		return true
	})
}

func entryTransitExpired(pending *PendingOrder, now time.Time, ttl time.Duration) bool {
	if pending == nil || pending.Protective || pending.CancelSubmitted {
		return false
	}
	if !strings.EqualFold(pending.Side, "buy") {
		return false
	}
	if !pendingEntryTransitStatus(pending.LastStatus) {
		return false
	}
	if pending.CreatedAt.IsZero() || ttl <= 0 {
		return false
	}

	return now.Sub(pending.CreatedAt) >= ttl
}

func pendingEntryTransitStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "open", "partially_filled", "partial", "new":
		return true
	default:
		return false
	}
}

func (desk *Desk) submitEntryCancel(pending *PendingOrder) {
	if desk == nil || pending == nil || pending.CancelSubmitted {
		return
	}

	identifier := pending.ExchangeOrderID
	if identifier == "" {
		identifier = pending.ClOrdID
	}
	if identifier == "" {
		return
	}

	pending.CancelSubmitted = true
	pending.LastStatus = "cancel_submitted"

	cancel := datura.Acquire("broker", datura.APPJSON).
		WithDestination("kraken:private").
		WithRole("orders").
		WithScope(pending.Symbol).
		WithPayload(datura.Map[any]{
			"method": "cancel_order",
			"params": datura.Map[any]{
				"symbol":      pending.Symbol,
				"order_id":    identifier,
				"cl_ord_id":   pending.ClOrdID,
				"decision_id": pending.DecisionID,
				"action_id":   pending.ActionID,
			},
		}.Marshal())

	desk.publishPendingDiagnostic(pending, "warning", "entry_transit_ttl_cancel_submitted")
	desk.sendPrivate(cancel)
}

func pendingLiveEntryExposure(pending *PendingOrder) bool {
	if pending == nil || pending.Protective || !strings.EqualFold(pending.Side, "buy") {
		return false
	}

	return pendingLiveOrderStatus(pending.LastStatus)
}

func pendingLiveSellExposure(pending *PendingOrder) bool {
	if pending == nil || !strings.EqualFold(pending.Side, "sell") {
		return false
	}

	return pendingLiveOrderStatus(pending.LastStatus)
}

func pendingLiveOrderStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "pending_ack", "order_ack_timeout", "open", "new", "partially_filled", "partial", "cancel_submitted":
		return true
	default:
		return false
	}
}
