package depthflow

import (
	"context"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
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

func depthflowFrame(rows ...kraken.BookData) *types.MarketFrame {
	return &types.MarketFrame{
		Books:        rows,
		Trades:       nil,
		CrossSection: types.NewCrossSection(),
	}
}

func TestSignal_MeasureDetectsLoadedImbalance(testingTB *testing.T) {
	Convey("Given repeated bid-heavy book frames", testingTB, func() {
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

func sessionSignals(
	ctx context.Context,
	api *websocket.API,
	instrument *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{NewSignal(ctx, api, instrument, channel)}
}

func TestSignal_MeasureFromMarket(testingTB *testing.T) {
	Convey("Given depthflow inside a paper Session market", testingTB, func() {
		calmSession, err := tests.NewSession(context.Background(), testingTB, tests.SessionOptions{
			Signals: sessionSignals,
		})
		So(err, ShouldBeNil)
		loadSession, err := tests.NewSession(context.Background(), testingTB, tests.SessionOptions{
			Signals: sessionSignals,
		})
		So(err, ShouldBeNil)

		Convey("When balanced and bid-loaded books play through Cut", func() {
			// Calm UPDATE books are bid-only and look "loaded"; compare two-sided
			// balanced snapshots against an intentional bid overweight instead.
			calmTheses, err := calmSession.Play(
				conditions.Imbalance(24, 0, 1, 1).Frames(),
			)
			So(err, ShouldBeNil)
			loadTheses, err := loadSession.Play(
				conditions.Imbalance(24, 0, 6, 0.15).Frames(),
			)
			So(err, ShouldBeNil)

			balanced, hasBalanced := tests.PeakSourceMetric(
				calmTheses,
				types.SourceDepthFlow,
				conditions.Subject(),
				types.MetricStrength,
			)
			loaded, hasLoaded := tests.PeakSourceMetric(
				loadTheses,
				types.SourceDepthFlow,
				conditions.Subject(),
				types.MetricStrength,
			)

			Convey("Then bid-loaded books lift strength versus balanced depth", func() {
				So(hasLoaded, ShouldBeTrue)
				So(hasBalanced, ShouldBeTrue)
				So(loaded, ShouldBeGreaterThan, balanced)
			})
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
		_, _ = signal.Calculate(depthflowFrame(depthflowBookRow("BTC/USD", 20, 4)))
	}

	frame := depthflowFrame(depthflowBookRow("BTC/USD", 24, 4))

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _ = signal.Calculate(frame)
	}
}
