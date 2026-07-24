package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLedgerReserve(t *testing.T) {
	Convey("Given an empty reservation ledger", t, func() {
		ledger := NewLedger()

		Convey("It reserves cash and a slot by intent", func() {
			So(ledger.Reserve(
				"intent-1", "BTC/USD", decimal.NewFromInt64(100), true,
			), ShouldBeNil)
			So(ledger.ReservedCash().Float64(), ShouldEqual, 100)
			So(ledger.ReservedSlots(), ShouldEqual, 1)
		})

		Convey("It rejects duplicate intents", func() {
			So(ledger.Reserve(
				"dup", "ETH/USD", decimal.NewFromInt64(10), false,
			), ShouldBeNil)
			So(ledger.Reserve(
				"dup", "ETH/USD", decimal.NewFromInt64(10), false,
			), ShouldNotBeNil)
		})

		Convey("It commits and clears the claim", func() {
			So(ledger.Reserve(
				"done", "SOL/USD", decimal.NewFromInt64(25), true,
			), ShouldBeNil)
			So(ledger.Commit("done"), ShouldBeNil)
			So(ledger.ReservedCash().Sign(), ShouldEqual, 0)
			So(ledger.ReservedSlots(), ShouldEqual, 0)
		})
	})
}

func BenchmarkLedgerReserve(b *testing.B) {
	ledger := NewLedger()
	cash := decimal.NewFromInt64(1)

	b.ReportAllocs()

	for b.Loop() {
		_ = ledger.Reserve("bench", "BTC/USD", cash, true)
		_ = ledger.Release("bench")
	}
}
