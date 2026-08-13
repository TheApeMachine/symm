package tests

import (
	"bytes"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCaptureSymbols(t *testing.T) {
	Convey("Given untouched pair metadata and ticker captures", t, func() {
		captureDirectory := "/Users/theapemachine/.symm/data/backtests/" +
			"kraken/2026-08-13-live-exact-v2/"
		pairs, err := os.Open(captureDirectory + "pairs.json")
		So(err, ShouldBeNil)
		defer pairs.Close()
		tickers, err := os.Open(captureDirectory + "ticker.jsonl")
		So(err, ShouldBeNil)
		defer tickers.Close()

		symbols, err := CaptureSymbols(pairs, tickers, 10)

		Convey("It should reconstruct every observed USD instrument", func() {
			So(err, ShouldBeNil)
			So(symbols, ShouldNotBeEmpty)
			So(symbols[0].Pair, ShouldEqual, "ACU/USD")
			var foundAKE bool

			for _, symbol := range symbols {
				if symbol.Pair != "AKE/USD" {
					continue
				}

				foundAKE = true
				So(symbol.StartPrice, ShouldEqual, 0.00578119)
				So(symbol.PriceIncrement, ShouldEqual, 0.00000001)
				So(symbol.PricePrecision, ShouldEqual, 8)
				So(symbol.QuantityPrecision, ShouldEqual, 5)
				So(symbol.OrderMinimum, ShouldEqual, 1100.0)
				So(symbol.CostMinimum, ShouldEqual, 0.5)
				So(symbol.TakerFeePercent, ShouldEqual, 0.4)
				So(symbol.MakerFeePercent, ShouldEqual, 0.23)
			}

			So(foundAKE, ShouldBeTrue)
		})
	})
}

func BenchmarkCaptureSymbols(b *testing.B) {
	captureDirectory := "/Users/theapemachine/.symm/data/backtests/" +
		"kraken/2026-08-13-live-exact-v2/"
	pairs, err := os.ReadFile(captureDirectory + "pairs.json")

	if err != nil {
		b.Fatal(err)
	}

	tickers, err := os.ReadFile(captureDirectory + "ticker.jsonl")

	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err = CaptureSymbols(
			bytes.NewReader(pairs),
			bytes.NewReader(tickers),
			10,
		); err != nil {
			b.Fatal(err)
		}
	}
}
