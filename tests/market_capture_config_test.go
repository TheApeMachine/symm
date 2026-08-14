package tests

import (
	"bytes"
	"os"
	"strings"
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

func TestCaptureSymbolsFromFrames(t *testing.T) {
	Convey("Given a self-contained live market capture", t, func() {
		capture := strings.Join([]string{
			`{"endpoint":"symm_metadata","payload":{"channel":"symm_metadata","type":"market_profiles","data":[{"symbol":"BTC/USD","pair":{"altname":"XBTUSD","wsname":"XBT/USD","pair_decimals":1,"lot_decimals":8,"ordermin":"0.00005","costmin":"0.5","tick_size":"0.1"},"taker":{"fee":"0.26"},"maker":{"fee":"0.16"}}]},"received_at":"2026-08-14T00:00:00Z"}`,
			`{"endpoint":"public","payload":{"channel":"ticker","data":[{"symbol":"BTC/USD","last":63108.25,"bid":63108.2,"ask":63108.3}]},"received_at":"2026-08-14T00:00:01Z"}`,
		}, "\n")

		symbols, err := CaptureSymbolsFromFrames(strings.NewReader(capture), 10)

		Convey("It should reproduce venue rules, active fees, and observed spread", func() {
			So(err, ShouldBeNil)
			So(symbols, ShouldHaveLength, 1)
			So(symbols[0].Pair, ShouldEqual, "BTC/USD")
			So(symbols[0].StartPrice, ShouldEqual, 63108.25)
			So(symbols[0].PriceIncrement, ShouldEqual, 0.1)
			So(symbols[0].PricePrecision, ShouldEqual, 1)
			So(symbols[0].QuantityPrecision, ShouldEqual, 8)
			So(symbols[0].TakerFeePercent, ShouldEqual, 0.26)
			So(symbols[0].MakerFeePercent, ShouldEqual, 0.16)
			So(symbols[0].OrderMinimum, ShouldEqual, 0.00005)
			So(symbols[0].CostMinimum, ShouldEqual, 0.5)
			So(symbols[0].BaseSpreadFraction, ShouldAlmostEqual,
				(63108.3-63108.2)/63108.25, 1e-15)
		})
	})

	Convey("Given capture metadata with no active maker fee", t, func() {
		capture := strings.Join([]string{
			`{"endpoint":"symm_metadata","payload":{"channel":"symm_metadata","type":"market_profiles","data":[{"symbol":"BTC/USD","pair":{"pair_decimals":1,"lot_decimals":8,"ordermin":"0.00005","costmin":"0.5","tick_size":"0.1"},"taker":{"fee":"0.26"},"maker":{}}]},"received_at":"2026-08-14T00:00:00Z"}`,
			`{"endpoint":"public","payload":{"channel":"ticker","data":[{"symbol":"BTC/USD","last":63108.25,"bid":63108.2,"ask":63108.3}]},"received_at":"2026-08-14T00:00:01Z"}`,
		}, "\n")

		_, err := CaptureSymbolsFromFrames(strings.NewReader(capture), 10)

		Convey("It should reject the incomplete execution economics", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "incomplete profile for BTC/USD")
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

func BenchmarkCaptureSymbolsFromFrames(b *testing.B) {
	capture := strings.Join([]string{
		`{"endpoint":"symm_metadata","payload":{"channel":"symm_metadata","type":"market_profiles","data":[{"symbol":"BTC/USD","pair":{"altname":"XBTUSD","wsname":"XBT/USD","pair_decimals":1,"lot_decimals":8,"ordermin":"0.00005","costmin":"0.5","tick_size":"0.1"},"taker":{"fee":"0.26"},"maker":{"fee":"0.16"}}]},"received_at":"2026-08-14T00:00:00Z"}`,
		`{"endpoint":"public","payload":{"channel":"ticker","data":[{"symbol":"BTC/USD","last":63108.25,"bid":63108.2,"ask":63108.3}]},"received_at":"2026-08-14T00:00:01Z"}`,
	}, "\n")
	b.ReportAllocs()

	for b.Loop() {
		if _, err := CaptureSymbolsFromFrames(strings.NewReader(capture), 10); err != nil {
			b.Fatal(err)
		}
	}
}
