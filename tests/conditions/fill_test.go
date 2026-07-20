package conditions_test

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

/*
TestEntryFill proves the private producer preserves the submitted request
identity and emits an exact decimal wallet transition.
*/
func TestEntryFill(t *testing.T) {
	Convey("Given a sized entry and its submitted Kraken request", t, func() {
		request := kraken.NewMarketOrder(
			"buy", decimal.NewFromInt64(10), conditions.Subject(),
		)
		raw, err := request.MarshalJSON()
		So(err, ShouldBeNil)
		decision := types.Decision{
			Action:           types.ActionEnter,
			Symbol:           conditions.Subject(),
			At:               time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			ProposedQuantity: decimal.NewFromInt64(10),
			ProposedNotional: decimal.NewFromFloat64(5.70),
			ReferencePrice:   decimal.NewFromFloat64(0.568),
		}

		frames := conditions.EntryFill(raw, decision, decimal.NewFromInt64(1000))

		Convey("Then acknowledgement, fill, and wallet facts are causally aligned", func() {
			So(len(frames), ShouldEqual, 3)
			So(frames[0].Channel, ShouldEqual, "add_order")
			So(kraken.NewOrderResponse(frames[0].Payload).ReqID, ShouldEqual, request.ReqID)
			So(frames[1].Channel, ShouldEqual, "executions")
			execution := kraken.NewExecution(frames[1].Payload)
			So(execution.Data[0].OrderID, ShouldEqual, "synthetic-entry-1")
			So(execution.Data[0].Cost.Cmp(decimal.NewFromFloat64(5.68)), ShouldEqual, 0)
			So(frames[2].Channel, ShouldEqual, "balances")
			balance := kraken.NewBalance(frames[2].Payload)
			So(balance.Data[0].Available.Cmp(
				decimal.NewFromFloat64(994.3),
			), ShouldEqual, 0)
			So(balance.Data[1].Balance.Cmp(decimal.NewFromInt64(10)), ShouldEqual, 0)
		})
	})
}

/*
TestExitFill proves a synthetic sell fill removes the base asset and credits
decimal proceeds net of the supplied venue fee.
*/
func TestExitFill(t *testing.T) {
	Convey("Given an exit decision and its submitted Kraken request", t, func() {
		request := kraken.NewMarketOrder(
			"sell", decimal.NewFromInt64(10), conditions.Subject(),
		)
		raw, err := request.MarshalJSON()
		So(err, ShouldBeNil)
		decision := types.Decision{
			Action:           types.ActionExit,
			Symbol:           conditions.Subject(),
			At:               time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			ProposedQuantity: decimal.NewFromInt64(10),
			ReferencePrice:   decimal.NewFromFloat64(0.568),
		}
		fee := decimal.NewFromFloat64(0.01)

		frames := conditions.ExitFill(
			raw, decision, decimal.NewFromInt64(1000), fee,
		)

		Convey("Then the fill and flattened wallet remain exact", func() {
			So(len(frames), ShouldEqual, 3)
			So(kraken.NewOrderResponse(frames[0].Payload).ReqID, ShouldEqual, request.ReqID)
			execution := kraken.NewExecution(frames[1].Payload)
			So(execution.Data[0].Side, ShouldEqual, "sell")
			So(execution.Data[0].FeeUsdEquiv.Cmp(fee), ShouldEqual, 0)
			balance := kraken.NewBalance(frames[2].Payload)
			So(len(balance.Data), ShouldEqual, 1)
			So(balance.Data[0].Asset, ShouldEqual, "USD")
			So(balance.Data[0].Available.Cmp(
				decimal.NewFromFloat64(1005.67),
			), ShouldEqual, 0)
		})
	})
}
