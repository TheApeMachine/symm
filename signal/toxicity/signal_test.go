package toxicity

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	tradefixture "github.com/theapemachine/symm/tests/fixtures/trade"
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

func sessionSignals(
	ctx context.Context,
	api *websocket.API,
	_ *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{NewSignal(ctx, api, channel)}
}

/*
seedToxicitySession boots a Level3 Session and seeds a two-sided touch so
toxicity PeekBook evidence can accumulate during Play.
*/
func seedToxicitySession(t testing.TB) *tests.Session {
	t.Helper()

	session, err := tests.NewSession(context.Background(), t, tests.SessionOptions{
		Signals: sessionSignals,
		Level3:  true,
	})

	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	seedAt := time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC)
	trade := kraken.NewTrade(tradeFixturePayload())
	tradeRow := trade.Data[0]
	bidPrice := tradeRow.Price
	askPrice := decimal.NewFromFloat64(tradeRow.Price.Float64() + 0.0001)

	if err := session.Level3.SeedTouch(
		conditions.Subject(), &bidPrice, askPrice, decimal.NewFromFloat64(1000), seedAt,
	); err != nil {
		t.Fatalf("seed touch: %v", err)
	}

	return session
}

/*
playToxicityClaims plays frames on a seeded Level3 Session and asserts claims.
*/
func playToxicityClaims(
	t testing.TB,
	frames iter.Seq[tests.Frame],
	claims ...tests.SourceClaim,
) []*types.Thesis {
	t.Helper()

	session := seedToxicitySession(t)
	theses, err := session.Play(frames)

	if err != nil {
		t.Fatalf("play: %v", err)
	}

	tests.RequireSourceClaims(t, theses, claims...)

	return theses
}

/*
TestSignal_MeasureFromMarket proves toxicity on the mock Conn Session path with
Level3 SeedTouch: toxic chase must emit BestPrice, FillVolume, and TradeVolume
peaks that exceed calm, while phantom quote still publishes touch BestPrice.
Cancel/retreat honesty remains covered by TestSignal_touchHonestyEmitsCancelledQuantity
until the phantom tape drives Level3 quantity retreats.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	symbol := conditions.Subject()

	toxicFamily := []tests.SourceClaim{
		{Source: types.SourceToxicity, Metric: types.MetricBestPrice, Symbol: symbol, Bound: tests.BoundPositive},
		{Source: types.SourceToxicity, Metric: types.MetricFillVolume, Symbol: symbol, Bound: tests.BoundPositive},
		{Source: types.SourceToxicity, Metric: types.MetricTradeVolume, Symbol: symbol, Bound: tests.BoundPositive},
	}

	t.Run("tape_toxic_chase", func(t *testing.T) {
		playToxicityClaims(t, conditions.TapeToxicChase().Frames(), toxicFamily...)
	})

	t.Run("toxic_exceeds_calm_fill_and_trade_volume", func(t *testing.T) {
		calm := playToxicityClaims(t, conditions.TapeCalm().Frames(),
			tests.SourceClaim{
				Source: types.SourceToxicity, Metric: types.MetricTradeVolume,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
		)
		hot := playToxicityClaims(t, conditions.TapeToxicChase().Frames(), toxicFamily...)

		tests.RequireSourceExceeds(
			t, hot, calm,
			types.SourceToxicity, symbol, types.MetricTradeVolume,
		)
		tests.RequireSourceExceeds(
			t, hot, calm,
			types.SourceToxicity, symbol, types.MetricFillVolume,
		)
	})

	t.Run("tape_phantom_keeps_touch_price", func(t *testing.T) {
		playToxicityClaims(t, conditions.TapePhantomQuote().Frames(),
			tests.SourceClaim{
				Source: types.SourceToxicity, Metric: types.MetricBestPrice,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
			tests.SourceClaim{
				Source: types.SourceToxicity, Metric: types.MetricTouchQuantity,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
		)
	})
}

func BenchmarkSignal_MeasureFromMarket(benchmark *testing.B) {
	session := seedToxicitySession(benchmark)
	frames := conditions.TapeToxicChase().Frames()
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if _, err := session.Play(frames); err != nil {
			benchmark.Fatal(err)
		}
	}
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
		_, _ = signal.Calculate(frame)
	}
}
