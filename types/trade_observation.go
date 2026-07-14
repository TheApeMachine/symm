package types

import "time"

/*
TradeObservation is one immutable broker or position fact appended to the
originating Thesis. Strings preserve the exchange's decimal representation
without placing the live mutable Position on the lifecycle record.
*/
type TradeObservation struct {
	Kind        string    `json:"kind"`
	Action      string    `json:"action,omitempty"`
	Symbol      string    `json:"symbol"`
	Side        string    `json:"side,omitempty"`
	Status      string    `json:"status,omitempty"`
	OrderID     string    `json:"orderId,omitempty"`
	ExecutionID string    `json:"executionId,omitempty"`
	Quantity    string    `json:"quantity,omitempty"`
	Price       string    `json:"price,omitempty"`
	Cost        string    `json:"cost,omitempty"`
	Fee         string    `json:"fee,omitempty"`
	PnL         string    `json:"pnl,omitempty"`
	ReturnPct   float64   `json:"returnPct,omitempty"`
	Decision    int       `json:"decision"`
	Error       string    `json:"error,omitempty"`
	At          time.Time `json:"at"`
}
