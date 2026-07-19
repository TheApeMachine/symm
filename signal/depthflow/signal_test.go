package depthflow

import (
	"context"
	"iter"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
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
		PriceIncrement: krakendecimal.NewFromFloat64(0.1),
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

func depthflowFrame(rows ...kraken.BookData) *types.MarketFrame {
	return &types.MarketFrame{
		Books:        rows,
		Trades:       nil,
		CrossSection: types.NewCrossSection(),
	}
}

func TestSignal_MeasureDetectsLoadedImbalance(t *testing.T) {
	Convey("Given repeated bid-heavy book frames", t, func() {
		signal := &Signal{
			ctx:      context.Background(),
			sample:   flow.NewSample(),
			bookflow: equation.NewBookflow(),
		}

		for range 6 {
			_, err := signal.Calculate(depthflowFrame(depthflowBookRow("BTC/USD", 20, 4)))
			So(err, ShouldBeNil)
		}

		Convey("When the final frame is measured", func() {
			result, err := signal.Calculate(depthflowFrame(depthflowBookRow("BTC/USD", 24, 4)))
			So(err, ShouldBeNil)

			Convey("Then depthflow loaded score and strength should rise", func() {
				loaded, ok := measureField(result, "BTC/USD", types.MetricLoadedScore)
				So(ok, ShouldBeTrue)
				So(loaded.Raw, ShouldBeGreaterThan, 0)
				So(loaded.Maturity, ShouldBeGreaterThan, 0.85)

				strength, ok := measureField(result, "BTC/USD", types.MetricStrength)
				So(ok, ShouldBeTrue)
				So(strength.Raw, ShouldBeGreaterThan, 0)
			})
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
depthCondition builds a mature book history followed by one explicit subject
profile so the adaptive gates are learned from the path rather than hard-coded.
*/
func depthCondition(
	baselineBids []float64,
	baselineAsks []float64,
	finalBids []float64,
	finalAsks []float64,
) *tests.Market {
	const baselineFrames = 8
	start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	bids := make([][]float64, baselineFrames+2)
	asks := make([][]float64, baselineFrames+2)
	spreads := make([]int, baselineFrames+2)
	stamps := make([]time.Time, baselineFrames+2)

	for index := range baselineFrames {
		bids[index] = append([]float64(nil), baselineBids...)
		asks[index] = append([]float64(nil), baselineAsks...)
		spreads[index] = 2
		stamps[index] = start.Add(time.Duration(index) * time.Second)
	}

	for index := baselineFrames; index < len(bids); index++ {
		bids[index] = append([]float64(nil), finalBids...)
		asks[index] = append([]float64(nil), finalAsks...)
		spreads[index] = 2
		stamps[index] = start.Add(time.Duration(index) * time.Second)
	}

	return conditions.BookPath(bids, asks, spreads, stamps)
}

/*
lastDepthEpoch returns the final complete metric bundle for the subject.
*/
func lastDepthEpoch(
	measurements []*types.Measurement,
	symbol string,
) map[types.MetricType]*types.Measurement {
	var at time.Time

	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Symbol == symbol && measurement.Metric == types.MetricStrength {
			at = measurement.At
			break
		}
	}

	epoch := make(map[types.MetricType]*types.Measurement)

	for _, measurement := range measurements {
		if measurement.Source == types.SourceDepthFlow &&
			measurement.Symbol == symbol && measurement.At.Equal(at) {
			epoch[measurement.Metric] = measurement
		}
	}

	return epoch
}

/*
TestSignal_MeasureFromMarket proves depthflow separates loaded, spoofed, thin,
and neutral depth structures through the production feed.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	Convey("Given independently defined depth-distribution regimes", t, func() {
		symbol := conditions.Subject()
		neutral := lastDepthEpoch(measureMarket(t, depthCondition(
			[]float64{12, 12, 12, 12, 12},
			[]float64{8, 8, 8, 8, 8},
			[]float64{11, 10, 10, 10, 10},
			[]float64{10, 10, 10, 10, 10},
		).Frames()), symbol)
		loaded := lastDepthEpoch(measureMarket(t, depthCondition(
			[]float64{20, 20, 20, 20, 20},
			[]float64{5, 5, 5, 5, 5},
			[]float64{24, 24, 24, 24, 24},
			[]float64{4, 4, 4, 4, 4},
		).Frames()), symbol)
		spoofed := lastDepthEpoch(measureMarket(t, depthCondition(
			[]float64{12, 12, 12, 12, 12},
			[]float64{8, 8, 8, 8, 8},
			[]float64{1, 100, 100, 100, 100},
			[]float64{30, 1, 1, 1, 1},
		).Frames()), symbol)
		thin := lastDepthEpoch(measureMarket(t, depthCondition(
			[]float64{20, 20, 20, 20, 20},
			[]float64{5, 5, 5, 5, 5},
			[]float64{1, 1, 1, 100, 100},
			[]float64{1, 1, 1, 1, 1},
		).Frames()), symbol)
		metrics := []types.MetricType{
			types.MetricLoadedScore,
			types.MetricSpoofScore,
			types.MetricThinScore,
			types.MetricNeutralScore,
			types.MetricStrength,
			types.MetricValue,
		}

		Convey("Then every regime emits the complete valid metric contract", func() {
			for _, epoch := range []map[types.MetricType]*types.Measurement{
				neutral, loaded, spoofed, thin,
			} {
				for _, metric := range metrics {
					measurement := epoch[metric]
					So(measurement, ShouldNotBeNil)
					So(measurement.ValidateStruct(), ShouldBeNil)
				}
			}
		})

		Convey("Then the dominant score matches each book's economic structure", func() {
			So(neutral[types.MetricNeutralScore].Raw, ShouldBeGreaterThan, 0.9)
			So(loaded[types.MetricLoadedScore].Raw, ShouldBeGreaterThan, loaded[types.MetricNeutralScore].Raw)
			So(spoofed[types.MetricSpoofScore].Raw, ShouldBeGreaterThan, spoofed[types.MetricLoadedScore].Raw)
			So(thin[types.MetricThinScore].Raw, ShouldBeGreaterThan, thin[types.MetricLoadedScore].Raw)

			for _, epoch := range []map[types.MetricType]*types.Measurement{
				neutral, loaded, spoofed, thin,
			} {
				dominant := max(
					max(epoch[types.MetricLoadedScore].Raw, epoch[types.MetricSpoofScore].Raw),
					max(epoch[types.MetricThinScore].Raw, epoch[types.MetricNeutralScore].Raw),
				)
				So(epoch[types.MetricStrength].Raw, ShouldAlmostEqual, dominant, 1e-12)
				So(epoch[types.MetricValue].Raw, ShouldAlmostEqual, dominant, 1e-12)
			}
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	signal := &Signal{
		ctx:      context.Background(),
		sample:   flow.NewSample(),
		bookflow: equation.NewBookflow(),
	}

	for range 6 {
		if _, err := signal.Calculate(depthflowFrame(depthflowBookRow("BTC/USD", 20, 4))); err != nil {
			benchmark.Fatal(err)
		}
	}

	frame := depthflowFrame(depthflowBookRow("BTC/USD", 24, 4))

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if _, err := signal.Calculate(frame); err != nil {
			benchmark.Fatal(err)
		}
	}
}
