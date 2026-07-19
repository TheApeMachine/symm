package sentiment

import (
	"context"
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

/*
TestSignal_MeasureFromMarket proves sentiment on the mock Conn Session path:
noise must emit Change/Strength, and sector-lift must emit Strength plus
divergent or leader evidence — absolute claims, not Abs(change) alone.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	symbol := conditions.Subject()
	options := tests.SessionOptions{Signals: sessionSignals}

	t.Run("tape_noise", func(t *testing.T) {
		tests.PlayMarketClaims(t, options, conditions.TapeNoise().Frames(),
			tests.SourceClaim{
				Source: types.SourceSentiment, Metric: types.MetricChange,
				Symbol: symbol, Bound: tests.BoundPresent,
			},
			tests.SourceClaim{
				Source: types.SourceSentiment, Metric: types.MetricStrength,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
		)
	})

	t.Run("tape_sector_lift", func(t *testing.T) {
		// Co-moving herd zeros DivergentScore/LeaderEvidence/SurgeScore/Breadth
		// on the subject; Strength (slump-mass sibling) is the absolute claim.
		tests.PlayMarketClaims(t, options, conditions.TapeSectorLift().Frames(),
			tests.SourceClaim{
				Source: types.SourceSentiment, Metric: types.MetricStrength,
				Symbol: symbol, Bound: tests.BoundPositive,
			},
			tests.SourceClaim{
				Source: types.SourceSentiment, Metric: types.MetricChange,
				Symbol: symbol, Bound: tests.BoundPresent,
			},
		)
	})
}

func BenchmarkSignal_MeasureFromMarket(benchmark *testing.B) {
	session, err := tests.NewSession(context.Background(), benchmark, tests.SessionOptions{
		Signals: sessionSignals,
	})

	if err != nil {
		benchmark.Fatal(err)
	}

	frames := conditions.TapeNoise().Frames()
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if _, err := session.Play(frames); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func TestSignal_Measure(t *testing.T) {
	Convey("Given current ticker rows with one clear cohort leader", t, func() {
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
func TestSignal_Publish(t *testing.T) {
	Convey("Given one complete sentiment observation", t, func() {
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
