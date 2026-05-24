package trader

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestPaperBrokerEnterFeeOnProceeds(t *testing.T) {
	convey.Convey("Given a paper broker entry", t, func() {
		broker := NewPaperBroker()
		request := BrokerEnterRequest{
			Symbol:      "BTC/EUR",
			Side:        positionLong,
			NotionalEUR: 100,
			Last:        100,
			Bid:         99.9,
			Ask:         100.1,
			StopPrice:   95,
			FeePct:      0.26,
		}

		convey.Convey("It should charge taker fee on fill proceeds", func() {
			fill, err := broker.Enter(context.Background(), request)
			convey.So(err, convey.ShouldBeNil)

			expectedProceeds := spotProceedsEUR(fill.BaseQty, fill.FillPrice)
			expectedFee := spotTakerFeeEUR(expectedProceeds, request.FeePct)

			convey.So(fill.FeeEUR, convey.ShouldAlmostEqual, expectedFee, 0.0001)
			convey.So(expectedProceeds, convey.ShouldAlmostEqual, fill.BaseQty*fill.FillPrice, 0.0001)
		})
	})
}

func TestPaperBrokerReturnsStopOrderID(t *testing.T) {
	convey.Convey("Given a long entry with protective stop", t, func() {
		broker := NewPaperBroker()
		fill, err := broker.Enter(context.Background(), BrokerEnterRequest{
			Symbol:      "BTC/EUR",
			Side:        positionLong,
			NotionalEUR: 100,
			Last:        100,
			Bid:         99.9,
			Ask:         100.1,
			StopPrice:   95,
			FeePct:      0.26,
		})

		convey.Convey("It should return entry and stop order ids", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(fill.OrderID, convey.ShouldNotBeBlank)
			convey.So(fill.StopOrderID, convey.ShouldNotBeBlank)
		})
	})
}

func TestPaperBrokerAmendStop(t *testing.T) {
	convey.Convey("Given a resting paper stop", t, func() {
		broker := NewPaperBroker()
		fill, err := broker.Enter(context.Background(), BrokerEnterRequest{
			Symbol:      "BTC/EUR",
			Side:        positionLong,
			NotionalEUR: 100,
			Last:        100,
			Bid:         99.9,
			Ask:         100.1,
			StopPrice:   95,
			FeePct:      0.26,
		})
		convey.So(err, convey.ShouldBeNil)

		convey.Convey("It should update the stop trigger", func() {
			err = broker.AmendStop(context.Background(), BrokerAmendStopRequest{
				OrderID:      fill.StopOrderID,
				TriggerPrice: 96,
			})
			convey.So(err, convey.ShouldBeNil)
		})

		convey.Convey("It should reject unknown stop ids", func() {
			err = broker.AmendStop(context.Background(), BrokerAmendStopRequest{
				OrderID:      "missing",
				TriggerPrice: 96,
			})
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}
