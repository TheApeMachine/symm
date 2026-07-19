package tests_test

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

var inventorySignals tests.SignalFactory = func(
	ctx context.Context,
	api *websocket.API,
	_ *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{pumpdump.NewSignal(ctx, api, channel)}
}

/*
TestSessionLockedInventoryStaysOpen proves wallet Balance is inventory authority:
Available=0 with positive Balance keeps the lot open and occupying a desk slot.
*/
func TestSessionLockedInventoryStaysOpen(t *testing.T) {
	Convey("Given an open lot whose exchange Available is fully reserved", t, func() {
		symbol := conditions.Subject()
		session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
			Signals: inventorySignals,
		})
		So(err, ShouldBeNil)

		lot, _, err := session.PlayOpen(
			t, conditions.PhantomDrawdown(24, 8, 0.015), symbol, 0.01, 0.01,
		)
		So(err, ShouldBeNil)
		So(lot, ShouldNotBeNil)

		err = session.SeedLockedLot(9_000, lot.Asset, 0.01)
		So(err, ShouldBeNil)

		holding, holdErr := session.Balance.Holding(symbol)
		So(holdErr, ShouldBeNil)

		Convey("Then Qty tracks Balance, SellableQty is zero, and the slot stays occupied", func() {
			So(holding.Status, ShouldEqual, types.OPEN)
			So(holding.Qty.Float64(), ShouldAlmostEqual, 0.01, 1e-9)
			So(holding.SellableQty, ShouldNotBeNil)
			So(holding.SellableQty.Sign(), ShouldEqual, 0)
			So(session.Desk.OpenPositions(), ShouldEqual, 1)
		})
	})
}

/*
TestSessionReservationSurvivesBalanceSnapshot proves local Book claims are not
erased when a Kraken balances snapshot replaces the quote row.
*/
func TestSessionReservationSurvivesBalanceSnapshot(t *testing.T) {
	Convey("Given a Book claim and a later quote snapshot", t, func() {
		session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
			Signals: inventorySignals,
		})
		So(err, ShouldBeNil)

		So(session.SeedQuoteCapital(1_000), ShouldBeNil)

		claim, bookErr := session.Balance.Book(decimal.NewFromFloat64(250), nil)
		So(bookErr, ShouldBeNil)
		So(claim, ShouldNotBeNil)

		before, cashErr := session.Balance.AvailableCash()
		So(cashErr, ShouldBeNil)
		So(before.Float64(), ShouldAlmostEqual, 750, 1e-9)

		So(session.SeedQuoteCapital(1_000), ShouldBeNil)

		after, cashErr := session.Balance.AvailableCash()
		So(cashErr, ShouldBeNil)

		Convey("Then effective available still subtracts the live claim", func() {
			So(after.Float64(), ShouldAlmostEqual, 750, 1e-9)
			So(session.Balance.Funded(claim.ID, decimal.NewFromFloat64(250)), ShouldBeTrue)
		})
	})
}
