package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPendingBookUpdate(testingTB *testing.T) {
	Convey("Given a pending order", testingTB, func() {
		book := NewPendingBook()
		So(book.Add(PendingOrder{ClOrdID: "abc", Symbol: "BTC/USD"}), ShouldBeTrue)

		Convey("When a terminal execution update arrives", func() {
			book.Update(map[string]any{
				"channel": "executions",
				"data": []map[string]any{{
					"cl_ord_id":    "abc",
					"order_status": "filled",
				}},
			})

			Convey("Then the pending order is removed", func() {
				So(book.Count(), ShouldEqual, 0)
			})
		})
	})
}

func BenchmarkPendingBookUpdate(benchmarkTB *testing.B) {
	book := NewPendingBook()
	frame := map[string]any{
		"channel": "executions",
		"data": []any{
			map[string]any{
				"cl_ord_id":    "abc",
				"order_status": "filled",
			},
		},
	}

	benchmarkTB.ReportAllocs()
	for index := 0; index < benchmarkTB.N; index++ {
		if err := book.Update(frame); err != nil {
			benchmarkTB.Fatal(err)
		}
	}
}
