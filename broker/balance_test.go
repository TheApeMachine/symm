package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBalanceBookFunds(testingTB *testing.T) {
	Convey("Given a balance book with a zero USD row", testingTB, func() {
		book := NewBalanceBook()
		frame := map[string]any{
			"channel": "balances",
			"data": []map[string]any{{
				"asset":   "USD",
				"balance": 0.0,
			}},
		}

		Convey("When the balance frame is observed", func() {
			err := book.Update(frame)
			funds, ok := book.Funds("USD")
			_, missingErr := book.RequireFunds("EUR")

			Convey("Then zero funds are distinct from a missing asset", func() {
				So(err, ShouldBeNil)
				So(ok, ShouldBeTrue)
				So(funds, ShouldEqual, 0)
				So(missingErr, ShouldNotBeNil)
			})
		})
	})
}

func TestBalanceBookHoldings(testingTB *testing.T) {
	Convey("Given a balance book with multiple assets", testingTB, func() {
		book := NewBalanceBook()
		frame := map[string]any{
			"channel": "balances",
			"data": []map[string]any{
				{"asset": "USD", "balance": 100.0},
				{"asset": "BTC", "balance": 0.25},
			},
		}

		Convey("When holdings are requested", func() {
			err := book.Update(frame)
			holdings, holdingsErr := book.Holdings()

			Convey("Then typed balance rows are returned", func() {
				So(err, ShouldBeNil)
				So(holdingsErr, ShouldBeNil)
				So(holdings.Rows, ShouldHaveLength, 2)
			})
		})
	})
}

func BenchmarkBalanceBookUpdate(benchmarkTB *testing.B) {
	book := NewBalanceBook()
	frame := map[string]any{
		"channel": "balances",
		"data": []any{
			map[string]any{"asset": "USD", "balance": 200.0},
			map[string]any{"asset": "BTC", "balance": 0.25},
		},
	}

	benchmarkTB.ReportAllocs()
	for index := 0; index < benchmarkTB.N; index++ {
		if err := book.Update(frame); err != nil {
			benchmarkTB.Fatal(err)
		}
	}
}
