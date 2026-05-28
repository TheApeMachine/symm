package trader

import (
	"math"
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRiskAdjustmentDampensDrawdown(t *testing.T) {
	Convey("Given equity below the portfolio peak", t, func() {
		originalDrawdown := config.System.MaxPortfolioDrawdownPct
		config.System.MaxPortfolioDrawdownPct = 0.20
		t.Cleanup(func() { config.System.MaxPortfolioDrawdownPct = originalDrawdown })

		risk := NewRisk()
		risk.peakEquity = 200
		wallet := NewWallet(PaperWallet, "EUR", 180, 0.26)
		measurement := engine.Measurement{
			Pairs: []asset.Pair{{Wsname: "BTC/EUR"}},
			Last:  100,
		}

		adjustment := risk.Adjustment(wallet, measurement, nil)

		Convey("It should scale confidence by remaining drawdown capacity", func() {
			So(adjustment.Reason, ShouldEqual, "drawdown_dampened")
			So(adjustment.Dampener, ShouldAlmostEqual, 0.5, 1e-9)
		})
	})
}

func TestRiskAdjustmentDampensCovariance(t *testing.T) {
	Convey("Given a candidate that concentrates portfolio covariance", t, func() {
		originalCorrelation := config.System.MaxSymbolCorrelation
		originalBar := config.System.CorrelationBarSeconds
		config.System.MaxSymbolCorrelation = 0.85
		config.System.CorrelationBarSeconds = 10
		t.Cleanup(func() {
			config.System.MaxSymbolCorrelation = originalCorrelation
			config.System.CorrelationBarSeconds = originalBar
		})

		risk := NewRisk()
		returns := []float64{
			0.01, -0.005, 0.002, 0.004, -0.001, 0.003,
			0.002, -0.002, 0.001, 0.004, 0.003, 0.002,
		}
		start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		interval := 10 * time.Second

		seedRiskPrices(risk, "BTC/EUR", 100, returns, interval, start)
		seedRiskPrices(risk, "ETH/EUR", 50, returns, interval, start)

		wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
		measurement := engine.Measurement{
			Pairs: []asset.Pair{{Wsname: "ETH/EUR"}},
			Last:  50,
		}

		adjustment := risk.Adjustment(wallet, measurement, []string{"BTC/EUR"})

		Convey("It should throttle the candidate through the systemic eigenvalue", func() {
			So(adjustment.HasSystemicMeasure, ShouldBeTrue)
			So(adjustment.SystemicCorrelation, ShouldAlmostEqual, 1, 1e-6)
			So(adjustment.Reason, ShouldEqual, "covariance_dampened")
			So(adjustment.Dampener, ShouldEqual, 0)
		})
	})
}

func TestRiskMarketRegime(t *testing.T) {
	Convey("Given directional and inefficient return paths", t, func() {
		risk := NewRisk()
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

		seedRiskPrices(risk, "BULL/EUR", 100, bullishReturns, interval, start)
		seedRiskPrices(risk, "CHOP/EUR", 100, choppyReturns, interval, start)

		Convey("It should classify by dynamic path efficiency", func() {
			So(risk.MarketRegime("BULL/EUR"), ShouldEqual, engine.RegimeBullish)
			So(risk.MarketRegime("CHOP/EUR"), ShouldEqual, engine.RegimeChoppy)
		})
	})
}

func TestPrincipalEigenvalue(t *testing.T) {
	Convey("Given a perfectly correlated two-asset matrix", t, func() {
		eigenvalue, ok := principalEigenvalue([][]float64{
			{1, 1},
			{1, 1},
		})

		Convey("It should return the systemic mode", func() {
			So(ok, ShouldBeTrue)
			So(math.Abs(eigenvalue-2), ShouldBeLessThan, 1e-9)
		})
	})
}

func BenchmarkRiskAdjustment(b *testing.B) {
	risk := NewRisk()
	returns := []float64{
		0.01, -0.005, 0.002, 0.004, -0.001, 0.003,
		0.002, -0.002, 0.001, 0.004, 0.003, 0.002,
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	interval := 10 * time.Second
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	measurement := engine.Measurement{
		Pairs: []asset.Pair{{Wsname: "ETH/EUR"}},
		Last:  50,
	}

	seedRiskPrices(risk, "BTC/EUR", 100, returns, interval, start)
	seedRiskPrices(risk, "ETH/EUR", 50, returns, interval, start)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = risk.Adjustment(wallet, measurement, []string{"BTC/EUR"})
	}
}
