package risk

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/correlation"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/wallet"
)

func TestWindowObserveSymbol(t *testing.T) {
	Convey("Given a rolling price window", t, func() {
		window := NewWindow()
		at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		window.ObserveSymbol("BTC/EUR", 100)
		window.ObserveSymbolAt("ETH/EUR", 2000, at)

		Convey("It should track the latest mark per symbol", func() {
			So(window.Mark("BTC/EUR"), ShouldAlmostEqual, 100, 1e-12)
			So(window.Mark("ETH/EUR"), ShouldAlmostEqual, 2000, 1e-12)
			So(window.Marks()["BTC/EUR"], ShouldAlmostEqual, 100, 1e-12)
		})
	})
}

func TestPortfolioObserveSymbol(t *testing.T) {
	Convey("Given a portfolio tracker", t, func() {
		portfolio := NewPortfolio()
		portfolio.ObserveSymbol("BTC/EUR", 100)

		Convey("It should expose the latest mark through the portfolio", func() {
			So(portfolio.Mark("BTC/EUR"), ShouldAlmostEqual, 100, 1e-12)
		})
	})
}

func TestDrawdownPct(t *testing.T) {
	Convey("Given peak equity above current marks", t, func() {
		drawdown := &Drawdown{}
		drawdown.UpdatePeakEquity(200)
		tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 150, 0.26)

		pct := drawdown.Pct(tradingWallet, map[string]float64{})

		Convey("It should report drawdown as a unit fraction", func() {
			So(pct, ShouldAlmostEqual, 0.25, 1e-9)
		})
	})
}

func TestMatrixSystemicConcentration(t *testing.T) {
	Convey("Given a perfectly correlated pair matrix", t, func() {
		matrix := &Matrix{rows: [][]float64{
			{1, 1},
			{1, 1},
		}}

		concentration, ok := matrix.SystemicConcentration()

		Convey("It should map the principal eigenvalue to unit concentration", func() {
			So(ok, ShouldBeTrue)
			So(concentration, ShouldAlmostEqual, 1, 1e-9)
		})
	})

	Convey("Given an identity matrix", t, func() {
		matrix := &Matrix{rows: [][]float64{
			{1, 0},
			{0, 1},
		}}

		concentration, ok := matrix.SystemicConcentration()

		Convey("It should report zero systemic concentration", func() {
			So(ok, ShouldBeTrue)
			So(concentration, ShouldEqual, 0)
		})
	})
}

func TestWindowMarketRegimeDead(t *testing.T) {
	Convey("Given a flat price path", t, func() {
		window := NewWindow()
		at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		for index := range window.minSamples + 2 {
			window.ObserveSymbolAt("BTC/EUR", 100, at.Add(time.Duration(index)*time.Second))
		}

		regime := window.MarketRegime("BTC/EUR")

		Convey("It should classify zero net drift as dead", func() {
			So(regime, ShouldEqual, engine.RegimeDead)
		})
	})
}

func TestWindowReturnsFromSamplesSkipsBadPrices(t *testing.T) {
	Convey("Given samples with non-positive prices", t, func() {
		window := NewWindow()
		returns := window.returnsFromSamples([]correlation.PriceSample{
			{Price: 0},
			{Price: 100},
			{Price: 110},
		})

		Convey("It should not fabricate returns across invalid endpoints", func() {
			So(len(returns), ShouldEqual, 1)
			So(returns[0], ShouldAlmostEqual, math.Log(110.0/100.0), 1e-12)
		})
	})
}

func BenchmarkWindowObserveSymbolAt(b *testing.B) {
	window := NewWindow()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	price := 100.0

	for b.Loop() {
		price *= math.Exp(0.0001)
		window.ObserveSymbolAt("BTC/EUR", price, at)
	}
}
