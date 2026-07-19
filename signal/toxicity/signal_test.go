package toxicity

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	tradefixture "github.com/theapemachine/symm/tests/fixtures/trade"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/trader"
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

func TestSignal_CalculateUsesTradeEventTime(t *testing.T) {
	Convey("Given trades with explicit event timestamps", t, func() {
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

func TestSignal_CalculateSkipsTradeWithoutTimestamp(t *testing.T) {
	Convey("Given a trade row without event time", t, func() {
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

func TestSignal_touchHonestyEmitsCancelledQuantity(t *testing.T) {
	Convey("Given a prior touch snapshot and a retreating bid", t, func() {
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

func tradeFixturePayload() []byte {
	for frame := range tradefixture.NewFixture(tradefixture.UPDATE, 1).Frames() {
		return frame.Payload
	}

	panic("tests: trade fixture missing")
}

func measureMarket(
	t testing.TB,
	frames iter.Seq[tests.Frame],
	retreat bool,
) []*types.Measurement {
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
	level3 := websocket.New(ctx, nil, true, websocket.Level3WebSocketURL)
	api.AttachLevel3(level3)
	seedAt := time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC)
	trade := kraken.NewTrade(tradeFixturePayload())
	tradeRow := trade.Data[0]
	bidPrice := tradeRow.Price
	askPrice := decimal.NewFromFloat64(tradeRow.Price.Float64() + 0.0001)
	level3.SeedTouch(
		conditions.Subject(), &bidPrice, askPrice,
		decimal.NewFromFloat64(1000), seedAt,
	)
	instrument := broker.NewInstrument(api, broker.NewPrice(api), nil)
	api.On("instrument", instrument.On)
	market, err := trader.NewMarket(ctx, api, instrument)
	So(err, ShouldBeNil)
	t.Cleanup(market.Close)
	signal := NewSignal(ctx, api, nil)
	measurements := make([]*types.Measurement, 0)
	cutAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	tradeFrame := 0

	for frame := range frames {
		if frame.Channel == "trade" {
			if retreat && tradeFrame == 1 {
				level3.SeedTouch(
					conditions.Subject(), &bidPrice, askPrice,
					decimal.NewFromFloat64(100), seedAt.Add(time.Second),
				)
			}

			tradeFrame++
		}

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
TestSignal_MeasureFromMarket proves toxicity distinguishes executions from
cancel-driven touch retreat through the production trade feed and Level3 book.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	Convey("Given two equal trades around a Level3 touch that later retreats", t, func() {
		trade := kraken.NewTrade(tradeFixturePayload()).Data[0]
		start := time.Date(2026, 7, 17, 1, 0, 1, 0, time.UTC)
		measurements := measureMarket(t, conditions.TradePath(
			[]float64{trade.Price.Float64(), trade.Price.Float64()},
			[]float64{10, 10},
			[]string{"buy", "buy"},
			[]time.Time{start, start.Add(time.Second)},
		).Frames(), true)

		Convey("Then the tape, touch, fill, cancellation, and retreat facts are all emitted", func() {
			volume, hasVolume := latestMetric(
				measurements, types.MetricTradeVolume, types.SideNone,
			)
			fill, hasFill := latestMetric(
				measurements, types.MetricFillVolume, types.SideBuy,
			)
			bidPrice, hasBidPrice := latestMetric(
				measurements, types.MetricBestPrice, types.SideBuy,
			)
			askPrice, hasAskPrice := latestMetric(
				measurements, types.MetricBestPrice, types.SideSell,
			)
			bidTouch, hasBidTouch := latestMetric(
				measurements, types.MetricTouchQuantity, types.SideBuy,
			)
			askTouch, hasAskTouch := latestMetric(
				measurements, types.MetricTouchQuantity, types.SideSell,
			)
			bidCancelled, hasBidCancelled := latestMetric(
				measurements, types.MetricCancelledQuantity, types.SideBuy,
			)
			askCancelled, hasAskCancelled := latestMetric(
				measurements, types.MetricCancelledQuantity, types.SideSell,
			)
			retreating, hasRetreat := latestMetric(
				measurements, types.MetricRetreatingQuantity, types.SideSell,
			)

			So([]bool{
				hasVolume, hasFill, hasBidPrice, hasAskPrice,
				hasBidTouch, hasAskTouch, hasBidCancelled,
				hasAskCancelled, hasRetreat,
			}, ShouldResemble, []bool{
				true, true, true, true, true, true, true, true, true,
			})

			for _, measurement := range []*types.Measurement{
				volume, fill, bidPrice, askPrice, bidTouch, askTouch,
				bidCancelled, askCancelled, retreating,
			} {
				So(measurement.Source, ShouldEqual, types.SourceToxicity)
				So(measurement.ValidateStruct(), ShouldBeNil)
			}

			So(volume.Raw, ShouldEqual, 10)
			So(fill.Raw, ShouldAlmostEqual, trade.Price.Float64()*10, 1e-12)
			So(askPrice.Raw-bidPrice.Raw, ShouldAlmostEqual, 0.0001, 1e-12)
			So(bidTouch.Raw, ShouldEqual, 100)
			So(askTouch.Raw, ShouldEqual, 100)
			So(bidCancelled.Raw, ShouldEqual, 890)
			So(askCancelled.Raw, ShouldEqual, 900)
			So(retreating.Raw, ShouldEqual, askCancelled.Raw)
			So(*retreating.Normalized, ShouldAlmostEqual, 0.9, 1e-12)
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
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
		if _, err := signal.Calculate(frame); err != nil {
			benchmark.Fatal(err)
		}
	}
}
