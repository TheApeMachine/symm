package exhaust

import (
	"context"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm"
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

func TestSignal_MeasureDetectsMechanicalDecay(testingTB *testing.T) {
	Convey("Given deteriorating bid depth on repeated book frames", testingTB, func() {
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

func TestSignal_MeasureSkipsBookWithoutIncrement(testingTB *testing.T) {
	Convey("Given a book row without price increment", testingTB, func() {
		signal := newExhaustSignal()
		row := exhaustBookRow("BTC/USD", 10, 10)
		row.PriceIncrement = *krakendecimal.NewFromFloat64(0)

		result, err := signal.Calculate(exhaustFrame(row))
		So(err, ShouldBeNil)

		Convey("Then it emits nothing for that symbol", func() {
			_, hasSymbol := measureField(result, "BTC/USD", types.MetricMechanical)
			So(hasSymbol, ShouldBeFalse)
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
	Convey("Given exhaust inside a paper Session market", testingTB, func() {
		calmSession, err := tests.NewSession(context.Background(), testingTB, tests.SessionOptions{
			Signals: sessionSignals,
		})
		So(err, ShouldBeNil)
		decaySession, err := tests.NewSession(context.Background(), testingTB, tests.SessionOptions{
			Signals: sessionSignals,
		})
		So(err, ShouldBeNil)

		Convey("When calm and decaying books play through Cut", func() {
			calmTheses, err := calmSession.Play(conditions.Calm(24).Frames())
			So(err, ShouldBeNil)
			decayTheses, err := decaySession.Play(
				conditions.Decay(24, 0, 0.9).Frames(),
			)
			So(err, ShouldBeNil)

			calm, hasCalm := tests.PeakSourceMetric(
				calmTheses,
				types.SourceExhaustion,
				conditions.Subject(),
				types.MetricMechanical,
			)
			decayed, hasDecay := tests.PeakSourceMetric(
				decayTheses,
				types.SourceExhaustion,
				conditions.Subject(),
				types.MetricMechanical,
			)

			Convey("Then book decay lifts mechanical exhaust versus calm", func() {
				So(hasCalm, ShouldBeTrue)
				So(hasDecay, ShouldBeTrue)
				So(decayed, ShouldBeGreaterThan, calm)
			})
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	signal := newExhaustSignal()

	books := make([]kraken.BookData, 0, 9)
	for index := range 9 {
		books = append(books, exhaustBookRow("BTC/USD", 20-float64(index)*2, 10))
	}
	_, _ = signal.Calculate(exhaustFrame(books...))

	frame := exhaustFrame(exhaustBookRow("BTC/USD", 2, 10))

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _ = signal.Calculate(frame)
	}
}
