package exhaust

import (
	"context"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func measureField(measurements []*types.Measurement, symbol string, metric types.MetricType) (*types.Measurement, bool) {
	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Symbol == symbol &&
			measurement.Metric == metric &&
			measurement.Source == types.SourceExhaustion &&
			measurement.Stream == types.Exhaust {
			return measurement, true
		}
	}

	return nil, false
}

func exhaustBookRow(symbol string, bidQuantity float64, askQuantity float64) kraken.BookData {
	return kraken.BookData{
		Symbol:         symbol,
		PriceIncrement: *krakendecimal.NewFromFloat64(1),
		Bids: []kraken.BookLevel{
			{Price: *krakendecimal.NewFromFloat64(100), Qty: bidQuantity},
		},
		Asks: []kraken.BookLevel{
			{Price: *krakendecimal.NewFromFloat64(101), Qty: askQuantity},
		},
		Timestamp: time.Now(),
	}
}

func TestSignal_MeasureDetectsMechanicalDecay(testingTB *testing.T) {
	Convey("Given deteriorating bid depth on repeated book frames", testingTB, func() {
		signal := &Signal{
			ctx:    context.Background(),
			book:   &Book{cache: []kraken.BookData{}},
			trade:  &Trade{cache: []kraken.TradeData{}},
			sample: algorithm.NewDecaySample(),
			decay:  equation.NewDecay(),
		}
		quantities := []float64{20, 18, 16, 14, 12, 10, 8, 6, 4}

		for _, bidQuantity := range quantities {
			signal.book.cache = append(signal.book.cache, exhaustBookRow("BTC/USD", bidQuantity, 10))
			signal.Measure(types.NewThesis(nil))
		}

		Convey("When the final frame is measured", func() {
			signal.book.cache = []kraken.BookData{
				exhaustBookRow("BTC/USD", 2, 10),
			}
			result := signal.Measure(types.NewThesis(nil))

			Convey("Then exhaust urgency and mechanical score should rise", func() {
				urgency, ok := measureField(result.Measurements, "BTC/USD", types.MetricUrgency)
				So(ok, ShouldBeTrue)
				So(urgency.Raw, ShouldBeGreaterThan, 0)
				So(urgency.Maturity, ShouldBeGreaterThan, 0.85)

				mechanical, ok := measureField(result.Measurements, "BTC/USD", types.MetricMechanical)
				So(ok, ShouldBeTrue)
				So(mechanical.Raw, ShouldBeGreaterThan, 0)

				So(len(signal.book.cache), ShouldEqual, 0)
			})
		})
	})
}

func TestSignal_MeasureSkipsBookWithoutIncrement(testingTB *testing.T) {
	Convey("Given a book row without price increment", testingTB, func() {
		signal := &Signal{
			ctx:    context.Background(),
			book:   &Book{cache: []kraken.BookData{}},
			trade:  &Trade{cache: []kraken.TradeData{}},
			sample: algorithm.NewDecaySample(),
			decay:  equation.NewDecay(),
		}
		row := exhaustBookRow("BTC/USD", 10, 10)
		row.PriceIncrement = *krakendecimal.NewFromFloat64(0)
		signal.book.cache = []kraken.BookData{row}

		result := signal.Measure(types.NewThesis(nil))

		Convey("Then it emits nothing for that symbol", func() {
			_, hasSymbol := measureField(result.Measurements, "BTC/USD", types.MetricMechanical)
			So(hasSymbol, ShouldBeFalse)
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	signal := &Signal{
		ctx:    context.Background(),
		book:   &Book{cache: []kraken.BookData{}},
		trade:  &Trade{cache: []kraken.TradeData{}},
		sample: algorithm.NewDecaySample(),
		decay:  equation.NewDecay(),
	}

	for index := range 9 {
		signal.book.cache = append(signal.book.cache, exhaustBookRow("BTC/USD", 20-float64(index)*2, 10))
		_ = signal.Measure(types.NewThesis(nil))
	}

	rows := []kraken.BookData{exhaustBookRow("BTC/USD", 2, 10)}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal.book.cache = append([]kraken.BookData(nil), rows...)
		_ = signal.Measure(types.NewThesis(nil))
	}
}
