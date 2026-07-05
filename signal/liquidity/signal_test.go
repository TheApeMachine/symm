package liquidity

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

const liquidityCrossSectionWarmupTicks = 4

func TestSignalIngestRoles(t *testing.T) {
	Convey("Given a liquidity signal", t, func() {
		signal := NewSignal(context.Background())

		Convey("When ingest roles are requested", func() {
			roles := signal.IngestRoles()

			Convey("Then it should consume ticker data", func() {
				So(roles, ShouldResemble, []string{"ticker"})
			})
		})
	})
}

func TestSignalMeasure(t *testing.T) {
	base := startAt(0)

	Convey("Given insufficient peer volume context", t, func() {
		crossSection := newTestCrossSection(t)
		signal := NewSignal(context.Background())
		defer func() { _ = signal.Close() }()

		ticker := tickerRow("SOLO/USD", 100, 1000, 0.1, base)
		measurements := measureTicker(t, signal, crossSection, ticker)

		Convey("When liquidity is measured", func() {
			Convey("Then it should abstain", func() {
				So(measurements, ShouldBeEmpty)
			})
		})
	})

	Convey("Given cross-section warmup and peak scarcity volume", t, func() {
		crossSection := newTestCrossSection(t)
		signal := NewSignal(context.Background())
		defer func() { _ = signal.Close() }()

		warmup(crossSection, signal, base)
		result := measureBest(t, signal, crossSection, "SCARCE/USD", 50, base)

		Convey("When liquidity is measured", func() {
			Convey("Then extreme scarcity should dominate", func() {
				So(result, ShouldNotBeNil)
				So(result.DominantCategory(), ShouldEqual, logic.CategoryExtremeScarcity)
				So(result.Metric("scarcityScore"), ShouldBeGreaterThan, result.Metric("medianScore"))
				So(result.Metric("scarcityScore"), ShouldBeGreaterThan, result.Metric("depthScore"))
			})
		})
	})

	Convey("Given cross-section warmup and median-band volume", t, func() {
		crossSection := newTestCrossSection(t)
		signal := NewSignal(context.Background())
		defer func() { _ = signal.Close() }()

		warmup(crossSection, signal, base)
		result := measureBest(t, signal, crossSection, "MEDIAN/USD", 1000, base)

		Convey("When liquidity is measured", func() {
			Convey("Then median depth should dominate", func() {
				So(result, ShouldNotBeNil)
				So(result.DominantCategory(), ShouldEqual, logic.CategoryMedianDepth)
				So(result.Metric("medianScore"), ShouldBeGreaterThan, result.Metric("scarcityScore"))
				So(result.Metric("medianScore"), ShouldBeGreaterThan, result.Metric("depthScore"))
			})
		})
	})

	Convey("Given cross-section warmup and deep volume", t, func() {
		crossSection := newTestCrossSection(t)
		signal := NewSignal(context.Background())
		defer func() { _ = signal.Close() }()

		warmup(crossSection, signal, base)
		result := measureBest(t, signal, crossSection, "DEEP/USD", 2500, base)

		Convey("When liquidity is measured", func() {
			Convey("Then robust liquidity should dominate", func() {
				So(result, ShouldNotBeNil)
				So(result.DominantCategory(), ShouldEqual, logic.CategoryRobustLiquidity)
				So(result.Metric("depthScore"), ShouldBeGreaterThan, result.Metric("scarcityScore"))
				So(result.Metric("depthScore"), ShouldBeGreaterThan, result.Metric("medianScore"))
			})
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := startAt(0)

	b.ReportAllocs()

	for b.Loop() {
		crossSection := newTestCrossSection(b)
		signal := NewSignal(context.Background())
		warmup(crossSection, signal, base)

		result := measureBest(b, signal, crossSection, "DEEP/USD", 2500, base)
		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if result.DominantCategory() != logic.CategoryRobustLiquidity {
			b.Fatalf("Measure classified %s, want %s",
				result.DominantCategory(), logic.CategoryRobustLiquidity)
		}

		_ = signal.Close()
	}
}

func warmup(
	crossSection *market.CrossSection,
	signal *Signal,
	base time.Time,
) {
	rows := []kraken.TickerData{
		tickerRow("BTC/USD", 100, 1100, 0.1, base),
		tickerRow("ETH/USD", 101, 950, 0.1, base),
		tickerRow("SOL/USD", 102, 1000, 0.1, base),
	}

	for tick := range liquidityCrossSectionWarmupTicks {
		for _, row := range rows {
			row.Timestamp = base.Add(time.Duration(tick) * time.Minute)
			_ = crossSection.Observe(kraken.TickerDataSlice{row})
			_, _ = signal.Measure(tickerInput(row), crossSection)
		}
	}
}

func measureBest(
	t testing.TB,
	signal *Signal,
	crossSection *market.CrossSection,
	symbol string,
	volume float64,
	base time.Time,
) *logic.Measurement {
	t.Helper()

	var best *logic.Measurement
	for tick := range liquidityCrossSectionWarmupTicks {
		ticker := tickerRow(symbol, 100, volume, 0.1, base.Add(time.Duration(tick+4)*time.Minute))
		measurements := measureTicker(t, signal, crossSection, ticker)

		if len(measurements) == 0 {
			continue
		}

		if best == nil || measurements[0].Confidence > best.Confidence {
			best = measurements[0]
		}
	}

	return best
}

func measureTicker(
	t testing.TB,
	signal *Signal,
	crossSection *market.CrossSection,
	ticker kraken.TickerData,
) []*logic.Measurement {
	t.Helper()

	if err := crossSection.Observe(kraken.TickerDataSlice{ticker}); err != nil {
		t.Fatal(err)
	}

	measurements, err := signal.Measure(tickerInput(ticker), crossSection)
	if err != nil {
		t.Fatal(err)
	}

	return measurements
}

func newTestCrossSection(t testing.TB) *market.CrossSection {
	t.Helper()

	crossSection, err := market.NewCrossSection(
		market.CrossSectionConfig{
			ReturnCap:  16,
			MinBars:    6,
			BreadthCap: 16,
		},
	)

	if err != nil {
		t.Fatal(err)
	}

	return crossSection
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
	volume float64,
	changePct float64,
	timestamp time.Time,
) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Last:      last,
		Volume:    volume,
		ChangePct: changePct,
		Timestamp: timestamp,
	}
}

func startAt(minutes int) time.Time {
	return time.Date(2026, 5, 30, 12, minutes, 0, 0, time.UTC)
}
