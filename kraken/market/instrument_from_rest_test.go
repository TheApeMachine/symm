package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInstrumentPairFromREST(t *testing.T) {
	Convey("Given Kraken AssetPairs REST metadata", t, func() {
		pair, err := InstrumentPairFromREST(&Pair{
			Wsname:       "BTC/EUR",
			Base:         "XXBT",
			Quote:        "ZEUR",
			LotDecimals:  8,
			PairDecimals: 1,
			CostDecimals: 5,
			Ordermin:     "0.0001",
			Costmin:      "0.45",
			TickSize:     "0.1",
			Status:       "online",
		})

		Convey("It should map sizing and increment fields", func() {
			So(err, ShouldBeNil)
			So(pair.Symbol, ShouldEqual, "BTC/EUR")
			So(pair.QtyMin, ShouldAlmostEqual, 0.0001, 1e-12)
			So(pair.CostMin, ShouldAlmostEqual, 0.45, 1e-12)
			So(pair.PriceIncrement, ShouldAlmostEqual, 0.1, 1e-12)
			So(pair.QtyIncrement, ShouldAlmostEqual, 1e-8, 1e-12)
			So(pair.Status, ShouldEqual, "online")
		})
	})

	Convey("Given a pair without wsname", t, func() {
		_, err := InstrumentPairFromREST(&Pair{Base: "XXBT"})

		Convey("It should return an error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkInstrumentPairFromREST(b *testing.B) {
	restPair := &Pair{
		Wsname:       "BTC/EUR",
		Base:         "XXBT",
		Quote:        "ZEUR",
		LotDecimals:  8,
		PairDecimals: 1,
		CostDecimals: 5,
		Ordermin:     "0.0001",
		Costmin:      "0.45",
		TickSize:     "0.1",
		Status:       "online",
	}

	for b.Loop() {
		_, err := InstrumentPairFromREST(restPair)

		if err != nil {
			b.Fatal(err)
		}
	}
}
