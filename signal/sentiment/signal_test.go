package sentiment

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

var sentimentCategories = []logic.CategoryType{
	logic.CategoryRiskOnSurge,
	logic.CategoryDivergentMove,
	logic.CategorySystemicSlump,
}

func TestSignalIngestRoles(t *testing.T) {
	Convey("Given a sentiment signal", t, func() {
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
	Convey("Given broad positive market breadth with a leading symbol", t, func() {
		crossSection := newTestCrossSection(t)
		signal := NewSignal(context.Background())
		defer func() { _ = signal.Close() }()

		base := startAt(0)
		symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD"}
		var result *logic.Measurement

		for tick := range 20 {
			at := base.Add(time.Duration(tick) * time.Minute)

			for symbolIndex, symbol := range symbols {
				changePct := 1.0 + float64(tick)*0.05 + float64(symbolIndex)*0.1
				ticker := tickerRow(symbol, 100+float64(tick)+float64(symbolIndex), changePct, at)
				result = observeAndMeasure(t, signal, crossSection, ticker)
			}
		}

		Convey("When sentiment is measured", func() {
			Convey("Then risk-on surge should dominate", func() {
				So(result, ShouldNotBeNil)
				So(result.Confidence, ShouldBeGreaterThan, 1.0/3.0)
				So(result.DominantCategory(), ShouldEqual, logic.CategoryRiskOnSurge)
				So(result.Metric("surgeScore"), ShouldBeGreaterThan, 0)
			})
		})
	})

	Convey("Given a local leader in a weak cross-section", t, func() {
		crossSection := newTestCrossSection(t)
		signal := NewSignal(context.Background())
		defer func() { _ = signal.Close() }()

		base := startAt(0)
		flatSymbols := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD"}

		for tick := range 16 {
			at := base.Add(time.Duration(tick) * time.Minute)

			for symbolIndex, symbol := range flatSymbols {
				changePct := 1.5 + float64(tick)*0.05 + float64(symbolIndex)*0.1
				ticker := tickerRow(symbol, 100+float64(tick), changePct, at)
				_ = observeAndMeasure(t, signal, crossSection, ticker)
			}
		}

		at := base.Add(16 * time.Minute)
		for symbolIndex, symbol := range flatSymbols {
			ticker := tickerRow(symbol, 100, -1-float64(symbolIndex)*0.2, at)
			_ = observeAndMeasure(t, signal, crossSection, ticker)
		}

		leader := tickerRow("LEAD/USD", 120, 6, at.Add(time.Minute))
		result := observeAndMeasure(t, signal, crossSection, leader)

		Convey("When sentiment is measured", func() {
			Convey("Then divergent move should dominate", func() {
				So(result, ShouldNotBeNil)
				So(result.DominantCategory(), ShouldEqual, logic.CategoryDivergentMove)
				So(result.Metric("divergentScore"), ShouldBeGreaterThan, 0)
				So(result.Metric("divergentScore"), ShouldBeGreaterThan, result.Metric("surgeScore"))
				So(result.Confidence, ShouldBeGreaterThan, 1.0/3.0)
			})
		})
	})

	Convey("Given weak breadth without leadership", t, func() {
		crossSection := newTestCrossSection(t)
		signal := NewSignal(context.Background())
		defer func() { _ = signal.Close() }()

		base := startAt(0)
		symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD"}

		for tick := range 20 {
			at := base.Add(time.Duration(tick) * time.Minute)

			for symbolIndex, symbol := range symbols {
				changePct := -1.0 - float64(tick)*0.05 - float64(symbolIndex)*0.1
				ticker := tickerRow(symbol, 100-float64(tick), changePct, at)
				_ = observeAndMeasure(t, signal, crossSection, ticker)
			}
		}

		laggard := tickerRow("FLAT/USD", 100, -0.4, base.Add(21*time.Minute))
		result := observeAndMeasure(t, signal, crossSection, laggard)

		Convey("When sentiment is measured", func() {
			Convey("Then systemic slump should dominate", func() {
				So(result, ShouldNotBeNil)
				So(result.DominantCategory(), ShouldEqual, logic.CategorySystemicSlump)
				So(result.Metric("slumpScore"), ShouldBeGreaterThan, 0)
				So(result.Metric("slumpScore"), ShouldBeGreaterThan, result.Metric("surgeScore"))
				So(result.Confidence, ShouldBeGreaterThan, 1.0/3.0)
			})
		})
	})

	Convey("Given leadership before a threshold exists", t, func() {
		crossSection := newTestCrossSection(t)
		signal := NewSignal(context.Background())
		defer func() { _ = signal.Close() }()

		leader := tickerRow("LEAD/USD", 100, 5, startAt(0))

		if err := crossSection.Observe(kraken.TickerDataSlice{leader}); err != nil {
			t.Fatal(err)
		}

		measurements, err := signal.Measure(tickerInput(leader), crossSection)

		Convey("When sentiment is measured", func() {
			Convey("Then it should abstain", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldBeEmpty)
			})
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := startAt(0)
	symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD"}

	b.ReportAllocs()

	for b.Loop() {
		crossSection := newTestCrossSection(b)
		signal := NewSignal(context.Background())

		for tick := range 20 {
			at := base.Add(time.Duration(tick) * time.Minute)

			for symbolIndex, symbol := range symbols {
				ticker := tickerRow(symbol, 100+float64(tick), 1+float64(symbolIndex), at)
				_ = observeAndMeasure(b, signal, crossSection, ticker)
			}
		}

		_ = signal.Close()
	}
}

func observeAndMeasure(
	t testing.TB,
	signal *Signal,
	crossSection *market.CrossSection,
	ticker kraken.TickerData,
) *logic.Measurement {
	t.Helper()

	if err := crossSection.Observe(kraken.TickerDataSlice{ticker}); err != nil {
		t.Fatal(err)
	}

	measurements, err := signal.Measure(tickerInput(ticker), crossSection)
	if err != nil {
		t.Fatal(err)
	}

	if len(measurements) == 0 {
		return nil
	}

	return measurements[0]
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
	return time.Date(2026, 5, 30, 12, minutes, 0, 0, time.UTC)
}
