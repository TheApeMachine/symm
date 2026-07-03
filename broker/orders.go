package broker

import (
	"strings"
	"time"
)

type PendingOrder struct {
	ClOrdID         string
	ExchangeOrderID string
	DecisionID      string
	ActionID        string
	Symbol          string
	Side            string
	OrderType       string
	Qty             float64
	Notional        float64
	CreatedAt       time.Time
	LastStatus      string
	CancelSubmitted bool
	Protective      bool
	Stoploss        *Stoploss
}

func (order PendingOrder) Key() string {
	return strings.TrimSpace(order.ClOrdID)
}

func pendingKey(symbol, side string) string {
	return strings.ToUpper(strings.TrimSpace(symbol)) + ":" +
		strings.ToLower(strings.TrimSpace(side))
}

func workingOrderKey(symbol, clOrdID string) string {
	return strings.ToUpper(strings.TrimSpace(symbol)) + ":" +
		strings.TrimSpace(clOrdID)
}
