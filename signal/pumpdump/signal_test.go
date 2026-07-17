package pumpdump

import (
	"context"
	"iter"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	"github.com/theapemachine/symm/types"
)

/*
drive replays a ticker timeline through Calculate one frame at a time, warming
the shared ignition state exactly as the live feed does, and returns the last
frame's measurements.
*/
func drive(signal *Signal, frames iter.Seq[tests.Frame]) []*types.Measurement {
	var last []*types.Measurement

	for frame := range frames {
		rows := kraken.NewTicker(frame.Payload).Data

		if len(rows) == 0 {
			continue
		}

		measurements, err := signal.Calculate(&types.MarketFrame{
			Tickers:      rows,
			CrossSection: types.NewCrossSection(),
		})

		if err != nil {
			continue
		}

		last = measurements
	}

	return last
}

func newSignal() *Signal {
	return &Signal{
		ctx:      context.Background(),
		ignition: equation.NewIgnition(),
	}
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
	Convey("Given pumpdump inside a paper Session market", testingTB, func() {
		calmSession, err := tests.NewSession(context.Background(), testingTB, tests.SessionOptions{
			Signals: sessionSignals,
		})
		So(err, ShouldBeNil)
		pumpSession, err := tests.NewSession(context.Background(), testingTB, tests.SessionOptions{
			Signals: sessionSignals,
		})
		So(err, ShouldBeNil)

		Convey("When calm and pumped conditions play through Cut", func() {
			calmTheses, err := calmSession.Play(conditions.Calm(32).Frames())
			So(err, ShouldBeNil)
			pumpTheses, err := pumpSession.Play(
				conditions.Pump(32, 16, 1.25, 8).Frames(),
			)
			So(err, ShouldBeNil)

			calm, hasCalm := tests.PeakMetric(
				calmTheses, "MATIC/USD", types.MetricRVOL,
			)
			pumped, hasPumped := tests.PeakMetric(
				pumpTheses, "MATIC/USD", types.MetricRVOL,
			)

			Convey("Then the pumped stream should lift relative volume", func() {
				So(hasCalm, ShouldBeTrue)
				So(hasPumped, ShouldBeTrue)
				So(pumped, ShouldBeGreaterThan, calm)
			})
		})
	})
}

func TestSignal_MeasureSkipsIncompleteRow(testingTB *testing.T) {
	Convey("Given a partial Kraken ticker row", testingTB, func() {
		signal := newSignal()

		Convey("When measure runs", func() {
			result, err := signal.Calculate(&types.MarketFrame{
				Tickers: []kraken.TickerData{
					{Symbol: "BTC/USD"},
				},
				CrossSection: types.NewCrossSection(),
			})

			Convey("Then it should wait without publishing metrics", func() {
				So(err, ShouldBeNil)
				So(result, ShouldBeEmpty)
			})
		})
	})
}

func TestSignal_MeasureEmitsWhileCalibrating(testingTB *testing.T) {
	Convey("Given a complete ticker that has not yet formed ignition baselines", testingTB, func() {
		signal := newSignal()
		at := time.Date(2026, 7, 17, 1, 3, 45, 0, time.UTC)
		row := kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       krakendecimal.NewFromFloat64(999),
			Ask:       krakendecimal.NewFromFloat64(1001),
			Last:      krakendecimal.NewFromFloat64(1000),
			Volume:    10,
			Timestamp: at,
		}

		first, err := signal.Calculate(&types.MarketFrame{
			Tickers:      []kraken.TickerData{row},
			CrossSection: types.NewCrossSection(),
		})
		So(err, ShouldBeNil)
		So(first, ShouldNotBeEmpty)

		second, err := signal.Calculate(&types.MarketFrame{
			Tickers: []kraken.TickerData{{
				Symbol:    row.Symbol,
				Bid:       row.Bid,
				Ask:       row.Ask,
				Last:      row.Last,
				Volume:    row.Volume,
				Timestamp: at.Add(time.Second),
			}},
			CrossSection: types.NewCrossSection(),
		})

		Convey("Then the second tick still publishes provisional ignition evidence", func() {
			So(err, ShouldBeNil)
			So(second, ShouldNotBeEmpty)

			found := false

			for _, measurement := range second {
				if measurement.Metric == types.MetricIgnition {
					found = true
					So(measurement.Validity.State, ShouldEqual, types.ValidityProvisional)
				}
			}

			So(found, ShouldBeTrue)
		})
	})
}

func TestSignal_Measure(testingTB *testing.T) {
	Convey("Given a replayed ticker timeline", testingTB, func() {
		signal := newSignal()
		fixture := tickerfixture.NewFixture(tickerfixture.UPDATE, 32)

		result := drive(signal, fixture.Frames())

		Convey("It should publish ignition metrics without categories", func() {
			ignition := 0.0

			for _, measurement := range result {
				if measurement.Symbol == "MATIC/USD" && measurement.Metric == types.MetricIgnition {
					ignition = measurement.Raw
				}
			}

			So(ignition, ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	market := tests.NewMarket().
		Feed(tickerfixture.NewFixture(tickerfixture.UPDATE, 32))

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_ = drive(newSignal(), market.Frames())
	}
}
