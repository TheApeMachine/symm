package depthflow

import (
	"context"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

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
			book:     &Book{cache: []kraken.BookData{}},
			trade:    &Trade{cache: []kraken.TradeData{}},
			sample:   flow.NewSample(),
			bookflow: equation.NewBookflow(),
		}

		for range 6 {
			signal.book.cache = append(signal.book.cache, depthflowBookRow("BTC/USD", 20, 4))
			signal.Measure(types.NewThesis(nil))
		}

		Convey("When the final frame is measured", func() {
			signal.book.cache = []kraken.BookData{depthflowBookRow("BTC/USD", 24, 4)}
			result := signal.Measure(types.NewThesis(nil))

			Convey("Then depthflow loaded score and strength should rise", func() {
				raw, ok := result.Measurements.Load("depthflow")
				So(ok, ShouldBeTrue)

				metrics := raw.(datura.Map[datura.Map[*krakendecimal.Decimal]])["BTC/USD"]
				So(metrics["loadedScore"].Float64(), ShouldBeGreaterThan, 0)
				So(metrics["strength"].Float64(), ShouldBeGreaterThan, 0)
				So(metrics["maturity"].Float64(), ShouldBeGreaterThan, 0.85)
				So(len(signal.book.cache), ShouldEqual, 0)
			})
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	signal := &Signal{
		ctx:      context.Background(),
		book:     &Book{cache: []kraken.BookData{}},
		trade:    &Trade{cache: []kraken.TradeData{}},
		sample:   flow.NewSample(),
		bookflow: equation.NewBookflow(),
	}

	for range 6 {
		signal.book.cache = append(signal.book.cache, depthflowBookRow("BTC/USD", 20, 4))
		_ = signal.Measure(types.NewThesis(nil))
	}

	rows := []kraken.BookData{depthflowBookRow("BTC/USD", 24, 4)}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal.book.cache = append([]kraken.BookData(nil), rows...)
		_ = signal.Measure(types.NewThesis(nil))
	}
}
