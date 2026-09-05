package broker

import (
	"errors"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

func increaseFixture(t testing.TB) (*Position, *executingConn) {
	t.Helper()
	position := closeFillPosition("10", "100", "1")
	position.pair.Symbol = "EDGE/USD"
	position.Holding.EntryQty = mustDecimal("10")
	position.Holding.EntryVWAP = mustDecimal("100")
	position.Holding.EntryFees = mustDecimal("1")
	position.EntryOrder = &spot.AddOrderRequest{ClOrdId: "entry"}
	conn := newExecutingConn(nil)
	position.api = websocket.NewAPI(t.Context(), conn, conn)
	position.price = entryEconomicsFixture(t, 110, 109, 20)
	position.Increase = &LotIncrease{position: position}
	return position, conn
}

func TestLotIncreasePlace(t *testing.T) {
	Convey("Given a filled spot lot and an independently admitted increase", t, func() {
		position, conn := increaseFixture(t)
		decision := types.Decision{ID: "increase", Action: types.ActionScale, ProposedQuantity: mustDecimal("2")}
		Convey("the final cash admission and authority precede venue submission", func() {
			var admitted bool
			decision.Admit = func(cost *types.EntryCost) error {
				admitted = true
				So(cost.EntryPrice.Cmp(mustDecimal("110")), ShouldEqual, 0)
				return nil
			}
			decision.Permit = func() bool { return admitted }
			So(position.Increase.Place(decision), ShouldBeNil)
			So(len(conn.submitted), ShouldEqual, 1)
			So(position.Increase.orders, ShouldResemble, []string{"venue-order-1"})
			So(position.Holding.Qty.Cmp(mustDecimal("10")), ShouldEqual, 0)
			So(position.Increase.Place(decision), ShouldNotBeNil)
		})
		Convey("insufficient capital and lost authority are pre-venue refusals", func() {
			decision.Admit = func(*types.EntryCost) error { return &types.ExecutionRefusal{State: "insufficient capital"} }
			var refusal *types.ExecutionRefusal
			So(errors.As(position.Increase.Place(decision), &refusal), ShouldBeTrue)
			decision.Admit = nil
			decision.Permit = func() bool { return false }
			So(errors.As(position.Increase.Place(decision), &refusal), ShouldBeTrue)
			So(len(conn.submitted), ShouldEqual, 0)
		})
	})
}

func TestLotIncreaseApply(t *testing.T) {
	Convey("Given an increase filled in multiple cumulative reports", t, func() {
		position, _ := increaseFixture(t)
		So(position.Increase.Place(types.Decision{ID: "increase", ProposedQuantity: mustDecimal("2")}), ShouldBeNil)
		execution := kraken.ExecutionData{ClientOrderID: "increase", OrderStatus: "partially_filled", CumQty: mustDecimal("1"), CumCost: mustDecimal("110"), FeeUsdEquiv: mustDecimal("0.1")}
		handled, err := position.Increase.Apply(execution)
		So(handled, ShouldBeTrue)
		So(err, ShouldBeNil)
		So(position.Holding.Qty.Cmp(mustDecimal("11")), ShouldEqual, 0)
		Convey("duplicate cumulative messages cannot buy inventory or fees twice", func() {
			_, err := position.Increase.Apply(execution)
			So(err, ShouldBeNil)
			So(position.Holding.Qty.Cmp(mustDecimal("11")), ShouldEqual, 0)
			So(position.Holding.EntryFee.Cmp(mustDecimal("1.1")), ShouldEqual, 0)
			So(position.Holding.EntryPrice.GetScale(), ShouldEqual, decimal.DefaultScale)
			execution.OrderStatus = "filled"
			execution.CumQty, execution.CumCost, execution.FeeUsdEquiv = mustDecimal("2"), mustDecimal("230"), mustDecimal("0.2")
			_, err = position.Increase.Apply(execution)
			So(err, ShouldBeNil)
			So(position.Holding.Qty.Cmp(mustDecimal("12")), ShouldEqual, 0)
			So(position.Holding.EntryPrice.Cmp(mustDecimal("102.5")), ShouldEqual, 0)
			So(position.Holding.EntryVWAP.Cmp(mustDecimal("102.5")), ShouldEqual, 0)
			So(position.Holding.EntryFee.Cmp(mustDecimal("1.2")), ShouldEqual, 0)
			So(position.Increase.order, ShouldBeNil)
			position.ReduceOrder = &spot.AddOrderRequest{ClOrdId: "reduce"}
			position.applyReduceFill(kraken.ExecutionData{OrderStatus: "filled", CumQty: mustDecimal("6")})
			So(position.Holding.EntryFee.Cmp(mustDecimal("0.6")), ShouldEqual, 0)
			So(position.Holding.EntryQty.Cmp(mustDecimal("12")), ShouldEqual, 0)
		})
		Convey("incomplete or backward cumulative economics cannot silently settle", func() {
			execution.CumCost = nil
			_, err := position.Increase.Apply(execution)
			So(err, ShouldNotBeNil)
			execution.CumCost = mustDecimal("100")
			_, err = position.Increase.Apply(execution)
			So(err, ShouldNotBeNil)
			execution.OrderStatus, execution.CumQty = "canceled", nil
			_, err = position.Increase.Apply(execution)
			So(err, ShouldNotBeNil)
			So(position.Increase.order, ShouldNotBeNil)
		})
	})
}

func TestLotIncreaseCancel(t *testing.T) {
	Convey("Given a buy in flight when an exit is requested", t, func() {
		position, conn := increaseFixture(t)
		So(position.Increase.Place(types.Decision{ID: "increase", ProposedQuantity: mustDecimal("2")}), ShouldBeNil)
		var submitted []string
		position.recordFill = func(kind string, execution kraken.ExecutionData) {
			if kind == "execution_submitted" {
				submitted = append(submitted, execution.ClientOrderID)
			}
		}
		position.handleGuardian(types.Decision{ID: "exit", Action: types.ActionExit, Permit: func() bool { return false }})
		So(position.Increase.exitID, ShouldEqual, "exit")
		So(position.ExitOrder, ShouldBeNil)
		So(len(submitted), ShouldEqual, 0)
		execution := kraken.ExecutionData{ClientOrderID: "increase", OrderStatus: "canceled", CumQty: mustDecimal("1"), CumCost: mustDecimal("110"), FeeUsdEquiv: mustDecimal("0.1")}
		_, err := position.Increase.Apply(execution)
		So(err, ShouldBeNil)
		So(mustDecimal(position.ExitOrder.Volume).Cmp(mustDecimal("11")), ShouldEqual, 0)
		So(position.ExitOrder.ClOrdId, ShouldEqual, "exit")
		So(len(conn.submitted), ShouldEqual, 2)
		So(submitted, ShouldResemble, []string{"exit"})
	})
}

func BenchmarkLotIncreaseApply(b *testing.B) {
	position, _ := increaseFixture(b)
	order := &spot.AddOrderRequest{ClOrdId: "increase"}
	execution := kraken.ExecutionData{ClientOrderID: "increase", OrderStatus: "partially_filled", CumQty: mustDecimal("1"), CumCost: mustDecimal("110"), FeeUsdEquiv: mustDecimal("0.1")}
	position.Increase.basis, position.Increase.totalBasis = mustDecimal("1000"), mustDecimal("1000")
	position.Increase.order = order
	position.Increase.quantity, position.Increase.cost, position.Increase.fee = decimal.NewFromInt64(0), decimal.NewFromInt64(0), decimal.NewFromInt64(0)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := position.Increase.Apply(execution); err != nil {
			b.Fatal(err)
		}
	}
}
