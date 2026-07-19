package fluid

import (
	"context"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

func sessionSignals(
	ctx context.Context,
	api *websocket.API,
	instrument *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{NewSignal(ctx, api, instrument, channel)}
}

/*
driveFluidPath feeds ticker, book, and optional aggressive trades that warm
the fluid grid. MATIC Level2 Session books cannot configure the exchange tick
for fluid, so falsifiers use BTC/USD with PriceIncrement set.
*/
func driveFluidPath(aggressive bool) ([]*types.Measurement, error) {
	symbolConfigValue.Store(nil)
	signal := NewSignal(context.Background(), nil, nil, nil)
	at := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	fixture := &symbolBookFixture{symbol: "BTC/USD"}
	out := make([]*types.Measurement, 0, 256)

	for index := 0; index < 20; index++ {
		stamp := at.Add(time.Duration(index) * 200 * time.Millisecond)
		midMove := 0.0

		if aggressive {
			midMove = float64(index) * 0.02
		}

		bid := 99.5 + midMove
		ask := 100.5 + midMove
		bidQty := 20.0
		askQty := 20.0
		volume := 1_000.0 + float64(index)*10

		if aggressive {
			bidQty = 40 - float64(index)*1.5
			askQty = 5 + float64(index)*2
			volume = 1_000 + float64(index)*200
		}

		ticker := kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       krakendecimal.NewFromFloat64(bid),
			Ask:       krakendecimal.NewFromFloat64(ask),
			Last:      krakendecimal.NewFromFloat64((bid + ask) / 2),
			Volume:    volume,
			Timestamp: stamp,
		}

		if aggressive {
			ticker.ChangePct = float64(index) * 0.5
		}

		if _, err := signal.Calculate(&types.MarketFrame{
			Tickers:      []kraken.TickerData{ticker},
			CrossSection: types.NewCrossSection(),
		}); err != nil {
			return out, err
		}

		row := fixture.snapshot(bid, bidQty, ask, askQty)
		row.Timestamp = stamp
		row.PriceIncrement = krakendecimal.NewFromFloat64(0.01)

		var trades []kraken.TradeData

		if aggressive {
			trades = []kraken.TradeData{{
				Symbol:    "BTC/USD",
				Side:      "buy",
				Price:     *krakendecimal.NewFromFloat64(ask),
				Qty:       3 + float64(index),
				Timestamp: stamp,
			}}
		}

		measurements, err := signal.Calculate(&types.MarketFrame{
			Tickers:      []kraken.TickerData{ticker},
			Books:        []kraken.BookData{row},
			Trades:       trades,
			CrossSection: types.NewCrossSection(),
		})

		if err != nil {
			return out, err
		}

		out = append(out, measurements...)
	}

	return out, nil
}

/*
peakFluidMetric returns the greatest raw value for a fluid metric.
*/
func peakFluidMetric(
	measurements []*types.Measurement,
	metric types.MetricType,
) (float64, bool) {
	peak := 0.0
	found := false

	for _, measurement := range measurements {
		if measurement == nil ||
			measurement.Source != types.SourceFluid ||
			measurement.Metric != metric {
			continue
		}

		found = true

		if measurement.Raw > peak {
			peak = measurement.Raw
		}
	}

	return peak, found
}

func TestSignal_MeasureFromMarket(testingTB *testing.T) {
	Convey("Given fluid Session smoke plus calm vs stressed full streams", testingTB, func() {
		previousInterval := viper.Get("signals.fluid.integration_interval")
		testingTB.Cleanup(func() {
			viper.Set("signals.fluid.integration_interval", previousInterval)
		})
		viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)

		session, err := tests.NewSession(context.Background(), testingTB, tests.SessionOptions{
			Signals: sessionSignals,
		})
		So(err, ShouldBeNil)

		Convey("When Session plays Decay and full-stream paths diverge", func() {
			_, err := session.Play(conditions.Decay(16, 0, 0.9).Frames())
			So(err, ShouldBeNil)

			calm, err := driveFluidPath(false)
			So(err, ShouldBeNil)
			stressed, err := driveFluidPath(true)
			So(err, ShouldBeNil)

			calmViscosity, hasCalm := peakFluidMetric(calm, types.MetricViscosity)
			stressViscosity, hasStress := peakFluidMetric(
				stressed,
				types.MetricViscosity,
			)
			_, hasLaminar := peakFluidMetric(
				stressed,
				types.MetricLaminarScore,
			)

			Convey("Then Session stays healthy and stress lifts viscosity", func() {
				So(session.Desk.OpenPositions(), ShouldEqual, 0)
				So(hasLaminar, ShouldBeTrue)
				So(hasStress, ShouldBeTrue)
				So(len(calm), ShouldBeGreaterThan, 0)

				if hasCalm {
					So(stressViscosity, ShouldBeGreaterThan, calmViscosity)
				}
			})
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	previousInterval := viper.Get("signals.fluid.integration_interval")
	benchmark.Cleanup(func() {
		viper.Set("signals.fluid.integration_interval", previousInterval)
	})
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	symbolConfigValue.Store(nil)
	signal := NewSignal(context.Background(), nil, nil, nil)
	at := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	fixture := &symbolBookFixture{symbol: "BTC/USD"}

	for index := 0; index < 4; index++ {
		stamp := at.Add(time.Duration(index) * 200 * time.Millisecond)
		ticker := kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       krakendecimal.NewFromFloat64(99.5),
			Ask:       krakendecimal.NewFromFloat64(100.5),
			Last:      krakendecimal.NewFromFloat64(100),
			Volume:    1_000,
			Timestamp: stamp,
		}
		if _, err := signal.Calculate(&types.MarketFrame{
			Tickers:      []kraken.TickerData{ticker},
			CrossSection: types.NewCrossSection(),
		}); err != nil {
			benchmark.Fatal(err)
		}
		row := fixture.snapshot(99.5, 20-float64(index), 100.5, 10)
		row.Timestamp = stamp
		row.PriceIncrement = krakendecimal.NewFromFloat64(0.01)
		if _, err := signal.Calculate(&types.MarketFrame{
			Tickers:      []kraken.TickerData{ticker},
			Books:        []kraken.BookData{row},
			CrossSection: types.NewCrossSection(),
		}); err != nil {
			benchmark.Fatal(err)
		}
	}

	stamp := at.Add(time.Second)
	ticker := kraken.TickerData{
		Symbol:    "BTC/USD",
		Bid:       krakendecimal.NewFromFloat64(99.5),
		Ask:       krakendecimal.NewFromFloat64(100.5),
		Last:      krakendecimal.NewFromFloat64(100),
		Volume:    1_200,
		Timestamp: stamp,
	}
	row := fixture.snapshot(99.5, 12, 100.5, 14)
	row.Timestamp = stamp
	row.PriceIncrement = krakendecimal.NewFromFloat64(0.01)
	frame := &types.MarketFrame{
		Tickers:      []kraken.TickerData{ticker},
		Books:        []kraken.BookData{row},
		CrossSection: types.NewCrossSection(),
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if _, err := signal.Calculate(frame); err != nil {
			benchmark.Fatal(err)
		}
	}
}
