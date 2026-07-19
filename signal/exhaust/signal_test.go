package exhaust

import (
	"context"
	"iter"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/trader"
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
		PriceIncrement: krakendecimal.NewFromFloat64(1),
		Bids: []kraken.BookLevel{
			{Price: *krakendecimal.NewFromFloat64(100), Qty: bidQuantity},
		},
		Asks: []kraken.BookLevel{
			{Price: *krakendecimal.NewFromFloat64(101), Qty: askQuantity},
		},
		Timestamp: time.Now(),
	}
}

func newExhaustSignal() *Signal {
	return &Signal{
		ctx:    context.Background(),
		sample: algorithm.NewDecaySample(),
		decay:  equation.NewDecay(),
	}
}

func exhaustFrame(books ...kraken.BookData) *types.MarketFrame {
	return &types.MarketFrame{
		Books:        books,
		Trades:       nil,
		CrossSection: types.NewCrossSection(),
	}
}

func TestSignal_MeasureDetectsMechanicalDecay(t *testing.T) {
	Convey("Given deteriorating bid depth on repeated book frames", t, func() {
		signal := newExhaustSignal()
		quantities := []float64{20, 18, 16, 14, 12, 10, 8, 6, 4, 2}

		books := make([]kraken.BookData, 0, len(quantities))
		for _, bidQuantity := range quantities {
			books = append(books, exhaustBookRow("BTC/USD", bidQuantity, 10))
		}

		Convey("When the deteriorating frames are measured", func() {
			result, err := signal.Calculate(exhaustFrame(books...))
			So(err, ShouldBeNil)

			Convey("Then exhaust urgency and mechanical score should rise", func() {
				urgency, ok := measureField(result, "BTC/USD", types.MetricUrgency)
				So(ok, ShouldBeTrue)
				So(urgency.Raw, ShouldBeGreaterThan, 0)
				So(urgency.Maturity, ShouldBeGreaterThan, 0.85)

				mechanical, ok := measureField(result, "BTC/USD", types.MetricMechanical)
				So(ok, ShouldBeTrue)
				So(mechanical.Raw, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestSignal_MeasureSkipsBookWithoutIncrement(t *testing.T) {
	Convey("Given a book row without price increment", t, func() {
		signal := newExhaustSignal()
		row := exhaustBookRow("BTC/USD", 10, 10)
		row.PriceIncrement = krakendecimal.NewFromFloat64(0)

		result, err := signal.Calculate(exhaustFrame(row))
		So(err, ShouldBeNil)

		Convey("Then it emits nothing for that symbol", func() {
			_, hasSymbol := measureField(result, "BTC/USD", types.MetricMechanical)
			So(hasSymbol, ShouldBeFalse)
		})
	})
}

func measureMarket(t testing.TB, frames iter.Seq[tests.Frame]) []*types.Measurement {
	t.Helper()
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	viper.Set("signals.feed_timeline_capacity", 128)
	viper.Set("signals.feed_track_capacity", 128)
	t.Cleanup(func() {
		viper.Set("signals.feed_timeline_capacity", previousTimeline)
		viper.Set("signals.feed_track_capacity", previousTrack)
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	mock := mockapi.NewMockAPI()
	api := websocket.NewAPI(ctx, mock.Public(), mock.Private(), nil)
	t.Cleanup(api.Close)
	instrument := broker.NewInstrument(api, broker.NewPrice(api), nil)
	api.On("instrument", instrument.On)
	market, err := trader.NewMarket(ctx, api, instrument)
	So(err, ShouldBeNil)
	t.Cleanup(market.Close)
	signal := NewSignal(ctx, api, instrument, nil)
	measurements := make([]*types.Measurement, 0)
	cutAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for frame := range frames {
		mock.Emit(frame.Channel, frame.Payload)
		cut, cutErr := market.Cut(cutAt)
		So(cutErr, ShouldBeNil)
		cutAt = cutAt.Add(time.Second)

		if cut.IsEmpty() {
			continue
		}

		if types.SignalInterest(signal)&types.FrameInterest(cut) == 0 {
			continue
		}

		measurements = append(
			measurements,
			signal.Measure(types.NewThesis(nil, cut))...,
		)
	}

	return measurements
}

/*
lastExhaustEpoch returns the final complete subject bundle from one scenario.
*/
func lastExhaustEpoch(
	measurements []*types.Measurement,
	symbol string,
) map[types.MetricType]*types.Measurement {
	var at time.Time

	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Symbol == symbol && measurement.Metric == types.MetricUrgency {
			at = measurement.At
			break
		}
	}

	epoch := make(map[types.MetricType]*types.Measurement)

	for _, measurement := range measurements {
		if measurement.Source == types.SourceExhaustion &&
			measurement.Symbol == symbol && measurement.At.Equal(at) {
			epoch[measurement.Metric] = measurement
		}
	}

	return epoch
}

/*
reversalCondition establishes bid support and then flips the same book to ask
pressure, giving reversal evidence its required prior context.
*/
func reversalCondition() *tests.Market {
	const baselineFrames = 8
	start := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	bids := make([][]float64, baselineFrames+2)
	asks := make([][]float64, baselineFrames+2)
	spreads := make([]int, baselineFrames+2)
	stamps := make([]time.Time, baselineFrames+2)

	for index := range bids {
		bids[index] = []float64{20, 20, 20}
		asks[index] = []float64{5, 5, 5}
		spreads[index] = 2
		stamps[index] = start.Add(time.Duration(index) * time.Second)
	}

	for index := baselineFrames; index < len(bids); index++ {
		bids[index] = []float64{4, 4, 4}
		asks[index] = []float64{24, 24, 24}
	}

	return conditions.BookPath(bids, asks, spreads, stamps)
}

/*
TestSignal_MeasureFromMarket proves exhaust separates mechanical collapse,
spread fragility, pressure fade, and active reversal through production input.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	Convey("Given independently defined exit-risk regimes", t, func() {
		symbol := conditions.Subject()
		mechanical := lastExhaustEpoch(measureMarket(t, conditions.MarketPath(
			[]float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
			[]float64{10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10},
			[]float64{0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2},
			[]float64{100, 100, 100, 100, 100, 100, 100, 100, 50, 25, 10},
		).Frames()), symbol)
		fragile := lastExhaustEpoch(measureMarket(t, conditions.MarketPath(
			[]float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
			[]float64{10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10},
			[]float64{0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.4, 0.8, 1.6},
			[]float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
		).Frames()), symbol)
		thermal := lastExhaustEpoch(measureMarket(t, conditions.MarketPath(
			[]float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
			[]float64{100, 100, 100, 100, 100, 100, 100, 1, 1, 1, 1},
			[]float64{0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2},
			[]float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
		).Frames()), symbol)
		reversal := lastExhaustEpoch(
			measureMarket(t, reversalCondition().Frames()), symbol,
		)
		metrics := []types.MetricType{
			types.MetricMechanical,
			types.MetricThermal,
			types.MetricFragile,
			types.MetricReversal,
			types.MetricUrgency,
			types.MetricStrength,
			types.MetricValue,
			types.MetricCategory,
		}

		Convey("Then every regime emits the complete valid metric contract", func() {
			for _, epoch := range []map[types.MetricType]*types.Measurement{
				mechanical, fragile, thermal, reversal,
			} {
				for _, metric := range metrics {
					measurement := epoch[metric]
					So(measurement, ShouldNotBeNil)
					So(measurement.ValidateStruct(), ShouldBeNil)
				}
			}
		})

		Convey("Then each path selects the exit risk it actually contains", func() {
			So(mechanical[types.MetricCategory].Raw, ShouldEqual, 1)
			So(mechanical[types.MetricMechanical].Raw, ShouldBeGreaterThan, 0)
			So(fragile[types.MetricCategory].Raw, ShouldEqual, 2)
			So(fragile[types.MetricFragile].Raw, ShouldBeGreaterThan, 0)
			So(thermal[types.MetricCategory].Raw, ShouldEqual, 3)
			So(thermal[types.MetricThermal].Raw, ShouldBeGreaterThan, 0)
			So(reversal[types.MetricCategory].Raw, ShouldEqual, 4)
			So(reversal[types.MetricReversal].Raw, ShouldBeGreaterThan, 0)

			for _, epoch := range []map[types.MetricType]*types.Measurement{
				mechanical, fragile, thermal, reversal,
			} {
				So(epoch[types.MetricUrgency].Raw, ShouldBeGreaterThan, 0)
				So(epoch[types.MetricStrength].Raw, ShouldAlmostEqual, epoch[types.MetricUrgency].Raw, 1e-12)
				So(epoch[types.MetricValue].Raw, ShouldAlmostEqual, epoch[types.MetricUrgency].Raw, 1e-12)
			}
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	signal := newExhaustSignal()

	books := make([]kraken.BookData, 0, 9)
	for index := range 9 {
		books = append(books, exhaustBookRow("BTC/USD", 20-float64(index)*2, 10))
	}
	if _, err := signal.Calculate(exhaustFrame(books...)); err != nil {
		benchmark.Fatal(err)
	}

	frame := exhaustFrame(exhaustBookRow("BTC/USD", 2, 10))

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if _, err := signal.Calculate(frame); err != nil {
			benchmark.Fatal(err)
		}
	}
}
