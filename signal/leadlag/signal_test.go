package leadlag

import (
	"context"
	"iter"
	"math"
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

func hasMeasurement(measurements []*types.Measurement, symbol string, metric types.MetricType) (*types.Measurement, bool) {
	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Symbol == symbol && measurement.Metric == metric {
			return measurement, true
		}
	}

	return nil, false
}

func marketFrame(rows ...kraken.TickerData) *types.MarketFrame {
	crossSection := types.NewCrossSection()
	crossSection.Measure(rows)

	return &types.MarketFrame{
		Tickers:      rows,
		CrossSection: crossSection,
	}
}

func seedLaggedPaths(
	section *Section,
	lagBars int,
	samples int,
) (btcLast float64, ethLast float64, at time.Time) {
	section.SetAnchor("BTC/USD")

	barInterval := 5 * time.Minute
	start := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

	for index := range samples {
		stepAt := start.Add(time.Duration(index) * barInterval)
		price := 100 + float64(index)*0.5

		section.ObservePrice("BTC/USD", price, stepAt)

		sourceIndex := index - lagBars

		if sourceIndex < 0 {
			sourceIndex = 0
		}

		section.ObservePrice("ETH/USD", 100+float64(sourceIndex)*0.5, stepAt)
	}

	at = start.Add(time.Duration(samples) * barInterval)
	sourceIndex := samples - 1 - lagBars

	if sourceIndex < 0 {
		sourceIndex = 0
	}

	return 100 + float64(samples-1)*0.5, 100 + float64(sourceIndex)*0.5, at
}

func TestSection_FeaturesDetectsDelayedFollower(t *testing.T) {
	Convey("Given a follower that leads the anchor in wall-clock time", t, func() {
		section := NewSection()
		section.SetAnchor("BTC/USD")

		start := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
		barInterval := 5 * time.Minute
		samples := 80
		leadBars := 3

		for index := range samples {
			at := start.Add(time.Duration(index) * barInterval)
			section.ObservePrice("ETH/USD", 100+float64(index)*0.5, at)
			section.ObservePrice(
				"BTC/USD",
				200+float64(index)*0.5,
				at.Add(time.Duration(leadBars)*barInterval),
			)
		}

		Convey("When Features evaluates the follower", func() {
			features := section.Features("ETH/USD")

			Convey("Then CrossLagScore should detect leading with negative lag", func() {
				So(features.LagOK, ShouldBeTrue)
				So(features.LagBars, ShouldBeLessThan, 0)
				So(features.LagCorr, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestSection_FeaturesUsesMoveBaseline(t *testing.T) {
	Convey("Given a flat anchor after a volatile prefix", t, func() {
		section := NewSection()
		section.SetAnchor("BTC/USD")

		start := time.Now().Add(-80 * time.Minute)
		flatBase := 100 + float64(65)*0.3

		for index := range 80 {
			stepAt := start.Add(time.Duration(index) * time.Minute)

			var anchorPrice float64

			if index < 65 {
				anchorPrice = 100 + float64(index)*0.3
			} else {
				anchorPrice = flatBase + math.Sin(float64(index)*0.02)*0.001
			}

			section.ObservePrice("BTC/USD", anchorPrice, stepAt)
			section.ObservePrice("ETH/USD", 100+math.Sin(float64(index)*0.9)*0.5, stepAt)
		}

		Convey("When Features evaluates the follower", func() {
			features := section.Features("ETH/USD")

			Convey("Then move baseline should report stall margin without anchor motion", func() {
				So(features.MoveReady, ShouldBeTrue)
				So(features.MoveMoved, ShouldBeFalse)
				So(features.StallMargin, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestSignal_MeasureEmitsWithoutLeader(t *testing.T) {
	Convey("Given a flat cohort with no cross-section leader", t, func() {
		signal := &Signal{
			ctx:     context.Background(),
			section: NewSection(),
		}
		now := time.Now()
		frame := marketFrame(
			kraken.TickerData{
				Symbol:    "BTC/USD",
				Last:      krakendecimal.NewFromFloat64(100),
				ChangePct: 0,
				Timestamp: now,
			},
			kraken.TickerData{
				Symbol:    "ETH/USD",
				Last:      krakendecimal.NewFromFloat64(50),
				ChangePct: 0,
				Timestamp: now,
			},
		)

		measurements, err := signal.Calculate(frame)

		Convey("Then every symbol still publishes provisional lead-lag evidence", func() {
			So(err, ShouldBeNil)

			btc, hasBTC := hasMeasurement(measurements, "BTC/USD", types.MetricStrength)
			eth, hasETH := hasMeasurement(measurements, "ETH/USD", types.MetricStrength)
			So(hasBTC, ShouldBeTrue)
			So(hasETH, ShouldBeTrue)
			So(btc.Validity.State, ShouldEqual, types.ValidityProvisional)
			So(eth.Validity.State, ShouldEqual, types.ValidityProvisional)
			So(btc.Validity.Reason, ShouldContainSubstring, "no cross-section leader")
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
lastMarketEpoch returns the final complete valid follower bundle so lead-lag
assertions keep correlation, lag, and category scores on one market state.
*/
func lastMarketEpoch(
	measurements []*types.Measurement,
	symbol string,
) map[types.MetricType]*types.Measurement {
	var at time.Time

	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Source == types.SourceLeadLag &&
			measurement.Symbol == symbol && measurement.Metric == types.MetricStrength &&
			measurement.Validity.State == types.ValidityValid {
			at = measurement.At
			break
		}
	}

	epoch := make(map[types.MetricType]*types.Measurement)

	for _, measurement := range measurements {
		if measurement.Source == types.SourceLeadLag &&
			measurement.Symbol == symbol && measurement.At.Equal(at) &&
			measurement.Validity.State == types.ValidityValid {
			epoch[measurement.Metric] = measurement
		}
	}

	return epoch
}

/*
TestSignal_MeasureFromMarket proves leadlag separates delayed, synchronous,
decoupled, and stalled anchor relationships through the production feed.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	Convey("Given independently defined anchor-relative cohort paths", t, func() {
		const follower = "ETH/USD"
		lagged := lastMarketEpoch(
			measureMarket(t, conditions.TapeLag().Frames()), follower,
		)
		synchronized := lastMarketEpoch(
			measureMarket(t, conditions.TapeAlpha().Frames()), follower,
		)
		decoupled := lastMarketEpoch(
			measureMarket(t, conditions.TapeNoise().Frames()), follower,
		)
		stalled := lastMarketEpoch(
			measureMarket(t, conditions.TapeStall().Frames()), follower,
		)
		metrics := []types.MetricType{
			types.MetricCorrelation,
			types.MetricSignedCorrelation,
			types.MetricSignedContempCorrelation,
			types.MetricSignedLagCorrelation,
			types.MetricLagFraction,
			types.MetricSampleSupport,
			types.MetricInefficient,
			types.MetricSync,
			types.MetricDecoupled,
			types.MetricStall,
			types.MetricStrength,
		}
		Convey("Then every regime emits the complete valid metric contract", func() {
			for _, epoch := range []map[types.MetricType]*types.Measurement{
				lagged, synchronized, decoupled, stalled,
			} {
				for _, metric := range metrics {
					measurement := epoch[metric]
					So(measurement, ShouldNotBeNil)
					So(measurement.Source, ShouldEqual, types.SourceLeadLag)
					So(measurement.Validity.State, ShouldEqual, types.ValidityValid)
					So(measurement.ValidateStruct(), ShouldBeNil)
				}
			}
		})

		Convey("Then the delayed follower is an inefficient positive-direction lag", func() {
			direction := lagged[types.MetricSignedLagDirection]
			So(direction, ShouldNotBeNil)
			So(direction.Raw, ShouldEqual, 1)
			So(direction.Peer, ShouldEqual, conditions.Subject())
			So(lagged[types.MetricLagFraction].Raw, ShouldBeGreaterThan, 0)
			So(lagged[types.MetricSignedLagCorrelation].Raw, ShouldBeGreaterThan, 0)
			So(lagged[types.MetricInefficient].Raw, ShouldBeGreaterThan, lagged[types.MetricSync].Raw)
			So(lagged[types.MetricInefficient].Raw, ShouldBeGreaterThan, lagged[types.MetricDecoupled].Raw)
			So(lagged[types.MetricStrength].Raw, ShouldAlmostEqual, lagged[types.MetricInefficient].Raw, 1e-12)
		})

		Convey("Then a contemporaneous aligned follower is synchronized", func() {
			So(synchronized[types.MetricSignedContempCorrelation].Raw, ShouldBeGreaterThan, 0.8)
			So(synchronized[types.MetricSync].Raw, ShouldBeGreaterThan, synchronized[types.MetricInefficient].Raw)
			So(synchronized[types.MetricSync].Raw, ShouldBeGreaterThan, synchronized[types.MetricDecoupled].Raw)
			So(synchronized[types.MetricStrength].Raw, ShouldAlmostEqual, synchronized[types.MetricSync].Raw, 1e-12)
		})

		Convey("Then an unrelated follower is decoupled", func() {
			So(decoupled[types.MetricDecoupled].Raw, ShouldBeGreaterThan, 0)
			So(decoupled[types.MetricDecoupled].Raw, ShouldBeGreaterThan, decoupled[types.MetricSync].Raw)
			So(decoupled[types.MetricDecoupled].Raw, ShouldBeGreaterThan, decoupled[types.MetricInefficient].Raw)
			So(decoupled[types.MetricStrength].Raw, ShouldAlmostEqual, decoupled[types.MetricDecoupled].Raw, 1e-12)
		})

		Convey("Then a stopped anchor with active peers carries stall evidence", func() {
			So(stalled[types.MetricStall].Raw, ShouldBeGreaterThan, 0)
			So(stalled[types.MetricStall].Raw, ShouldBeGreaterThan, stalled[types.MetricInefficient].Raw)
			So(stalled[types.MetricStall].Raw, ShouldBeGreaterThan, stalled[types.MetricSync].Raw)
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	signal := &Signal{
		ctx:     context.Background(),
		section: NewSection(),
	}
	btcLast, ethLast, at := seedLaggedPaths(signal.section, 6, 120)
	frame := marketFrame(
		kraken.TickerData{
			Symbol:    "BTC/USD",
			Last:      krakendecimal.NewFromFloat64(btcLast),
			ChangePct: 5,
			Timestamp: at,
		},
		kraken.TickerData{
			Symbol:    "ETH/USD",
			Last:      krakendecimal.NewFromFloat64(ethLast),
			ChangePct: 4,
			Timestamp: at,
		},
	)

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if _, err := signal.Calculate(frame); err != nil {
			benchmark.Fatal(err)
		}
	}
}
