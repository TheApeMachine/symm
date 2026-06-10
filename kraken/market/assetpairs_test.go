package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFeePercentAtVolume(t *testing.T) {
	Convey("Given Kraken fee tiers", t, func() {
		tiers := [][]float64{
			{0, 0.40},
			{50_000, 0.26},
			{100_000, 0.24},
		}

		Convey("It should select the tier for the current volume", func() {
			low, err := FeePercentAtVolume(tiers, 0)

			So(err, ShouldBeNil)
			So(low, ShouldEqual, 0.40)

			mid, err := FeePercentAtVolume(tiers, 50_000)

			So(err, ShouldBeNil)
			So(mid, ShouldEqual, 0.26)

			high, err := FeePercentAtVolume(tiers, 150_000)

			So(err, ShouldBeNil)
			So(high, ShouldEqual, 0.24)
		})
	})
}

func TestPairFeeRates(t *testing.T) {
	Convey("Given one asset pair", t, func() {
		pair := &Pair{
			Wsname: "BTC/USD",
			Fees: [][]float64{
				{0, 0.26},
			},
			FeesMaker: [][]float64{
				{0, 0.16},
			},
			TickSize: "0.1",
		}

		Convey("It should expose decimal fee rates", func() {
			taker, err := pair.TakerFeeRate(0)

			So(err, ShouldBeNil)
			So(taker, ShouldAlmostEqual, 0.0026, 1e-12)

			maker, err := pair.MakerFeeRate(0)

			So(err, ShouldBeNil)
			So(maker, ShouldAlmostEqual, 0.0016, 1e-12)

			tickSize, err := pair.TickSizeFloat()

			So(err, ShouldBeNil)
			So(tickSize, ShouldEqual, 0.1)
		})
	})
}

func BenchmarkPairTakerFeeRate(b *testing.B) {
	pair := &Pair{
		Wsname: "BTC/USD",
		Fees: [][]float64{
			{0, 0.26},
			{50_000, 0.24},
		},
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = pair.TakerFeeRate(0)
	}
}
