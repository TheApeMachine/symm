package depthflow

import (
	"context"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func measureField(measurements []*types.Measurement, symbol string, metric types.MetricType) (*types.Measurement, bool) {
	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Symbol == symbol &&
			measurement.Metric == metric &&
			measurement.Source == types.SourceDepthFlow &&
			measurement.Stream == types.DepthFlow {
			return measurement, true
		}
	}

	return nil, false
}

func depthflowBookRow(symbol string, bidQuantity float64, askQuantity float64) kraken.BookData {
	return kraken.BookData{
		Symbol:         symbol,
		PriceIncrement: *krakendecimal.NewFromFloat64(0.1),
		Bids: []kraken.BookLevel{
			{Price: *krakendecimal.NewFromFloat64(100.0), Qty: bidQuantity},
			{Price: *krakendecimal.NewFromFloat64(99.9), Qty: bidQuantity},
		},
		Asks: []kraken.BookLevel{
			{Price: *krakendecimal.NewFromFloat64(100.2), Qty: askQuantity},
			{Price: *krakendecimal.NewFromFloat64(100.3), Qty: askQuantity},
		},
		Timestamp: time.Now(),
	}
}

func TestSignal_MeasureDetectsLoadedImbalance(testingTB *testing.T) {
	Convey("Given repeated bid-heavy book frames", testingTB, func() {
		signal := &Signal{
			ctx:      context.Background(),
			book:     &Book{cache: bookCache()},
			trade:    &Trade{cache: tradeCache()},
			sample:   flow.NewSample(),
			bookflow: equation.NewBookflow(),
		}

		for range 6 {
			signal.book.cache = bookCache(append(bookRows(signal.book.cache), depthflowBookRow("BTC/USD", 20, 4))...)
			signal.Measure(types.NewThesis(nil))
		}

		Convey("When the final frame is measured", func() {
			signal.book.cache = bookCache(depthflowBookRow("BTC/USD", 24, 4))
			result := signal.Measure(types.NewThesis(nil))

			Convey("Then depthflow loaded score and strength should rise", func() {
				loaded, ok := measureField(result.Measurements, "BTC/USD", types.MetricLoadedScore)
				So(ok, ShouldBeTrue)
				So(loaded.Raw, ShouldBeGreaterThan, 0)
				So(loaded.Maturity, ShouldBeGreaterThan, 0.85)

				strength, ok := measureField(result.Measurements, "BTC/USD", types.MetricStrength)
				So(ok, ShouldBeTrue)
				So(strength.Raw, ShouldBeGreaterThan, 0)

				So(len(bookRows(signal.book.cache)), ShouldEqual, 0)
			})
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	signal := &Signal{
		ctx:      context.Background(),
		book:     &Book{cache: bookCache()},
		trade:    &Trade{cache: tradeCache()},
		sample:   flow.NewSample(),
		bookflow: equation.NewBookflow(),
	}

	for range 6 {
		signal.book.cache = bookCache(append(bookRows(signal.book.cache), depthflowBookRow("BTC/USD", 20, 4))...)
		_ = signal.Measure(types.NewThesis(nil))
	}

	rows := []kraken.BookData{depthflowBookRow("BTC/USD", 24, 4)}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal.book.cache = bookCache(rows...)
		_ = signal.Measure(types.NewThesis(nil))
	}
}
