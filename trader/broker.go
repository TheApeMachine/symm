package trader

import (
	"context"

	"github.com/theapemachine/symm/kraken/market"
)

/*
BrokerFill is one confirmed or simulated trade execution.
*/
type BrokerFill struct {
	FillPrice   float64
	BaseQty     float64
	FeeEUR      float64
	OrderID     string
	StopOrderID string
}

/*
BrokerEnterRequest is one long or short entry order.
*/
type BrokerEnterRequest struct {
	Symbol      string
	Side        int
	NotionalEUR float64
	Last        float64
	Bid         float64
	Ask         float64
	StopPrice   float64
	FeePct      float64
	BidLevels   []market.BookLevel
	AskLevels   []market.BookLevel
}

/*
BrokerExitRequest is one position close order.
*/
type BrokerExitRequest struct {
	Symbol      string
	Side        int
	NotionalEUR float64
	BaseQty     float64
	Last        float64
	Bid         float64
	Ask         float64
	FeePct      float64
	BidLevels   []market.BookLevel
	AskLevels   []market.BookLevel
	StopExit    bool
	StopPrice   float64
	LimitPrice  float64
	StopOrderID string
}

/*
FillPoller reads one buffered execution without blocking.
*/
type FillPoller interface {
	PollFill(orderID string) (BrokerFill, bool)
}

/*
BrokerAmendStopRequest updates a resting exchange stop trigger.
*/
type BrokerAmendStopRequest struct {
	OrderID      string
	TriggerPrice float64
}

/*
ExecutionBroker submits entries and exits to Kraken or simulates them in paper mode.
*/
type ExecutionBroker interface {
	Live() bool
	SupportsShort() bool
	Enter(ctx context.Context, request BrokerEnterRequest) (BrokerFill, error)
	Exit(ctx context.Context, request BrokerExitRequest) (BrokerFill, error)
	AmendStop(ctx context.Context, request BrokerAmendStopRequest) error
}
