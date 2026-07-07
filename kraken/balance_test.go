package kraken

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewBalanceDataSliceFromSpot(testingTB *testing.T) {
	Convey("Given spot REST balances", testingTB, func() {
		usd, err := decimal.NewFromString("200.25")
		So(err, ShouldBeNil)
		btc, err := decimal.NewFromString("0.1")
		So(err, ShouldBeNil)

		balances := map[string]*decimal.Decimal{
			"USD": usd,
			"BTC": btc,
		}
		rows := NewBalanceDataSliceFromSpot(balances)

		Convey("Then they should become sorted balance rows", func() {
			So(rows, ShouldHaveLength, 2)
			So(rows[0].Asset, ShouldEqual, "BTC")
			So(rows[0].Balance.String(), ShouldEqual, "0.1")
			So(rows[0].Available.String(), ShouldEqual, "0.1")
			So(rows[1].Asset, ShouldEqual, "USD")
			So(rows[1].Balance.String(), ShouldEqual, "200.25")
		})
	})
}

func BenchmarkNewBalanceDataSliceFromSpot(benchmarkTB *testing.B) {
	usd, _ := decimal.NewFromString("200.25")
	btc, _ := decimal.NewFromString("0.1")
	balances := map[string]*decimal.Decimal{
		"USD": usd,
		"BTC": btc,
	}

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		_ = NewBalanceDataSliceFromSpot(balances)
	}
}
