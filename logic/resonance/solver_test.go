package resonance

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestExtractFeatures(t *testing.T) {
	Convey("Given measurements with raw and normalized metric values", t, func() {
		normalized := 0.25
		notFinite := math.Inf(1)
		thesis := types.NewThesis()
		thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				"relative_depth": {
					Raw:        25_000_000,
					Normalized: &normalized,
				},
				"raw_notional": {
					Raw: 25_000_000,
				},
				"invalid": {
					Raw:        1,
					Normalized: &notFinite,
				},
			},
		}})

		features := (&Solver{}).extractFeatures(thesis)

		Convey("Then only the finite normalized reading enters predictive coding", func() {
			So(features, ShouldResemble, map[string]float64{
				"liquidity:BTC/USD::relative_depth": normalized,
			})
		})
	})
}

func TestUpdate(t *testing.T) {
	Convey("Given normalized predictive-coding features", t, func() {
		first := 0.25
		second := -0.5
		thesis := types.NewThesis()
		thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				"first":  {Normalized: &first},
				"second": {Normalized: &second},
			},
		}})
		solver := NewSolver(make(chan []byte, 1), nil)
		solver.alphaCtrl = NewAlphaController(solver.alpha, solver.alpha, solver.alpha)
		thesis.Tickers.Store("BTC/USD", kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       decimal.NewFromFloat64(99),
			Ask:       decimal.NewFromFloat64(101),
			Timestamp: time.Unix(1, 0),
		})

		err := solver.Update(thesis)

		Convey("Then surprise and energy are reported per input dimension", func() {
			So(err, ShouldBeNil)
			surprise, surpriseOK := thesis.Resonance.Load("surprise")
			energy, energyOK := thesis.Resonance.Load("energy")
			featureCount := float64(len(solver.featureSchema))

			So(surpriseOK, ShouldBeTrue)
			So(energyOK, ShouldBeTrue)
			So(surprise, ShouldAlmostEqual,
				solver.manifold.ReconstructionError()/math.Sqrt(featureCount))
			So(energy, ShouldAlmostEqual, solver.manifold.Energy()/featureCount)
		})

		Convey("Then the next market epoch produces a visible forward return curve", func() {
			thesis.Tickers.Store("BTC/USD", kraken.TickerData{
				Symbol:    "BTC/USD",
				Bid:       decimal.NewFromFloat64(100),
				Ask:       decimal.NewFromFloat64(102),
				Timestamp: time.Unix(2, 0),
			})

			err = solver.Update(thesis)
			curve, curveOK := thesis.Resonance.Load("forwardCurve")
			horizon, horizonOK := thesis.Resonance.Load("activeHorizon")

			So(err, ShouldBeNil)
			So(curveOK, ShouldBeTrue)
			So(horizonOK, ShouldBeTrue)
			So(curve, ShouldHaveLength, horizon.(int))
			So(solver.targetSamples, ShouldEqual, 1)
		})
	})
}
