package toxicity

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

/*
frameFrom builds an immutable market cut from raw trade rows, mirroring the
central feed so Calculate consumes explicit evidence.
*/
func frameFrom(rows ...kraken.TradeData) *types.MarketFrame {
	return &types.MarketFrame{
		Trades:       rows,
		CrossSection: types.NewCrossSection(),
	}
}

func newSignal() *Signal {
	return &Signal{
		ctx:        context.Background(),
		level3:     NewLevel3(websocket.NewAPI(context.Background(), nil, nil, nil)),
		priorTouch: map[string]touchSnapshot{},
	}
}

func TestSignal_CalculateUsesTradeEventTime(testingTB *testing.T) {
	Convey("Given trades with explicit event timestamps", testingTB, func() {
		eventAt := time.Unix(42, 500_000_000).UTC()
		signal := newSignal()

		measurements, err := signal.Calculate(frameFrom(kraken.TradeData{
			Symbol:    "BTC/USD",
			Side:      "buy",
			Price:     *decimal.NewFromFloat64(100),
			Qty:       1,
			Timestamp: eventAt,
		}))

		Convey("Then measurements use the trade event time and typed subjects", func() {
			So(err, ShouldBeNil)
			So(measurements, ShouldNotBeEmpty)

			volume := measurements[0]
			So(volume.At, ShouldEqual, eventAt)
			So(volume.Subject, ShouldEqual, types.SubjectLevel3Tape)
			So(volume.Maturity, ShouldBeGreaterThan, 0)
			So(volume.Validity.State, ShouldEqual, types.ValidityProvisional)
			So(volume.Validity.Readiness, ShouldEqual, types.ReadinessObservation)
			So(volume.Normalized, ShouldBeNil)
			So(volume.Scale.Kind, ShouldEqual, types.ScaleObservationWindow)
		})
	})
}

func TestSignal_CalculateSkipsTradeWithoutTimestamp(testingTB *testing.T) {
	Convey("Given a trade row without event time", testingTB, func() {
		signal := newSignal()

		measurements, err := signal.Calculate(frameFrom(kraken.TradeData{
			Symbol: "BTC/USD",
			Price:  *decimal.NewFromFloat64(100),
			Qty:    1,
		}))

		Convey("Then it ignores it instead of inventing wall-clock time", func() {
			So(err, ShouldBeNil)
			So(measurements, ShouldBeEmpty)
		})
	})
}

func TestSignal_touchHonestyEmitsCancelledQuantity(testingTB *testing.T) {
	Convey("Given a prior touch snapshot and a retreating bid", testingTB, func() {
		eventAt := time.Unix(50, 0).UTC()
		signal := &Signal{}
		row := &symbolEvidence{
			latestAt:   eventAt,
			tradeCount: 1,
		}
		prior := touchSnapshot{
			bidQuantity: 5,
			askQuantity: 3,
			observedAt:  time.Unix(49, 0),
		}

		measurements := signal.touchHonesty(
			"BTC/USD", row, prior, 2, 3, 0.5,
		)

		Convey("Then it emits cancelled and retreating touch evidence", func() {
			cancelled, ok := latestMetric(measurements, types.MetricCancelledQuantity, types.SideBuy)
			So(ok, ShouldBeTrue)
			So(cancelled.Raw, ShouldEqual, 3)
			So(cancelled.Validity.State, ShouldEqual, types.ValidityProvisional)
			So(cancelled.Normalized, ShouldNotBeNil)
			So(*cancelled.Normalized, ShouldAlmostEqual, 0.6, 1e-9)

			retreating, ok := latestMetric(measurements, types.MetricRetreatingQuantity, types.SideBuy)
			So(ok, ShouldBeTrue)
			So(retreating.Raw, ShouldEqual, 3)
			So(retreating.At, ShouldEqual, eventAt)
		})
	})
}

func latestMetric(
	measurements []*types.Measurement,
	metric types.MetricType,
	side types.MeasurementSide,
) (*types.Measurement, bool) {
	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Metric == metric && measurement.Side == side {
			return measurement, true
		}
	}

	return nil, false
}

func sessionSignals(
	ctx context.Context,
	api *websocket.API,
	_ *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{NewSignal(ctx, api, channel)}
}

func TestSignal_MeasureFromMarket(testingTB *testing.T) {
	Convey("Given toxicity inside a paper Session with Level3 touch", testingTB, func() {
		calmSession, err := tests.NewSession(testingTB, tests.SessionOptions{
			Signals: sessionSignals,
			Level3:  true,
		})
		So(err, ShouldBeNil)
		hotSession, err := tests.NewSession(testingTB, tests.SessionOptions{
			Signals: sessionSignals,
			Level3:  true,
		})
		So(err, ShouldBeNil)

		calmSession.SeedTouch(conditions.Subject(), 0.56, 0.57, 1000)
		hotSession.SeedTouch(conditions.Subject(), 0.56, 0.57, 1000)

		Convey("When calm and aggression tapes play through Cut", func() {
			calmTheses, err := calmSession.Play(conditions.Calm(24).Frames())
			So(err, ShouldBeNil)
			hotTheses, err := hotSession.Play(
				conditions.Aggression(24, 4, 8).Frames(),
			)
			So(err, ShouldBeNil)

			calm, hasCalm := tests.PeakSourceMetric(
				calmTheses,
				types.SourceToxicity,
				conditions.Subject(),
				types.MetricTradeVolume,
			)
			hot, hasHot := tests.PeakSourceMetric(
				hotTheses,
				types.SourceToxicity,
				conditions.Subject(),
				types.MetricTradeVolume,
			)

			Convey("Then aggression lifts trade-volume evidence under PeekBook", func() {
				So(hasHot, ShouldBeTrue)

				if hasCalm {
					So(hot, ShouldBeGreaterThan, calm)
				}
			})
		})
	})
}

func BenchmarkSignal_Calculate(benchmark *testing.B) {
	benchmark.ReportAllocs()

	eventAt := time.Unix(1, 0).UTC()
	signal := newSignal()
	frame := frameFrom(kraken.TradeData{
		Symbol:    "BTC/USD",
		Timestamp: eventAt,
		Price:     *decimal.NewFromFloat64(100),
		Qty:       1,
	})

	for benchmark.Loop() {
		_, _ = signal.Calculate(frame)
	}
}
