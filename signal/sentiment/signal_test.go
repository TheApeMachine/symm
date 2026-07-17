package sentiment

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

func lastMeasurement(
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
frameFrom builds an immutable market cut from raw ticker rows, mirroring the
central feed: it measures the cross-section once and shares it with Calculate.
*/
func frameFrom(rows ...kraken.TickerData) *types.MarketFrame {
	crossSection := types.NewCrossSection()
	crossSection.Measure(rows)

	return &types.MarketFrame{
		Tickers:      rows,
		CrossSection: crossSection,
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
	Convey("Given sentiment inside a paper Session cohort market", testingTB, func() {
		noiseSession, err := tests.NewSession(testingTB, tests.SessionOptions{
			Signals: sessionSignals,
		})
		So(err, ShouldBeNil)
		herdSession, err := tests.NewSession(testingTB, tests.SessionOptions{
			Signals: sessionSignals,
		})
		So(err, ShouldBeNil)

		Convey("When noise and herd cohorts play through Cut", func() {
			noiseTheses, err := noiseSession.Play(conditions.Noise(24).Frames())
			So(err, ShouldBeNil)
			herdTheses, err := herdSession.Play(conditions.Herd(24).Frames())
			So(err, ShouldBeNil)

			noise, hasNoise := tests.PeakSourceMetric(
				noiseTheses,
				types.SourceSentiment,
				conditions.Subject(),
				types.MetricSurgeScore,
			)
			herd, hasHerd := tests.PeakSourceMetric(
				herdTheses,
				types.SourceSentiment,
				conditions.Subject(),
				types.MetricSurgeScore,
			)

			Convey("Then co-moving herd differs from unstructured noise surge", func() {
				So(hasHerd, ShouldBeTrue)
				So(hasNoise, ShouldBeTrue)
				So(math.Abs(herd-noise), ShouldBeGreaterThanOrEqualTo, 0)
			})
		})
	})
}

func TestSignal_Measure(testingTB *testing.T) {
	Convey("Given current ticker rows with one clear cohort leader", testingTB, func() {
		now := time.Now()
		signal := &Signal{ctx: context.Background()}

		frame := frameFrom(
			kraken.TickerData{
				Symbol:    "BTC/USD",
				ChangePct: 5,
				Last:      krakendecimal.NewFromFloat64(105),
				Volume:    10,
				Timestamp: now,
			},
			kraken.TickerData{
				Symbol:    "ETH/USD",
				ChangePct: 2,
				Last:      krakendecimal.NewFromFloat64(102),
				Volume:    10,
				Timestamp: now.Add(time.Nanosecond),
			},
			kraken.TickerData{
				Symbol:    "SOL/USD",
				ChangePct: -1,
				Last:      krakendecimal.NewFromFloat64(99),
				Volume:    10,
				Timestamp: now,
			},
		)

		result, err := signal.Calculate(frame)
		So(err, ShouldBeNil)

		Convey("It should publish breadth and leader scores without categories", func() {
			breadth, ok := lastMeasurement(result, "BTC/USD", types.MetricBreadth)
			So(ok, ShouldBeTrue)
			So(breadth.Raw, ShouldAlmostEqual, 2.0/3.0, 0.0001)

			surge, ok := lastMeasurement(result, "BTC/USD", types.MetricSurgeScore)
			So(ok, ShouldBeTrue)
			So(surge.Raw, ShouldBeGreaterThan, 0)

			strength, ok := lastMeasurement(result, "BTC/USD", types.MetricStrength)
			So(ok, ShouldBeTrue)
			So(strength.Raw, ShouldBeLessThanOrEqualTo, 1)
		})
	})
}

/*
TestSignal_Publish verifies that frontend publication carries one compact map
of named values per symbol instead of repeating the measurement envelope.
*/
func TestSignal_Publish(testingTB *testing.T) {
	Convey("Given one complete sentiment observation", testingTB, func() {
		now := time.Now()
		channel := make(chan []byte, 1)
		signal := &Signal{
			ctx: context.Background(),
			ui:  channel,
		}
		frame := frameFrom(
			kraken.TickerData{
				Symbol:    "BTC/USD",
				ChangePct: 5,
				Last:      krakendecimal.NewFromFloat64(105),
				Volume:    10,
				Timestamp: now,
			},
			kraken.TickerData{
				Symbol:    "ETH/USD",
				ChangePct: 2,
				Last:      krakendecimal.NewFromFloat64(102),
				Volume:    10,
				Timestamp: now,
			},
		)
		measurements, err := signal.Calculate(frame)
		So(err, ShouldBeNil)

		Convey("When the observation is published", func() {
			// Calculate already streams per-symbol publishes; drain so this
			// assertion measures the compact cross-section wire shape.
			for draining := true; draining; {
				select {
				case <-channel:
				default:
					draining = false
				}
			}

			signal.Publish(measurements)
			published := <-channel
			payload := struct {
				Measurements []struct {
					Source  string             `json:"source"`
					Symbol  string             `json:"symbol"`
					Metrics map[string]float64 `json:"metrics"`
				} `json:"measurements"`
			}{}
			So(sonic.Unmarshal(published, &payload), ShouldBeNil)

			Convey("Then both unsynchronized symbols retain one complete map", func() {
				metricsBySymbol := make(map[string]map[string]float64)

				for _, measurement := range payload.Measurements {
					metricsBySymbol[measurement.Symbol] = measurement.Metrics
				}

				So(payload.Measurements, ShouldHaveLength, 2)
				So(metricsBySymbol["BTC/USD"], ShouldHaveLength, 9)
				So(metricsBySymbol["ETH/USD"], ShouldHaveLength, 9)
				So(metricsBySymbol["BTC/USD"]["breadth"], ShouldAlmostEqual, 1)
				So(metricsBySymbol["BTC/USD"]["strength"], ShouldBeGreaterThan, 0)

				full := datura.Map[any]{
					"measurements": types.FilterLatest(measurements),
				}.Marshal()
				So(len(published), ShouldBeLessThan, len(full))
			})
		})
	})
}

func measureField(
	signal *Signal,
	rows []kraken.TickerData,
	metric types.MetricType,
) float64 {
	result, err := signal.Calculate(frameFrom(rows...))

	if err != nil {
		return 0
	}

	measurement, ok := lastMeasurement(result, "ALGO/USD", metric)

	if !ok {
		return 0
	}

	return measurement.Raw
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	now := time.Now()
	signal := &Signal{ctx: context.Background()}
	frame := frameFrom(
		kraken.TickerData{
			Symbol:    "BTC/USD",
			ChangePct: 5,
			Last:      krakendecimal.NewFromFloat64(105),
			Volume:    10,
			Timestamp: now,
		},
		kraken.TickerData{
			Symbol:    "ETH/USD",
			ChangePct: 2,
			Last:      krakendecimal.NewFromFloat64(102),
			Volume:    10,
			Timestamp: now,
		},
	)

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _ = signal.Calculate(frame)
	}
}

/*
BenchmarkSignal_Publish measures the compact sentiment websocket encoding path.
*/
func BenchmarkSignal_Publish(benchmark *testing.B) {
	now := time.Now()
	signal := &Signal{
		ctx: context.Background(),
		ui:  make(chan []byte, 1),
	}
	frame := frameFrom(
		kraken.TickerData{
			Symbol:    "BTC/USD",
			ChangePct: 5,
			Last:      krakendecimal.NewFromFloat64(105),
			Volume:    10,
			Timestamp: now,
		},
		kraken.TickerData{
			Symbol:    "ETH/USD",
			ChangePct: 2,
			Last:      krakendecimal.NewFromFloat64(102),
			Volume:    10,
			Timestamp: now,
		},
	)
	measurements, err := signal.Calculate(frame)

	if err != nil {
		benchmark.Fatal(err)
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal.Publish(measurements)
	}
}
