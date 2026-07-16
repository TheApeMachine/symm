package leadlag

import (
	"context"
	"math"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
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

func TestSection_FeaturesDetectsDelayedFollower(testingTB *testing.T) {
	Convey("Given a follower that leads the anchor in wall-clock time", testingTB, func() {
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

func TestSection_FeaturesUsesMoveBaseline(testingTB *testing.T) {
	Convey("Given a flat anchor after a volatile prefix", testingTB, func() {
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

func TestSignal_MeasureEmitsLeadlag(testingTB *testing.T) {
	Convey("Given a seeded leadlag section and leader rows", testingTB, func() {
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
				ChangePct: 1,
				Timestamp: at,
			},
		)

		measurements, err := signal.Calculate(frame)

		Convey("Then it should emit numeric leadlag measurements", func() {
			So(err, ShouldBeNil)

			strength, hasFollower := hasMeasurement(measurements, "ETH/USD", types.MetricStrength)

			So(hasFollower, ShouldBeTrue)
			So(strength.Raw, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignal_MeasureSkipsIncompleteRow(testingTB *testing.T) {
	Convey("Given a follower row without a last price", testingTB, func() {
		signal := &Signal{
			ctx:     context.Background(),
			section: NewSection(),
		}
		now := time.Now()

		signal.section.SetAnchor("BTC/USD")
		signal.section.ObservePrice("BTC/USD", 100, now)

		frame := marketFrame(
			kraken.TickerData{
				Symbol:    "BTC/USD",
				Last:      krakendecimal.NewFromFloat64(100),
				ChangePct: 5,
				Timestamp: now,
			},
			kraken.TickerData{
				Symbol:    "ETH/USD",
				Timestamp: now,
			},
		)

		measurements, err := signal.Calculate(frame)

		Convey("Then it should omit the incomplete follower", func() {
			So(err, ShouldBeNil)

			_, hasFollower := hasMeasurement(measurements, "ETH/USD", types.MetricStrength)
			So(hasFollower, ShouldBeFalse)
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
		_, _ = signal.Calculate(frame)
	}
}
