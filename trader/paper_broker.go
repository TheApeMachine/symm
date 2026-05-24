package trader

import (
	"context"

	"github.com/theapemachine/symm/config"
)

/*
PaperBroker simulates taker fills with configured slippage and fees.
*/
type PaperBroker struct{}

/*
NewPaperBroker creates a paper execution broker.
*/
func NewPaperBroker() *PaperBroker {
	return &PaperBroker{}
}

/*
Live reports false for paper execution.
*/
func (paperBroker *PaperBroker) Live() bool {
	return false
}

/*
Enter simulates one entry fill from quote and book depth.
*/
func (paperBroker *PaperBroker) Enter(
	_ context.Context,
	request BrokerEnterRequest,
) (BrokerFill, error) {
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

	fee := config.System.TakerFee(request.NotionalEUR, request.FeePct)
	baseQty := request.NotionalEUR / fill

	return BrokerFill{
		FillPrice: fill,
		BaseQty:   baseQty,
		FeeEUR:    fee,
	}, nil
}

/*
Exit simulates one exit fill from quote and book depth.
*/
func (paperBroker *PaperBroker) Exit(
	_ context.Context,
	request BrokerExitRequest,
) (BrokerFill, error) {
	fillSide := "sell"

	if request.Side == positionShort {
		fillSide = "buy"
	}

	exitFill := config.System.SlippageFill(
		request.Last, request.Bid, request.Ask, fillSide, config.System.SlippageBPS,
		request.NotionalEUR, request.BidLevels, request.AskLevels,
	)

	if exitFill <= 0 {
		return BrokerFill{}, errInvalidFill
	}

	proceeds := request.BaseQty * exitFill

	if proceeds <= 0 && request.Last > 0 {
		proceeds = request.NotionalEUR * (exitFill / request.Last)
	}

	fee := config.System.TakerFee(proceeds, request.FeePct)

	return BrokerFill{
		FillPrice: exitFill,
		BaseQty:   request.BaseQty,
		FeeEUR:    fee,
	}, nil
}

/*
AmendStop is a no-op in paper mode.
*/
func (paperBroker *PaperBroker) AmendStop(
	_ context.Context,
	_ BrokerAmendStopRequest,
) error {
	return nil
}

var errInvalidFill = errString("invalid fill price")

type errString string

func (err errString) Error() string { return string(err) }
