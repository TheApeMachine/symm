package logic

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

/*
metricsTickerRow builds a minimal, decimal-backed kraken.TickerData fixture
for feeding a real signal producer, mirroring the fixtures each signal
package's own tests use.
*/
func metricsTickerRow(symbol string, bid, ask, bidQty, askQty, volume, vwap float64) kraken.TickerData {
	return kraken.TickerData{
		Symbol: symbol,
		Bid:    decimal.NewFromFloat64(bid),
		BidQty: bidQty,
		Ask:    decimal.NewFromFloat64(ask),
		AskQty: askQty,
		Last:   decimal.NewFromFloat64((bid + ask) / 2),
		Volume: volume,
		Vwap:   vwap,
	}
}

/*
TestAnalyzerManifoldDepositsRealLiquidityMeasurement keeps analyzerMetrics aligned
with the liquidity ticker producer's emitted metric keys.
*/
func TestAnalyzerManifoldDepositsRealLiquidityMeasurement(t *testing.T) {
	Convey("Given a real liquidity ticker measurement from a live cross-section", t, func() {
		crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())
		So(err, ShouldBeNil)

		peerOne := metricsTickerRow("ETH/USD", 99, 101, 5, 5, 100, 100)
		peerTwo := metricsTickerRow("SOL/USD", 99, 101, 5, 5, 100, 100)
		subject := metricsTickerRow("BTC/USD", 999, 1001, 2, 2, 1, 1000)

		rows := []kraken.TickerData{subject, peerOne, peerTwo}
		now := time.Now()

		for index := range rows {
			rows[index].Timestamp = now
		}

		So(crossSection.Observe(rows), ShouldBeNil)

		measurements, err := liquidity.NewTicker().Measure(rows[0], crossSection)
		So(err, ShouldBeNil)
		So(measurements, ShouldHaveLength, 1)

		mapping, ok := analyzerMetrics[types.SourceLiquidity]["ticker"]
		So(ok, ShouldBeTrue)

		Convey("Then every axis the deposit mapping requires is present in the real output", func() {
			for _, metricKey := range mapping {
				_, present := measurements[0].Metrics[metricKey]
				So(present, ShouldBeTrue)
			}
		})

		Convey("When the measurement is run through the analyzer", func() {
			analyzer := NewAnalyzer(nil, nil)
			theses := analyzer.Update(measurements)

			Convey("Then it deposits into an opened manifold solver instead of being rejected", func() {
				So(theses, ShouldHaveLength, 1)

				manifoldFound, ok := analyzer.manifolds.Load("BTC/USD")
				So(ok, ShouldBeTrue)

				manifold := manifoldFound.(*Manifold)
				So(manifold, ShouldNotBeNil)
				So(manifold.solver, ShouldNotBeNil)
			})
		})
	})
}
