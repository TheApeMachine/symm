package liquidity

import (
	"context"
	"iter"
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
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/types"
)

func measureField(measurements []*types.Measurement, symbol string, metric types.MetricType) (*types.Measurement, bool) {
	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Symbol == symbol && measurement.Metric == metric {
			return measurement, true
		}
	}

	return nil, false
}

func liquidityRow(
	symbol string, bid, ask, bidQty, askQty, volume, vwap float64,
) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Bid:       krakendecimal.NewFromFloat64(bid),
		BidQty:    bidQty,
		Ask:       krakendecimal.NewFromFloat64(ask),
		AskQty:    askQty,
		Last:      krakendecimal.NewFromFloat64((bid + ask) / 2),
		Volume:    volume,
		Vwap:      vwap,
		Timestamp: time.Now(),
	}
}

// liquidityFrame builds a MarketFrame carrying the given ticker rows and a
// cross-section populated from them, matching how production feeds Calculate.
func liquidityFrame(rows ...kraken.TickerData) *types.MarketFrame {
	crossSection := types.NewCrossSection()
	crossSection.Measure(rows)

	return &types.MarketFrame{
		Tickers:      rows,
		CrossSection: crossSection,
	}
}

func measure(signal *Signal, rows ...kraken.TickerData) []*types.Measurement {
	measurements, err := signal.Calculate(liquidityFrame(rows...))
	So(err, ShouldBeNil)

	return measurements
}

func TestSignal_MeasureRequiresTwoExecutablePeers(t *testing.T) {
	Convey("Given a cross-section with only one observed symbol", t, func() {
		signal := &Signal{}

		result := measure(signal,
			liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
		)

		Convey("Then it emits provisional depth without inventing a peer median", func() {
			depth, ok := measureField(result, "BTC/USD", types.MetricExecutableTouchDepth)
			So(ok, ShouldBeTrue)
			So(depth.Validity.State, ShouldEqual, types.ValidityProvisional)
			So(depth.Validity.Reason, ShouldContainSubstring, "peer executable-depth median")

			_, hasRelative := measureField(result, "BTC/USD", types.MetricRelativeTouchDepth)
			So(hasRelative, ShouldBeFalse)
		})
	})
}

func TestSignal_MeasureUsesExecutableValueNotRawVolume(t *testing.T) {
	Convey("Given two penny-priced peers with huge raw volume but tiny quote notional", t, func() {
		signal := &Signal{}

		result := measure(signal,
			liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
			liquidityRow("PENNY1/USD", 0.0001, 0.0001, 1_000_000, 1_000_000, 1_000_000, 0.0001),
			liquidityRow("PENNY2/USD", 0.0001, 0.0001, 1_000_000, 1_000_000, 1_000_000, 0.0001),
		)

		Convey("Then current executable depth, not raw units, determines scarcity", func() {
			relative, ok := measureField(result, "BTC/USD", types.MetricRelativeTouchDepth)
			So(ok, ShouldBeTrue)
			So(relative.Raw, ShouldBeGreaterThan, 1)
			So(relative.Subject, ShouldEqual, types.SubjectPeerLiquidity)
			So(relative.Maturity, ShouldBeGreaterThan, 0)

			strength, ok := measureField(result, "BTC/USD", types.MetricScarcityScore)
			So(ok, ShouldBeTrue)
			So(strength.Raw, ShouldEqual, 0)
		})
	})
}

func TestSignal_MeasureAtPeerMedianIsBalanced(t *testing.T) {
	Convey("Given a subject whose notional and depth match its peers exactly", t, func() {
		signal := &Signal{}

		result := measure(signal,
			liquidityRow("BTC/USD", 99, 101, 5, 5, 100, 100),
			liquidityRow("ETH/USD", 99, 101, 5, 5, 100, 100),
			liquidityRow("SOL/USD", 99, 101, 5, 5, 100, 100),
		)

		Convey("Then relative touch depth is one and scarcity is zero", func() {
			relative, ok := measureField(result, "BTC/USD", types.MetricRelativeTouchDepth)
			So(ok, ShouldBeTrue)
			So(relative.Raw, ShouldAlmostEqual, 1, 1e-9)

			strength, ok := measureField(result, "BTC/USD", types.MetricScarcityScore)
			So(ok, ShouldBeTrue)
			So(strength.Raw, ShouldEqual, 0)
		})
	})
}

func TestSignal_MeasureDoesNotMixTurnoverIntoTouchDepth(t *testing.T) {
	Convey("Given high reported turnover but below-median executable touch depth", t, func() {
		signal := &Signal{}

		result := measure(signal,
			liquidityRow("BTC/USD", 99, 101, 0.5, 0.5, 1_000_000, 100),
			liquidityRow("ETH/USD", 99, 101, 1, 1, 100, 100),
			liquidityRow("SOL/USD", 99, 101, 1, 1, 100, 100),
		)

		Convey("Then turnover cannot inflate the current depth ratio", func() {
			relative, ok := measureField(result, "BTC/USD", types.MetricRelativeTouchDepth)
			So(ok, ShouldBeTrue)
			So(relative.Raw, ShouldAlmostEqual, 0.5, 1e-9)

			strength, ok := measureField(result, "BTC/USD", types.MetricScarcityScore)
			So(ok, ShouldBeTrue)
			So(strength.Raw, ShouldAlmostEqual, 0.5, 1e-9)
		})
	})
}

func TestSignal_MeasureDoesNotRequireReportedTurnover(t *testing.T) {
	Convey("Given executable peers without reported turnover", t, func() {
		signal := &Signal{}

		result := measure(signal,
			liquidityRow("BTC/USD", 99, 101, 0.5, 0.5, 0, 0),
			liquidityRow("ETH/USD", 99, 101, 1, 1, 0, 0),
			liquidityRow("SOL/USD", 99, 101, 1, 1, 0, 0),
		)

		Convey("Then touch scarcity remains measurable without invented turnover", func() {
			strength, ok := measureField(result, "BTC/USD", types.MetricScarcityScore)
			So(ok, ShouldBeTrue)
			So(strength.Raw, ShouldAlmostEqual, 0.5, 1e-9)

			_, hasNotional := measureField(result, "BTC/USD", types.MetricReportedVolumeNotional)
			So(hasNotional, ShouldBeFalse)
		})
	})
}

func TestSignal_MeasureSkipsNonExecutableSubject(t *testing.T) {
	Convey("Given a subject with a one-sided quote among executable peers", t, func() {
		signal := &Signal{}

		result := measure(signal,
			kraken.TickerData{
				Symbol:    "BTC/USD",
				Bid:       krakendecimal.NewFromFloat64(0),
				BidQty:    0,
				Ask:       krakendecimal.NewFromFloat64(101),
				AskQty:    5,
				Last:      krakendecimal.NewFromFloat64(101),
				Volume:    100,
				Vwap:      100,
				Timestamp: time.Now(),
			},
			liquidityRow("ETH/USD", 99, 101, 5, 5, 100, 100),
			liquidityRow("SOL/USD", 99, 101, 5, 5, 100, 100),
		)

		Convey("Then it emits nothing for the unexecutable subject", func() {
			_, hasSubject := measureField(result, "BTC/USD", types.MetricRelativeTouchDepth)
			So(hasSubject, ShouldBeFalse)
		})
	})
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
liquidityEpoch returns the last coherent subject observation that includes a
peer scarcity score, excluding later provisional single-row feed cuts.
*/
func liquidityEpoch(
	measurements []*types.Measurement,
	symbol string,
) map[types.MetricType]*types.Measurement {
	var at time.Time

	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Symbol == symbol && measurement.Metric == types.MetricScarcityScore {
			at = measurement.At
			break
		}
	}

	epoch := make(map[types.MetricType]*types.Measurement)

	for _, measurement := range measurements {
		if measurement.Symbol == symbol && measurement.At.Equal(at) &&
			measurement.Validity.State == types.ValidityValid {
			epoch[measurement.Metric] = measurement
		}
	}

	return epoch
}

/*
TestSignal_MeasureFromMarket proves the production feed keeps executable touch
depth separate from reported turnover and measures scarcity against the cohort.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	Convey("Given identical turnover cohorts with healthy and starved subject quotes", t, func() {
		symbol := conditions.Subject()
		healthy := measureMarket(t, conditions.TapeSectorLift().Frames())
		starved := measureMarket(t, conditions.TapeThinBook().Frames())

		Convey("When the final cohort observations are measured", func() {
			healthyEpoch := liquidityEpoch(healthy, symbol)
			starvedEpoch := liquidityEpoch(starved, symbol)
			healthyDepth := healthyEpoch[types.MetricExecutableTouchDepth]
			starvedDepth := starvedEpoch[types.MetricExecutableTouchDepth]
			healthyRelative := healthyEpoch[types.MetricRelativeTouchDepth]
			starvedRelative := starvedEpoch[types.MetricRelativeTouchDepth]
			healthyScarcity := healthyEpoch[types.MetricScarcityScore]
			starvedScarcity := starvedEpoch[types.MetricScarcityScore]
			healthyMedian := healthyEpoch[types.MetricExecutableTouchDepthMedian]
			starvedMedian := starvedEpoch[types.MetricExecutableTouchDepthMedian]
			healthyTurnover := healthyEpoch[types.MetricReportedVolumeNotional]
			starvedTurnover := starvedEpoch[types.MetricReportedVolumeNotional]
			healthyTurnoverMedian := healthyEpoch[types.MetricReportedVolumeNotionalMedian]
			starvedTurnoverMedian := starvedEpoch[types.MetricReportedVolumeNotionalMedian]

			Convey("Then every promised liquidity measurement is present and valid", func() {
				for _, measurement := range []*types.Measurement{
					healthyDepth, starvedDepth,
					healthyRelative, starvedRelative,
					healthyScarcity, starvedScarcity,
					healthyMedian, starvedMedian,
					healthyTurnover, starvedTurnover,
					healthyTurnoverMedian, starvedTurnoverMedian,
				} {
					So(measurement, ShouldNotBeNil)
					So(measurement.Source, ShouldEqual, types.SourceLiquidity)
					So(measurement.Validity.State, ShouldEqual, types.ValidityValid)
					So(measurement.ValidateStruct(), ShouldBeNil)
				}
			})

			Convey("Then healthy depth is neutral while starved depth is scarce", func() {
				So(healthyRelative.Raw, ShouldAlmostEqual, 1, 1e-9)
				So(healthyScarcity.Raw, ShouldAlmostEqual, 0, 1e-9)
				So(starvedRelative.Raw, ShouldBeGreaterThan, 0)
				So(starvedRelative.Raw, ShouldBeLessThan, 0.1)
				So(starvedScarcity.Raw, ShouldBeGreaterThan, 0.9)
				So(starvedDepth.Raw, ShouldBeLessThan, healthyDepth.Raw)
			})

			Convey("Then starving quotes cannot alter peer depth or turnover context", func() {
				So(starvedMedian.Raw, ShouldAlmostEqual, healthyMedian.Raw, 1e-9)
				So(starvedTurnover.Raw, ShouldAlmostEqual, healthyTurnover.Raw, 1e-9)
				So(starvedTurnoverMedian.Raw, ShouldAlmostEqual, healthyTurnoverMedian.Raw, 1e-9)
			})
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	signal := &Signal{}
	frame := liquidityFrame(
		liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000),
		liquidityRow("ETH/USD", 99, 101, 5, 5, 100, 100),
		liquidityRow("SOL/USD", 99, 101, 5, 5, 100, 100),
	)

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if _, err := signal.Calculate(frame); err != nil {
			benchmark.Fatal(err)
		}
	}
}
