package websocket

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/smartystreets/goconvey/convey"
)

func TestLifecycleFills(t *testing.T) {
	convey.Convey("Given a market fill model", t, func() {
		lifecycle := &Lifecycle{}
		volume := decimal.NewFromFloat64(0.0002)
		totalCost := decimal.NewFromFloat64(12.82598)
		model := map[string]any{
			"action":   "market_order_filled",
			"order_id": "PAPER-00025",
			"trade_id": "PAPER-00026",
			"pair":     "BTCUSD",
			"side":     "buy",
			"volume":   volume.Float64(),
			"price":    64129.9,
			"cost":     totalCost.Float64(),
		}

		executions := lifecycle.fills(model)

		convey.Convey("It should split into two partial legs", func() {
			convey.So(len(executions), convey.ShouldEqual, 2)
		})

		convey.Convey("It should preserve total quantity and cost", func() {
			firstLeg := executions[0].Data[0]
			secondLeg := executions[1].Data[0]

			convey.So(firstLeg.OrderStatus, convey.ShouldEqual, "partially_filled")
			convey.So(secondLeg.OrderStatus, convey.ShouldEqual, "filled")

			legVolume := decimal.NewFromFloat64(firstLeg.LastQty).
				Add(decimal.NewFromFloat64(secondLeg.LastQty))

			convey.So(legVolume.Cmp(volume), convey.ShouldEqual, 0)
			convey.So(
				decimal.NewFromFloat64(firstLeg.CumQty).Cmp(
					decimal.NewFromFloat64(firstLeg.LastQty),
				),
				convey.ShouldEqual,
				0,
			)
			convey.So(
				decimal.NewFromFloat64(secondLeg.CumQty).Cmp(volume),
				convey.ShouldEqual,
				0,
			)

			legCost := (&firstLeg.Cost).Add(&secondLeg.Cost)

			convey.So(legCost.Cmp(totalCost), convey.ShouldEqual, 0)
		})
	})

	convey.Convey("Given a fill too small to split", t, func() {
		lifecycle := &Lifecycle{}
		model := map[string]any{
			"action":   "market_order_filled",
			"order_id": "PAPER-00025",
			"trade_id": "PAPER-00026",
			"pair":     "BTCUSD",
			"side":     "buy",
			"volume":   0.0001,
			"price":    64129.9,
			"cost":     6.41299,
		}

		executions := lifecycle.fills(model)

		convey.Convey("It should emit one execution", func() {
			convey.So(len(executions), convey.ShouldEqual, 1)
		})
	})

	convey.Convey("Given a zero-volume fill model", t, func() {
		lifecycle := &Lifecycle{}
		model := map[string]any{
			"action":   "market_order_filled",
			"order_id": "PAPER-00025",
			"trade_id": "PAPER-00026",
			"pair":     "BTCUSD",
			"side":     "buy",
			"volume":   0.0,
			"price":    64129.9,
			"cost":     0.0,
		}

		executions := lifecycle.fills(model)

		convey.Convey("It should not split empty fills", func() {
			convey.So(len(executions), convey.ShouldEqual, 1)
		})
	})
}

func BenchmarkLifecycleFills(b *testing.B) {
	lifecycle := &Lifecycle{}
	model := map[string]any{
		"action":   "market_order_filled",
		"order_id": "PAPER-00025",
		"trade_id": "PAPER-00026",
		"pair":     "BTCUSD",
		"side":     "buy",
		"volume":   0.0002,
		"price":    64129.9,
		"cost":     12.82598,
	}

	for b.Loop() {
		_ = lifecycle.fills(model)
	}
}
