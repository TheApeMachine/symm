package risk

import (
	"math"
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/wallet"

	. "github.com/smartystreets/goconvey/convey"
)

type priceSeed struct {
	portfolio *Portfolio
}

func (seed *priceSeed) observeReturns(
	symbol string,
	basePrice float64,
	returns []float64,
	interval time.Duration,
	start time.Time,
) {
	seed.portfolio.ObserveSymbolAt(symbol, basePrice, start)

	price := basePrice

	for index, logReturn := range returns {
		price *= math.Exp(logReturn)
		seed.portfolio.ObserveSymbolAt(
			symbol,
			price,
			start.Add(time.Duration(index+1)*interval),
		)
	}
}

func TestAdjusterDampensDrawdown(t *testing.T) {
	Convey("Given equity below the portfolio peak", t, func() {
		originalDrawdown := config.System.MaxPortfolioDrawdownPct
		config.System.MaxPortfolioDrawdownPct = 0.20
		t.Cleanup(func() { config.System.MaxPortfolioDrawdownPct = originalDrawdown })

		portfolio := NewPortfolio()
		portfolio.UpdatePeakEquity(200)
		tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 180, 0.26)
		measurement := engine.Measurement{
			Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
			Last:  100,
		}

		adjustment := portfolio.Adjust(tradingWallet, measurement, nil)

		Convey("It should scale confidence by remaining drawdown capacity", func() {
			So(adjustment.Reason, ShouldEqual, "drawdown_dampened")
			So(adjustment.Dampener, ShouldAlmostEqual, 0.5, 1e-9)
		})
	})
}

func TestAdjusterDampensCovariance(t *testing.T) {
	Convey("Given a candidate that concentrates portfolio covariance", t, func() {
		originalCorrelation := config.System.MaxSymbolCorrelation
		originalBar := config.System.CorrelationBarSeconds
		config.System.MaxSymbolCorrelation = 0.85
		config.System.CorrelationBarSeconds = 10
		t.Cleanup(func() {
			config.System.MaxSymbolCorrelation = originalCorrelation
			config.System.CorrelationBarSeconds = originalBar
		})

		portfolio := NewPortfolio()
		returns := []float64{
			0.01, -0.005, 0.002, 0.004, -0.001, 0.003,
			0.002, -0.002, 0.001, 0.004, 0.003, 0.002,
		}
		start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		interval := 10 * time.Second
		seed := priceSeed{portfolio: portfolio}

		seed.observeReturns("BTC/EUR", 100, returns, interval, start)
		seed.observeReturns("ETH/EUR", 50, returns, interval, start)

		tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)
		measurement := engine.Measurement{
			Pairs: []asset.Pair{{Wsname: "ETH/EUR"}},
			Last:  50,
		}

		adjustment := portfolio.Adjust(tradingWallet, measurement, []string{"BTC/EUR"})

		Convey("It should throttle the candidate through the systemic eigenvalue", func() {
			So(adjustment.HasSystemicMeasure, ShouldBeTrue)
			So(adjustment.SystemicCorrelation, ShouldAlmostEqual, 1, 1e-6)
			So(adjustment.Reason, ShouldEqual, "covariance_dampened")
			So(adjustment.Dampener, ShouldEqual, 0)
		})
	})
}

func TestPortfolioMarketRegime(t *testing.T) {
	Convey("Given directional and inefficient return paths", t, func() {
		portfolio := NewPortfolio()
		start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		interval := 10 * time.Second
		bullishReturns := []float64{
			0.01, 0.006, 0.004, 0.003, 0.002, 0.001,
			0.003, 0.002, 0.001, 0.002, 0.003, 0.001,
		}
		choppyReturns := []float64{
			0.01, -0.01, 0.008, -0.008, 0.006, -0.006,
			0.005, -0.005, 0.004, -0.004, 0.003, -0.003,
		}
		seed := priceSeed{portfolio: portfolio}

		seed.observeReturns("BULL/EUR", 100, bullishReturns, interval, start)
		seed.observeReturns("CHOP/EUR", 100, choppyReturns, interval, start)

		Convey("It should classify by dynamic path efficiency", func() {
			So(portfolio.MarketRegime("BULL/EUR"), ShouldEqual, engine.RegimeBullish)
			So(portfolio.MarketRegime("CHOP/EUR"), ShouldEqual, engine.RegimeChoppy)
		})
	})
}

func TestMatrixPrincipalEigenvalue(t *testing.T) {
	Convey("Given a perfectly correlated two-asset matrix", t, func() {
		matrix := &Matrix{rows: [][]float64{
			{1, 1},
			{1, 1},
		}}
		eigenvalue, ok := matrix.PrincipalEigenvalue()

		Convey("It should return the systemic mode", func() {
			So(ok, ShouldBeTrue)
			So(math.Abs(eigenvalue-2), ShouldBeLessThan, 1e-9)
		})
	})
}

func BenchmarkPortfolioAdjust(b *testing.B) {
	portfolio := NewPortfolio()
	returns := []float64{
		0.01, -0.005, 0.002, 0.004, -0.001, 0.003,
		0.002, -0.002, 0.001, 0.004, 0.003, 0.002,
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	interval := 10 * time.Second
	seed := priceSeed{portfolio: portfolio}

	seed.observeReturns("BTC/EUR", 100, returns, interval, start)
	seed.observeReturns("ETH/EUR", 50, returns, interval, start)

	tradingWallet := wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26)
	measurement := engine.Measurement{
		Pairs: []asset.Pair{{Wsname: "ETH/EUR"}},
		Last:  50,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = portfolio.Adjust(tradingWallet, measurement, []string{"BTC/EUR"})
	}
}
