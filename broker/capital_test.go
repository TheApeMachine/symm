package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCapitalReserve(t *testing.T) {
	Convey("Given a capital reserve with a USD balance snapshot", t, func() {
		capital := NewCapital("USD")
		balances := testBalances(t, "USD", 200)
		first := PendingOrder{Symbol: "BTC/USD", Side: "buy", Notional: 140}
		second := PendingOrder{Symbol: "ETH/USD", Side: "buy", Notional: 80}

		Convey("When two buys exceed the batch reserve", func() {
			firstErr := capital.Reserve(first, balances)
			secondErr := capital.Reserve(second, balances)

			Convey("Then the second reserve is rejected without mutating balances", func() {
				cash, ok := balances.Funds("USD")
				So(firstErr, ShouldBeNil)
				So(secondErr, ShouldNotBeNil)
				So(ok, ShouldBeTrue)
				So(cash, ShouldEqual, 200)
			})
		})
	})
}

func BenchmarkCapitalReserve(benchmark *testing.B) {
	capital := NewCapital("USD")
	balances := testBalances(benchmark, "USD", 200)
	order := PendingOrder{Symbol: "BTC/USD", Side: "buy", Notional: 10}

	benchmark.ReportAllocs()
	for index := 0; index < benchmark.N; index++ {
		capital.Reset()

		if err := capital.Reserve(order, balances); err != nil {
			benchmark.Fatal(err)
		}
	}
}
