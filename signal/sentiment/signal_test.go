package sentiment

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/trader"
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

func measureMarket(t testing.TB, frames iter.Seq[tests.Frame]) []*types.Measurement {
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
	instrument := broker.NewInstrument(api, broker.NewPrice(api), nil)
	api.On("instrument", instrument.On)
	market, err := trader.NewMarket(ctx, api, instrument)
	So(err, ShouldBeNil)
	t.Cleanup(market.Close)
	signal := NewSignal(ctx, api, nil)
	measurements := make([]*types.Measurement, 0)
	cutAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for frame := range frames {
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
lastSentimentEpoch returns the final subject bundle without mixing metric peaks
from different market states.
*/
func lastSentimentEpoch(
	measurements []*types.Measurement,
	symbol string,
) map[types.MetricType]*types.Measurement {
	var at time.Time

	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Source == types.SourceSentiment &&
			measurement.Symbol == symbol && measurement.Metric == types.MetricStrength {
			at = measurement.At
			break
		}
	}

	epoch := make(map[types.MetricType]*types.Measurement)

	for _, measurement := range measurements {
		if measurement.Source == types.SourceSentiment &&
			measurement.Symbol == symbol && measurement.At.Equal(at) {
			epoch[measurement.Metric] = measurement
		}
	}

	return epoch
}

/*
TestSignal_MeasureFromMarket proves sentiment separates broad risk-on,
single-symbol divergence, and systemic decline through the production feed.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	Convey("Given independently defined breadth and leadership regimes", t, func() {
		symbol := conditions.Subject()
		riskOn := lastSentimentEpoch(
			measureMarket(t, conditions.TapeAlpha().Frames()), symbol,
		)
		divergent := lastSentimentEpoch(
			measureMarket(t, conditions.TapeDivergence().Frames()), symbol,
		)
		slump := lastSentimentEpoch(
			measureMarket(t, conditions.TapeSlump().Frames()), symbol,
		)
		metrics := []types.MetricType{
			types.MetricChange,
			types.MetricBreadth,
			types.MetricLeaderStrength,
			types.MetricLeaderEvidence,
			types.MetricRelativeLead,
			types.MetricSurgeScore,
			types.MetricDivergentScore,
			types.MetricSlumpScore,
			types.MetricStrength,
		}

		Convey("Then every regime emits the complete valid metric contract", func() {
			for _, epoch := range []map[types.MetricType]*types.Measurement{
				riskOn, divergent, slump,
			} {
				So(epoch, ShouldHaveLength, len(metrics))

				for _, metric := range metrics {
					measurement := epoch[metric]
					So(measurement, ShouldNotBeNil)
					So(measurement.Source, ShouldEqual, types.SourceSentiment)
					So(measurement.Validity.State, ShouldEqual, types.ValidityValid)
					So(measurement.ValidateStruct(), ShouldBeNil)
				}
			}
		})

		Convey("Then a rising cohort with a stronger leader is risk-on", func() {
			So(riskOn[types.MetricChange].Raw, ShouldBeGreaterThan, 0)
			So(riskOn[types.MetricBreadth].Raw, ShouldAlmostEqual, 1, 1e-12)
			So(riskOn[types.MetricLeaderStrength].Raw, ShouldBeGreaterThan, 0)
			So(riskOn[types.MetricLeaderEvidence].Raw, ShouldBeGreaterThan, 1)
			So(riskOn[types.MetricRelativeLead].Raw, ShouldEqual, 1)
			So(riskOn[types.MetricSurgeScore].Raw, ShouldBeGreaterThan, 0)
			So(riskOn[types.MetricDivergentScore].Raw, ShouldEqual, 0)
			So(riskOn[types.MetricSlumpScore].Raw, ShouldEqual, 0)
			So(riskOn[types.MetricStrength].Raw, ShouldAlmostEqual, riskOn[types.MetricSurgeScore].Raw, 1e-12)
		})

		Convey("Then a rising leader against falling peers is divergent", func() {
			So(divergent[types.MetricChange].Raw, ShouldBeGreaterThan, 0)
			So(divergent[types.MetricBreadth].Raw, ShouldAlmostEqual, 1.0/3.0, 1e-12)
			So(divergent[types.MetricRelativeLead].Raw, ShouldEqual, 1)
			So(divergent[types.MetricDivergentScore].Raw, ShouldBeGreaterThan, divergent[types.MetricSurgeScore].Raw)
			So(divergent[types.MetricDivergentScore].Raw, ShouldBeGreaterThan, divergent[types.MetricSlumpScore].Raw)
			So(divergent[types.MetricStrength].Raw, ShouldAlmostEqual, divergent[types.MetricDivergentScore].Raw, 1e-12)
		})

		Convey("Then a uniformly falling leaderless cohort is a systemic slump", func() {
			So(slump[types.MetricChange].Raw, ShouldBeLessThan, 0)
			So(slump[types.MetricBreadth].Raw, ShouldEqual, 0)
			So(slump[types.MetricLeaderStrength].Raw, ShouldEqual, 0)
			So(slump[types.MetricLeaderEvidence].Raw, ShouldEqual, 0)
			So(slump[types.MetricRelativeLead].Raw, ShouldEqual, 0)
			So(slump[types.MetricSurgeScore].Raw, ShouldEqual, 0)
			So(slump[types.MetricDivergentScore].Raw, ShouldEqual, 0)
			So(slump[types.MetricSlumpScore].Raw, ShouldEqual, 1)
			So(slump[types.MetricStrength].Raw, ShouldEqual, 1)
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
		if _, err := signal.Calculate(frame); err != nil {
			benchmark.Fatal(err)
		}
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
