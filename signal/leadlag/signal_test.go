package leadlag

import (
	"context"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given repeated multi-symbol lead-lag cuts", t, func() {
		signal := &Signal{ctx: context.Background(), section: NewSection()}
		start := time.Unix(1_700_007_000, 0).UTC()

		Reset(func() {
			signal.Close()
		})

		for leg, prices := range [][]float64{
			{100, 100, 100},
			{110, 101, 99},
			{121, 102, 98},
		} {
			at := start.Add(time.Duration(leg) * time.Second)
			thesis := types.NewThesis(t.Context(), nil)

			for index, symbol := range []string{"AAA/USD", "BBB/USD", "CCC/USD"} {
				thesis.Tickers.Store(symbol, []kraken.TickerData{{
					Symbol:    symbol,
					Last:      decimal.NewFromFloat64(prices[index]),
					Timestamp: at,
				}})
			}

			measurements := signal.Measure(thesis)

			if leg == 0 {
				So(measurements, ShouldHaveLength, 3)
			}

			if leg == 2 {
				So(measurements, ShouldHaveLength, 3)
			}
		}

		Convey("It retains every symbol history across measurement passes", func() {
			for _, symbol := range []string{"AAA/USD", "BBB/USD", "CCC/USD"} {
				So(signal.section.PriceSampleCount(symbol), ShouldEqual, 3)
			}
		})
	})

	Convey("Given a retained anchor without a ticker in the current frame", t, func() {
		signal := &Signal{ctx: context.Background(), section: NewSection()}
		start := time.Unix(1_700_008_000, 0).UTC()

		for _, sample := range []struct {
			symbol string
			first  float64
			second float64
		}{
			{symbol: "AAA/USD", first: 100, second: 120},
			{symbol: "BBB/USD", first: 100, second: 105},
			{symbol: "CCC/USD", first: 100, second: 101},
		} {
			So(signal.section.ObservePrice(sample.symbol, sample.first, start),
				ShouldBeTrue)
			So(signal.section.ObservePrice(
				sample.symbol, sample.second, start.Add(time.Second),
			), ShouldBeTrue)
		}

		measurements := signal.measureFrame([]kraken.TickerData{{
			Symbol: "BBB/USD", Last: decimal.NewFromFloat64(106),
			Timestamp: start.Add(2 * time.Second),
		}})

		Convey("It should emit the exact retained anchor endpoint", func() {
			So(measurements, ShouldHaveLength, 2)
			var anchor *types.Measurement
			var follower *types.Measurement

			for _, measurement := range measurements {
				if measurement.Symbol == "AAA/USD" {
					anchor = measurement
				}

				if measurement.Symbol == "BBB/USD" {
					follower = measurement
				}
			}

			So(anchor, ShouldNotBeNil)
			So(anchor.Peer, ShouldBeEmpty)
			So(anchor.At, ShouldEqual, start.Add(time.Second))
			So(anchor.Sample(types.MetricLastPrice, types.SideNone).Raw,
				ShouldEqual, 120.0)
			So(follower, ShouldNotBeNil)
			So(follower.Peer, ShouldEqual, "AAA/USD")
		})
	})
}

func BenchmarkMeasure(b *testing.B) {
	signal := &Signal{ctx: context.Background(), section: NewSection()}
	at := time.Unix(1_700_009_000, 0).UTC()

	if !signal.section.ObservePrice("AAA/USD", 100, at) ||
		!signal.section.ObservePrice("AAA/USD", 120, at.Add(time.Second)) {
		b.Fatal("failed to seed anchor prices")
	}

	tickers := make([]kraken.TickerData, 64)

	for index := range tickers {
		symbol := "SIM" + strconv.Itoa(index) + "/USD"

		if !signal.section.ObservePrice(symbol, 100, at) ||
			!signal.section.ObservePrice(symbol, 101, at.Add(time.Second)) {
			b.Fatal("failed to seed follower prices")
		}

		tickers[index] = kraken.TickerData{
			Symbol: symbol,
			Last:   decimal.NewFromFloat64(102),
		}
	}

	at = at.Add(time.Second)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		at = at.Add(time.Second)

		for index := range tickers {
			tickers[index].Timestamp = at
		}

		_ = signal.measureFrame(tickers)
	}
}
