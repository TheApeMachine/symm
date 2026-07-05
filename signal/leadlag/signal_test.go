package leadlag

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func leadlagTestSpacing() time.Duration {
	return 15 * time.Second
}

func TestSectionPriceSamples(t *testing.T) {
	Convey("Given ticker observations", t, func() {
		section := NewSection()
		section.SetAnchor("BTC/EUR")
		start := startAt(0)

		for index := range 20 {
			section.ObservePrice(
				"BTC/EUR",
				100+float64(index),
				start.Add(time.Duration(index)*leadlagTestSpacing()),
			)
		}

		Convey("When price samples are counted", func() {
			count := section.PriceSampleCount("BTC/EUR")

			Convey("Then it should retain enough samples for correlation", func() {
				So(count, ShouldBeGreaterThanOrEqualTo, minCorrelationSamples(16))
			})
		})
	})
}

func TestSectionCrossLagInsufficientData(t *testing.T) {
	Convey("Given sparse histories", t, func() {
		section := NewSection()
		section.SetAnchor("BTC/EUR")
		now := startAt(0)

		section.ObservePrice("BTC/EUR", 100, now)
		section.ObservePrice("ETH/EUR", 200, now)

		Convey("When lag features are requested", func() {
			features := section.Features("ETH/EUR")

			Convey("Then it should refuse to score lag", func() {
				So(features.LagOK, ShouldBeFalse)
			})
		})
	})
}

func TestRecentPathMove(t *testing.T) {
	Convey("Given a flat anchor path across the lag window", t, func() {
		start := startAt(0)
		samples := make([]priceSample, minCorrelationSamples(16))

		for index := range minCorrelationSamples(16) {
			samples[index] = priceSample{
				at:    start.Add(time.Duration(index) * 2 * time.Minute),
				value: 50000,
			}
		}

		Convey("When recent path move is measured", func() {
			move, ok := recentPathMove(
				samples,
				medianSampleSpacing(samples)*time.Duration(maxLagBarsForCount(16)),
			)

			Convey("Then it should report a near-zero move", func() {
				So(ok, ShouldBeTrue)
				So(move, ShouldBeLessThan, 1e-6)
			})
		})
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given only two spaced ticks on anchor and follower", t, func() {
		crossSection := leaderCrossSection(t)
		signal := NewSignal(context.Background())
		defer func() { _ = signal.Close() }()

		signal.Section.SetAnchor("BTC/USD")
		start := startAt(60)
		sampleCount := pearsonFloor + 1

		for index := range sampleCount {
			at := start.Add(time.Duration(index) * leadlagTestSpacing())
			signal.Section.ObservePrice("BTC/USD", 50000+float64(index)*10, at)
			signal.Section.ObservePrice("ETH/USD", 3000+float64(index)*0.5, at)
		}

		follower := tickerRow(
			"ETH/USD",
			3000,
			0,
			start.Add(time.Duration(sampleCount)*leadlagTestSpacing()),
		)
		result := measureTicker(t, signal, crossSection, follower)

		Convey("When lead-lag is measured", func() {
			Convey("Then it should emit evidence on the first valid observation", func() {
				So(result, ShouldNotBeNil)
				So(result.Confidence, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestSignalMeasureCategorySemantics(t *testing.T) {
	Convey("Given anchor impulse with a lagging follower", t, func() {
		crossSection := leaderCrossSection(t)
		signal := NewSignal(context.Background())
		defer func() { _ = signal.Close() }()

		signal.Section.SetAnchor("BTC/USD")

		const (
			flatSamples  = 200
			trackSamples = 140
			spikeSamples = 20
		)
		totalSamples := flatSamples + trackSamples + spikeSamples
		lagDelay := maxLagBarsForCount(totalSamples) - 1
		start := startAt(60)

		for index := range flatSamples {
			at := start.Add(time.Duration(index) * leadlagTestSpacing())
			signal.Section.ObservePrice("BTC/USD", 50000, at)
			signal.Section.ObservePrice("ETH/USD", 3000, at)
		}

		for range 13 {
			_ = signal.Section.Features("BTC/USD")
		}

		for index := range trackSamples {
			global := flatSamples + index
			at := start.Add(time.Duration(global) * leadlagTestSpacing())
			anchorPrice := 50000.0 + float64(index)*5
			followerPrice := 3000.0

			if index >= lagDelay {
				followerPrice = 3000.0 + float64(index-lagDelay)*0.0012
			}

			signal.Section.ObservePrice("BTC/USD", anchorPrice, at)
			signal.Section.ObservePrice("ETH/USD", followerPrice, at)
			_ = signal.Section.Features("ETH/USD")
		}

		trackAnchorEnd := 50000.0 + float64(trackSamples-1)*5
		for index := range spikeSamples {
			global := flatSamples + trackSamples + index
			at := start.Add(time.Duration(global) * leadlagTestSpacing())
			anchorPrice := trackAnchorEnd + float64(index+1)*2500

			signal.Section.ObservePrice("BTC/USD", anchorPrice, at)
			signal.Section.ObservePrice("ETH/USD", 3000, at)
			_ = signal.Section.Features("ETH/USD")
		}

		follower := tickerRow(
			"ETH/USD",
			3000,
			0,
			start.Add(time.Duration(totalSamples)*leadlagTestSpacing()),
		)
		result := measureTicker(t, signal, crossSection, follower)

		Convey("When lead-lag is measured", func() {
			Convey("Then decoupled move should dominate", func() {
				So(result, ShouldNotBeNil)
				So(result.DominantCategory(), ShouldEqual, logic.CategoryDecoupledMove)
				So(result.Metric("decoupled"), ShouldBeGreaterThan, result.Metric("sync"))
				So(result.Confidence, ShouldBeGreaterThan, 0.25)
			})
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := startAt(60)
	spacing := leadlagTestSpacing()
	crossSection := leaderCrossSection(b)

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background())

		for index := range 48 {
			at := base.Add(time.Duration(index) * spacing)
			signal.Section.ObservePrice("BTC/USD", 50000+float64(index)*5, at)
			signal.Section.ObservePrice("ETH/USD", 3000+float64(index)*0.01, at)

			follower := tickerRow("ETH/USD", 3000+float64(index)*0.01, 0, at)
			_ = measureTicker(b, signal, crossSection, follower)
		}

		_ = signal.Close()
	}
}

func leaderCrossSection(t testing.TB) *market.CrossSection {
	t.Helper()

	crossSection, err := market.NewCrossSection(market.DefaultCrossSectionConfig())
	if err != nil {
		t.Fatal(err)
	}

	base := startAt(0)
	rows := []struct {
		name   string
		change float64
	}{
		{"BTC/USD", 0.9},
		{"ETH/USD", 0.01},
		{"SOL/USD", 0.012},
	}

	for index, row := range rows {
		ticker := tickerRow(
			row.name,
			100+float64(index),
			row.change*100,
			base.Add(time.Duration(index)*time.Minute),
		)

		if err := crossSection.Observe(kraken.TickerDataSlice{ticker}); err != nil {
			t.Fatal(err)
		}
	}

	return crossSection
}

func measureTicker(
	t testing.TB,
	signal *Signal,
	crossSection *market.CrossSection,
	ticker kraken.TickerData,
) *logic.Measurement {
	t.Helper()

	measurements, err := signal.Measure(tickerInput(ticker), crossSection)
	if err != nil {
		t.Fatal(err)
	}

	if len(measurements) == 0 {
		return nil
	}

	return measurements[0]
}

func tickerInput(ticker kraken.TickerData) market.Input {
	return market.Input{
		Role:   "ticker",
		At:     ticker.Timestamp,
		Ticker: kraken.TickerDataSlice{ticker},
	}
}

func tickerRow(
	symbol string,
	last float64,
	changePct float64,
	timestamp time.Time,
) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Last:      last,
		Volume:    1000,
		ChangePct: changePct,
		Timestamp: timestamp,
	}
}

func startAt(minutes int) time.Time {
	return time.Date(2026, 6, 11, 11, minutes, 0, 0, time.UTC)
}
