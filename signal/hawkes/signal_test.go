package hawkes

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

func newTestSignal() *Signal {
	return &Signal{
		ctx:      context.Background(),
		sample:   excitation.NewSample(),
		process:  excitation.NewProcess(),
		evidence: NewEvidence(),
	}
}

func frameOf(rows ...kraken.TradeData) *types.MarketFrame {
	return &types.MarketFrame{Trades: rows, CrossSection: types.NewCrossSection()}
}

func measureField(
	measurements []*types.Measurement, symbol string, metric types.MetricType,
) (*types.Measurement, bool) {
	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Symbol == symbol && measurement.Metric == metric {
			return measurement, true
		}
	}

	return nil, false
}

/*
peakHawkesSideMetric returns the greatest raw buy/sell Hawkes value for subject.
*/
func peakHawkesSideMetric(
	theses []*types.Thesis,
	side types.MeasurementSide,
	metric types.MetricType,
) (float64, bool) {
	peak := 0.0
	found := false
	initialized := false

	for _, thesis := range theses {
		if thesis == nil {
			continue
		}

		for _, measurement := range thesis.Measurements {
			if measurement.Source != types.SourceHawkes ||
				measurement.Symbol != conditions.Subject() ||
				measurement.Metric != metric ||
				measurement.Side != side {
				continue
			}

			found = true

			if !initialized {
				peak = measurement.Raw
				initialized = true
				continue
			}

			if measurement.Raw > peak {
				peak = measurement.Raw
			}
		}
	}

	return peak, found
}

func tradeRow(symbol, side string, price float64, quantity float64, at time.Time) kraken.TradeData {
	return kraken.TradeData{
		Symbol:    symbol,
		Side:      side,
		Price:     *decimal.NewFromFloat64(price),
		Qty:       quantity,
		Timestamp: at,
	}
}

func TestSignalOnTrade(t *testing.T) {
	Convey("Given a Hawkes signal driven by the central market cut", t, func() {
		signal := newTestSignal()
		at := time.Date(2023, 9, 25, 9, 4, 31, 0, time.UTC)
		row := tradeRow("BTC/USD", "buy", 100.5, 1.25, at)

		Convey("When a trade frame is calculated", func() {
			_, err := signal.Calculate(frameOf(row))

			Convey("Then Calculate should accept the row without error", func() {
				So(err, ShouldBeNil)
			})
		})

		Convey("When an empty frame arrives", func() {
			measurements, err := signal.Calculate(frameOf())

			Convey("Then nothing should be measured", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldBeEmpty)
			})
		})

		Convey("When frame calculations overlap measurement drains", func() {
			wait := sync.WaitGroup{}
			wait.Add(2)

			go func() {
				defer wait.Done()

				for range 100 {
					signal.Calculate(frameOf(row))
				}
			}()

			go func() {
				defer wait.Done()

				for range 100 {
					_ = signal.Measure(types.NewThesis(nil, frameOf(row)))
				}
			}()

			wait.Wait()
		})
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given a Hawkes signal with enough marked arrivals to identify a stream", t, func() {
		signal := newTestSignal()
		start := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		sides := []string{"buy", "sell", "buy", "sell", "buy", "sell"}
		var result []*types.Measurement

		Convey("When each trade arrives on its own tick", func() {
			for index, side := range sides {
				frame := frameOf(
					tradeRow("BTC/USD", side, 100+float64(index), 1, start.Add(time.Duration(index)*time.Second)),
				)
				measurements, err := signal.Calculate(frame)
				So(err, ShouldBeNil)
				result = measurements
			}

			Convey("Then event-count observation measurements should be emitted", func() {
				count, ok := measureField(result, "BTC/USD", types.MetricEventCount)
				So(ok, ShouldBeTrue)
				So(count.Raw, ShouldBeGreaterThan, 0)
			})
		})
	})
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
TestSignal_MeasureFromMarket proves Hawkes on the mock Conn Session path:
toxic chase must emit EventCount (catalog PeakSourceMetric) and side-aware
buy EventCount, exceeding calm — not a relative-only smoke check.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	symbol := conditions.Subject()
	options := tests.SessionOptions{Signals: sessionSignals}

	t.Run("tape_toxic_chase", func(t *testing.T) {
		hot := tests.PlayMarketClaims(t, options, conditions.TapeToxicChase().Frames(),
			tests.SourceClaim{
				Source: types.SourceHawkes, Metric: types.MetricEventCount,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
		)

		buyCount, ok := peakHawkesSideMetric(hot, types.SideBuy, types.MetricEventCount)

		if !ok || buyCount <= 0 {
			t.Fatalf("want buy EventCount > 0 on toxic chase (ok=%v peak=%g)", ok, buyCount)
		}
	})

	t.Run("toxic_exceeds_calm_buy_event_count", func(t *testing.T) {
		// Unsigned PeakSourceMetric EventCount saturates equally on calm vs chase;
		// side-aware buy EventCount is the meaningful contrast.
		calm := tests.PlayMarketClaims(t, options, conditions.TapeCalm().Frames())
		hot := tests.PlayMarketClaims(t, options, conditions.TapeToxicChase().Frames(),
			tests.SourceClaim{
				Source: types.SourceHawkes, Metric: types.MetricEventCount,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
		)

		calmBuy, hasCalm := peakHawkesSideMetric(calm, types.SideBuy, types.MetricEventCount)
		hotBuy, hasHot := peakHawkesSideMetric(hot, types.SideBuy, types.MetricEventCount)

		if !hasHot {
			t.Fatal("want buy EventCount on toxic chase")
		}

		if !hasCalm {
			t.Fatal("want buy EventCount on calm")
		}

		if hotBuy <= calmBuy {
			t.Fatalf(
				"want toxic buy EventCount (%g) > calm (%g)",
				hotBuy, calmBuy,
			)
		}
	})
}

func BenchmarkSignal_MeasureFromMarket(benchmark *testing.B) {
	session, err := tests.NewSession(context.Background(), benchmark, tests.SessionOptions{
		Signals: sessionSignals,
	})

	if err != nil {
		benchmark.Fatal(err)
	}

	frames := conditions.TapeToxicChase().Frames()
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if _, err := session.Play(frames); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	signal := newTestSignal()
	start := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	for index := range 8 {
		_, _ = signal.Calculate(frameOf(tradeRow(
			"MATIC/USD",
			"buy",
			0.56+float64(index)*0.001,
			1+float64(index),
			start.Add(time.Duration(index)*time.Second),
		)))
	}

	frame := frameOf(tradeRow(
		"MATIC/USD", "buy", 0.57, 4, start.Add(9*time.Second),
	))

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _ = signal.Calculate(frame)
	}
}
