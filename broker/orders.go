package broker

import (
	"strings"
	"time"
)

type PendingOrder struct {
	ClOrdID         string
	ExchangeOrderID string
	Symbol          string
	Side            string
	OrderType       string
	Qty             float64
	Notional        float64
	CreatedAt       time.Time
	LastStatus      string
	Protective      bool
	Attempt         int
}

func (order PendingOrder) Key() string {
	return pendingKey(order.Symbol, order.Side)
}

func pendingKey(symbol, side string) string {
	return strings.ToUpper(strings.TrimSpace(symbol)) + ":" +
		strings.ToLower(strings.TrimSpace(side))
}
