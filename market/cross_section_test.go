package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestCrossSectionAggregateCache(testingTB *testing.T) {
	Convey("Given a warmed cross section from ticker rows", testingTB, func() {
		crossSection, err := NewCrossSection(DefaultCrossSectionConfig())

		So(err, ShouldBeNil)

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		for index, name := range []string{"BTC/USD", "ETH/USD", "SOL/USD"} {
			So(observeTicker(
				crossSection,
				base.Add(time.Duration(index)*time.Minute),
				tickerRow(name, 100+float64(index), 1000+float64(index), 0.01),
			), ShouldBeNil)
		}

		Convey("It should serve breadth and volumes from cached aggregates", func() {
			So(crossSection.Breadth(), ShouldAlmostEqual, 1, 1e-9)
			So(len(crossSection.Volumes()), ShouldEqual, 3)
			So(crossSection.IsLeader("BTC/USD", 0.05), ShouldBeTrue)
		})
	})
}

func TestCrossSectionLeader(testingTB *testing.T) {
	Convey("Given a universe where one obscure pair moves hardest", testingTB, func() {
		crossSection, err := NewCrossSection(DefaultCrossSectionConfig())

		So(err, ShouldBeNil)

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		rows := []struct {
			name   string
			change float64
		}{
			{"BTC/USD", 0.01},
			{"ETH/USD", 0.012},
			{"SOL/USD", 0.008},
			{"UNFI/USD", 4.15},
		}

		for index, sample := range rows {
			So(observeTicker(
				crossSection,
				base.Add(time.Duration(index)*time.Minute),
				tickerRow(sample.name, 100+float64(index), 1000, sample.change),
			), ShouldBeNil)
		}

		Convey("It should anchor on the hardest mover, never a fixed major", func() {
			So(crossSection.Leader(), ShouldEqual, "UNFI/USD")
		})
	})
}

func TestCrossSectionLeaderEmptyWhenFlat(testingTB *testing.T) {
	Convey("Given a flat universe with no breakout", testingTB, func() {
		crossSection, err := NewCrossSection(DefaultCrossSectionConfig())

		So(err, ShouldBeNil)

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		for index, name := range []string{"BTC/USD", "ETH/USD", "SOL/USD"} {
			So(observeTicker(
				crossSection,
				base.Add(time.Duration(index)*time.Minute),
				tickerRow(name, 100+float64(index), 1000, 0.01),
			), ShouldBeNil)
		}

		Convey("It should report no leader rather than picking one by vibes", func() {
			So(crossSection.Leader(), ShouldEqual, "")
		})
	})
}

func TestCrossSectionSymbolSamples(testingTB *testing.T) {
	Convey("Given timestamped ticker observations", testingTB, func() {
		crossSection, err := NewCrossSection(CrossSectionConfig{
			ReturnCap:  3,
			MinBars:    2,
			BreadthCap: 3,
		})

		So(err, ShouldBeNil)

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		for index := range 5 {
			So(observeTicker(
				crossSection,
				base.Add(time.Duration(index)*time.Second),
				tickerRow("BTC/USD", 100+float64(index), 1000, 0.01),
			), ShouldBeNil)
		}

		Convey("It should retain the return-cap plus one timestamped prices", func() {
			samples := crossSection.SymbolSamples("BTC/USD", 4)
			returns := crossSection.SymbolReturns("BTC/USD", 3)

			So(samples, ShouldHaveLength, 4)
			So(returns, ShouldHaveLength, 3)
			So(samples[0].At, ShouldEqual, base.Add(time.Second))
			So(samples[3].Value, ShouldEqual, 104)
		})
	})
}

func TestCrossSectionRegime(testingTB *testing.T) {
	Convey("Given a warmed cross section", testingTB, func() {
		crossSection, err := NewCrossSection(DefaultCrossSectionConfig())

		So(err, ShouldBeNil)

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		rows := []struct {
			name   string
			change float64
		}{
			{"BTC/USD", 0.02},
			{"ETH/USD", -0.01},
			{"SOL/USD", 0.03},
		}

		for index, sample := range rows {
			So(observeTicker(
				crossSection,
				base.Add(time.Duration(index)*time.Minute),
				tickerRow(sample.name, 100+float64(index), 1000, sample.change),
			), ShouldBeNil)
		}

		Convey("It should publish finite backend-owned regime axes", func() {
			reading := crossSection.Regime()

			So(reading.Volatility, ShouldBeGreaterThanOrEqualTo, 0)
			So(reading.Trend, ShouldBeGreaterThan, 0)
			So(reading.Bullish, ShouldBeGreaterThan, 0)
			So(reading.Bearish, ShouldBeGreaterThan, 0)
			So(reading.Choppiness, ShouldBeGreaterThanOrEqualTo, 0)
			So(reading.Observed, ShouldEqual, 3)
			So(reading.Confidence, ShouldBeGreaterThan, 0)
			So(reading.At.IsZero(), ShouldBeFalse)
		})
	})
}

func TestPeerWindowSnapshotCache(testingTB *testing.T) {
	Convey("Given a warmed cross section from ticker rows", testingTB, func() {
		crossSection, err := NewCrossSection(DefaultCrossSectionConfig())

		So(err, ShouldBeNil)

		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		for index, name := range []string{"BTC/USD", "ETH/USD", "SOL/USD"} {
			price := 100.0 + float64(index)

			for step := range 5 {
				So(observeTicker(
					crossSection,
					base.Add(time.Duration(step)*time.Minute),
					tickerRow(name, price*(1+0.01*float64(step)), 1000, 0.01),
				), ShouldBeNil)
			}
		}

		first := crossSection.PeerCache.Snapshot(crossSection, 3)
		second := crossSection.PeerCache.Snapshot(crossSection, 3)

		Convey("It should reuse the cached snapshot for the same window", func() {
			So(len(first.MarketReturns), ShouldEqual, len(second.MarketReturns))
			So(first.MarketReturns, ShouldResemble, second.MarketReturns)
		})
	})
}

func BenchmarkPeerWindowSnapshot(b *testing.B) {
	crossSection, err := NewCrossSection(DefaultCrossSectionConfig())

	if err != nil {
		b.Fatal(err)
	}

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for index := range 32 {
		_ = observeTicker(
			crossSection,
			base.Add(time.Duration(index)*time.Second),
			tickerRow("SYM/USD", 100+float64(index%5), 1000, 0.01),
		)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = crossSection.PeerCache.Snapshot(crossSection, 3)
	}
}

func observeTicker(
	crossSection *CrossSection,
	at time.Time,
	rows ...map[string]any,
) error {
	tickers := make(kraken.TickerDataSlice, 0, len(rows))
	for _, row := range rows {
		tickers = append(tickers, kraken.TickerData{
			Symbol:    row["symbol"].(string),
			Last:      row["last"].(float64),
			Volume:    row["volume"].(float64),
			ChangePct: row["change_pct"].(float64),
			Timestamp: at,
		})
	}

	return crossSection.Observe(tickers)
}

func tickerRow(
	symbol string,
	price float64,
	volume float64,
	change float64,
) map[string]any {
	return map[string]any{
		"symbol":     symbol,
		"last":       price,
		"volume":     volume,
		"change_pct": change * 100,
	}
}
