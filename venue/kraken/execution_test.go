package kraken

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
)

func TestNewExecution(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewExecution is called", func() {
			execution := NewExecution()

			Convey("defaults carry snap flags on the channel", func() {
				So(execution.Channel, ShouldEqual, "executions")
				So(execution.Type, ShouldEqual, "update")
				So(execution.Data, ShouldHaveLength, 0)
				req := execution.Request()
				So(req["method"], ShouldEqual, "subscribe")
				params := req["params"].(map[string]any)
				So(params["channel"], ShouldEqual, "executions")
				So(params["snap_orders"], ShouldEqual, true)
				So(params["snap_trades"], ShouldEqual, true)
			})
		})
	})
}

func TestNewExecutionFromMap(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewExecutionFromMap builds a paper fill", func() {
			execution := NewExecutionFromMap(map[string]any{
				"cl_ord_id": "exit-order-id",
				"pair":      "SLAY/USD",
				"side":      "sell",
				"exec_type": "trade",
				"volume":    0.1,
				"price":     1.25,
			}).(*Execution)

			Convey("client order id and symbol are preserved", func() {
				So(execution.Data, ShouldHaveLength, 1)
				So(execution.Data[0].ClientOrderID, ShouldEqual, "exit-order-id")
				So(execution.Data[0].Symbol, ShouldEqual, "SLAY/USD")
				So(execution.Data[0].Side, ShouldEqual, "sell")
				So(execution.Data[0].LastQty.String(), ShouldEqual, "0.1")
				So(execution.Data[0].LastPrice.String(), ShouldEqual, "1.25")
			})
		})
	})
}

func TestTradable(t *testing.T) {
	Convey("Setup", t, func() {
		qty := decimal.RequireFromString("0.1")
		execution := &Execution{
			Data: []ExecutionData{{
				ExecType: "trade",
				LastQty:  &qty,
			}},
		}

		Convey("When the row is a positive trade fill", func() {
			Convey("Tradable is true", func() {
				So(execution.Tradable(), ShouldBeTrue)
			})
		})

		Convey("When LastQty is zero", func() {
			zero := decimal.Zero
			execution.Data[0].LastQty = &zero

			Convey("Tradable is false", func() {
				So(execution.Tradable(), ShouldBeFalse)
			})
		})
	})
}

func TestMarshalJSONExecution(t *testing.T) {
	Convey("Setup", t, func() {
		price := decimal.RequireFromString("50000.5")
		qty := decimal.RequireFromString("0.1")
		cost := decimal.RequireFromString("5000.05")
		fee := decimal.RequireFromString("12.50")

		execution := &Execution{
			Channel: "executions",
			Type:    "update",
			Data: []ExecutionData{{
				Symbol:        "BTC/USD",
				OrderID:       "ord-999",
				ClientOrderID: "cl-ord-888",
				ExecID:        "exec-777",
				ExecType:      "trade",
				Side:          "buy",
				OrderType:     "limit",
				OrderStatus:   "filled",
				LastPrice:     &price,
				LastQty:       &qty,
				Cost:          &cost,
				CumQty:        &qty,
				CumCost:       &cost,
				AvgPrice:      &price,
				FeeUsdEquiv:   &fee,
			}},
		}

		Convey("When sonic round-trips MarshalJSON and UnmarshalJSON", func() {
			raw, err := sonic.Marshal(execution)
			So(err, ShouldBeNil)

			var decoded Execution
			So(sonic.Unmarshal(raw, &decoded), ShouldBeNil)

			Convey("fields survive the round trip", func() {
				So(decoded.Channel, ShouldEqual, "executions")
				So(decoded.Type, ShouldEqual, "update")
				So(decoded.Data, ShouldHaveLength, 1)
				So(decoded.Data[0].Symbol, ShouldEqual, "BTC/USD")
				So(decoded.Data[0].OrderID, ShouldEqual, "ord-999")
				So(decoded.Data[0].LastPrice.String(), ShouldEqual, "50000.5")
				So(decoded.Data[0].LastQty.String(), ShouldEqual, "0.1")
			})
		})
	})
}

func TestToMarketExecution(t *testing.T) {
	Convey("Setup", t, func() {
		price := decimal.RequireFromString("100")
		qty := decimal.RequireFromString("2")
		execution := &Execution{
			Channel: "executions",
			Type:    "update",
			Data: []ExecutionData{{
				Symbol:      "BTC/USD",
				ExecID:      "e1",
				ExecType:    "trade",
				Side:        "buy",
				LastPrice:   &price,
				LastQty:     &qty,
				OrderStatus: "filled",
			}},
		}

		Convey("When ToMarket projects into market.Execution", func() {
			book := market.NewExecution(nil)
			execution.ToMarket(book)

			Convey("fill rows land on the desk book", func() {
				So(book.Data, ShouldHaveLength, 1)
				So(book.Data[0].Symbol, ShouldEqual, "BTC/USD")
				So(book.Data[0].ExecID, ShouldEqual, "e1")
				So(book.Data[0].LastQty.String(), ShouldEqual, "2")
			})
		})
	})
}

func BenchmarkNewExecution(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = NewExecution().Request()
	}
}

func BenchmarkNewExecutionFromMap(b *testing.B) {
	model := map[string]any{
		"cl_ord_id": "exit-order-id",
		"pair":      "SLAY/USD",
		"side":      "sell",
		"exec_type": "trade",
		"volume":    0.1,
		"price":     1.25,
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = NewExecutionFromMap(model)
	}
}

func BenchmarkTradable(b *testing.B) {
	qty := decimal.RequireFromString("0.1")
	execution := &Execution{
		Data: []ExecutionData{{
			ExecType: "trade",
			LastQty:  &qty,
		}},
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = execution.Tradable()
	}
}

func BenchmarkMarshalJSONExecution(b *testing.B) {
	price := decimal.RequireFromString("50000.5")
	qty := decimal.RequireFromString("0.1")
	cost := decimal.RequireFromString("5000.05")
	fee := decimal.RequireFromString("12.50")
	execution := &Execution{
		Channel: "executions",
		Type:    "update",
		Data: []ExecutionData{{
			Symbol:        "BTC/USD",
			OrderID:       "ord-999",
			ClientOrderID: "cl-ord-888",
			ExecID:        "exec-777",
			ExecType:      "trade",
			Side:          "buy",
			OrderType:     "limit",
			OrderStatus:   "filled",
			LastPrice:     &price,
			LastQty:       &qty,
			Cost:          &cost,
			CumQty:        &qty,
			CumCost:       &cost,
			AvgPrice:      &price,
			FeeUsdEquiv:   &fee,
		}},
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = sonic.Marshal(execution)
	}
}

func BenchmarkToMarketExecution(b *testing.B) {
	price := decimal.RequireFromString("100")
	qty := decimal.RequireFromString("2")
	execution := &Execution{
		Channel: "executions",
		Type:    "update",
		Data: []ExecutionData{{
			Symbol:      "BTC/USD",
			ExecID:      "e1",
			ExecType:    "trade",
			Side:        "buy",
			LastPrice:   &price,
			LastQty:     &qty,
			OrderStatus: "filled",
		}},
	}
	book := market.NewExecution(nil)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		execution.ToMarket(book)
	}
}
