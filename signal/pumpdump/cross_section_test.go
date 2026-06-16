package pumpdump

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestCrossSectionLastRvol(testingTB *testing.T) {
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	Convey("Given a long silence before a thin print", testingTB, func() {
		crossSection := NewCrossSection(time.Minute, 0)

		_ = crossSection.observeTrade(&krakenmarket.TradeUpdate{
			Symbol:    "ALT/EUR",
			Price:     1,
			Qty:       1000,
			Timestamp: eventAt,
		})
		_ = crossSection.observeTrade(&krakenmarket.TradeUpdate{
			Symbol:    "ALT/EUR",
			Price:     1,
			Qty:       1000,
			Timestamp: eventAt.Add(time.Second),
		})
		_ = crossSection.observeTrade(&krakenmarket.TradeUpdate{
			Symbol:    "ALT/EUR",
			Price:     1,
			Qty:       5,
			Timestamp: eventAt.Add(10 * time.Minute),
		})

		Convey("It should decay stale volume context instead of phantom-spiking", func() {
			So(crossSection.LastRvol("ALT/EUR"), ShouldBeLessThan, 0.1)
		})
	})
}

func TestCrossSectionVerticalityPayload(testingTB *testing.T) {
	Convey("Given warmed volume and spread baselines", testingTB, func() {
		crossSection := NewCrossSection(time.Minute, 0)
		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		_ = crossSection.observeTrade(&krakenmarket.TradeUpdate{
			Symbol:    "ETH/EUR",
			Price:     100,
			Qty:       2,
			Timestamp: eventAt,
		})
		_ = crossSection.observeBook(&krakenmarket.BookUpdate{
			Symbol:    "ETH/EUR",
			Bids:      []krakenmarket.BookLevel{{Price: 100, Qty: 8}},
			Asks:      []krakenmarket.BookLevel{{Price: 100.1, Qty: 4}},
			Timestamp: eventAt.Add(time.Millisecond),
		})

		payload, ok := crossSection.verticalityPayload("ETH/EUR", 1.5, 0.015)

		Convey("It should expose the verticality feature vector", func() {
			So(ok, ShouldBeTrue)
			So(len(payload), ShouldEqual, 4)
			So(payload[0], ShouldBeGreaterThan, 0)
			So(payload[3], ShouldEqual, 1.5)
		})
	})
}
