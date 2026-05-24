package trader

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/config"
)

/*
PaperBroker simulates Kraken spot fills with slippage, proceeds-based fees,
and resting OTO stop orders for paper/live parity.
*/
type PaperBroker struct {
	mu           sync.Mutex
	nextOrderSeq uint64
	restingStops map[string]paperRestingStop
}

type paperRestingStop struct {
	symbol       string
	triggerPrice float64
}

/*
NewPaperBroker creates a paper execution broker.
*/
func NewPaperBroker() *PaperBroker {
	return &PaperBroker{
		restingStops: make(map[string]paperRestingStop),
	}
}

/*
Live reports false for paper execution.
*/
func (paperBroker *PaperBroker) Live() bool {
	return false
}

/*
SupportsShort reports whether paper mode may open synthetic short positions.
*/
func (paperBroker *PaperBroker) SupportsShort() bool {
	return config.System.AllowPaperShorts
}

/*
Enter simulates one entry fill from quote and book depth.
*/
func (paperBroker *PaperBroker) Enter(
	ctx context.Context,
	request BrokerEnterRequest,
) (BrokerFill, error) {
	if err := paperBroker.waitForLatency(ctx); err != nil {
		return BrokerFill{}, err
	}

	fillSide := "buy"

	if request.Side == positionShort {
		fillSide = "sell"
	}

	fill := config.System.SlippageFill(
		request.Last, request.Bid, request.Ask, fillSide, config.System.SlippageBPS,
		request.NotionalEUR, request.BidLevels, request.AskLevels,
	)

	if fill <= 0 {
		return BrokerFill{}, errInvalidFill
	}

	baseQty := roundBaseQty(request.NotionalEUR/fill, paperLotDecimals)

	if baseQty <= 0 {
		return BrokerFill{}, errInvalidFill
	}

	proceeds, fee, _ := spotLongEntryCost(baseQty, fill, request.FeePct)

	if request.Side == positionShort {
		proceeds = spotProceedsEUR(baseQty, fill)
		fee = spotTakerFeeEUR(proceeds, request.FeePct)
	}

	orderID, stopOrderID := paperBroker.allocateOrderIDs(request)

	return BrokerFill{
		FillPrice:   fill,
		BaseQty:     baseQty,
		FeeEUR:      fee,
		OrderID:     orderID,
		StopOrderID: stopOrderID,
	}, nil
}

func (paperBroker *PaperBroker) allocateOrderIDs(request BrokerEnterRequest) (string, string) {
	seq := atomic.AddUint64(&paperBroker.nextOrderSeq, 1)
	orderID := fmt.Sprintf("paper-%d", seq)

	if request.StopPrice <= 0 || request.Side != positionLong {
		return orderID, ""
	}

	stopOrderID := fmt.Sprintf("paper-stop-%d", seq)

	paperBroker.mu.Lock()
	paperBroker.restingStops[stopOrderID] = paperRestingStop{
		symbol:       request.Symbol,
		triggerPrice: request.StopPrice,
	}
	paperBroker.mu.Unlock()

	return orderID, stopOrderID
}

/*
Exit simulates one exit fill from quote and book depth.
*/
func (paperBroker *PaperBroker) Exit(
	ctx context.Context,
	request BrokerExitRequest,
) (BrokerFill, error) {
	if err := paperBroker.waitForLatency(ctx); err != nil {
		return BrokerFill{}, err
	}

	fillSide := "sell"

	if request.Side == positionShort {
		fillSide = "buy"
	}

	var exitFill float64

	if request.StopExit && request.Side == positionLong {
		exitFill = StopLossLimitFill(
			request.Last,
			request.StopPrice,
			request.LimitPrice,
			request.Bid,
			request.Ask,
			request.BaseQty,
			request.BidLevels,
		)
	}

	if exitFill <= 0 {
		notional := request.NotionalEUR

		if request.BaseQty > 0 && request.Last > 0 {
			notional = request.BaseQty * request.Last
		}

		exitFill = config.System.SlippageFill(
			request.Last, request.Bid, request.Ask, fillSide, config.System.SlippageBPS,
			notional, request.BidLevels, request.AskLevels,
		)
	}

	if exitFill <= 0 {
		return BrokerFill{}, errInvalidFill
	}

	proceeds := spotProceedsEUR(request.BaseQty, exitFill)

	if proceeds <= 0 && request.Last > 0 {
		proceeds = request.NotionalEUR * (exitFill / request.Last)
	}

	fee := spotTakerFeeEUR(proceeds, request.FeePct)

	if request.StopOrderID != "" {
		paperBroker.clearRestingStop(request.StopOrderID)
	}

	return BrokerFill{
		FillPrice: exitFill,
		BaseQty:   request.BaseQty,
		FeeEUR:    fee,
	}, nil
}

func (paperBroker *PaperBroker) clearRestingStop(stopOrderID string) {
	paperBroker.mu.Lock()
	delete(paperBroker.restingStops, stopOrderID)
	paperBroker.mu.Unlock()
}

/*
AmendStop updates a resting paper stop trigger, mirroring live amend_order.
*/
func (paperBroker *PaperBroker) AmendStop(
	_ context.Context,
	request BrokerAmendStopRequest,
) error {
	if request.OrderID == "" {
		return fmt.Errorf("stop order id is required")
	}

	if request.TriggerPrice <= 0 {
		return fmt.Errorf("stop trigger price must be positive")
	}

	paperBroker.mu.Lock()
	defer paperBroker.mu.Unlock()

	stop, ok := paperBroker.restingStops[request.OrderID]

	if !ok {
		return fmt.Errorf("unknown stop order %s", request.OrderID)
	}

	stop.triggerPrice = request.TriggerPrice
	paperBroker.restingStops[request.OrderID] = stop

	return nil
}

func (paperBroker *PaperBroker) waitForLatency(ctx context.Context) error {
	latency := config.System.PaperOrderLatency

	if latency <= 0 {
		return nil
	}

	timer := time.NewTimer(latency)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

const paperLotDecimals = 8

var errInvalidFill = errString("invalid fill price")

type errString string

func (err errString) Error() string { return string(err) }
