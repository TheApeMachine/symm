package resonance

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
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
			So(features, ShouldResemble, map[string]map[string]float64{
				"BTC/USD": {
					"liquidity:BTC/USD:relative_depth": normalized,
				},
			})
		})
	})

	Convey("Given a relative reading whose peer rotates", t, func() {
		normalized := 0.5
		thesis := types.NewThesis()

		reading := func(peer string) *types.Measurement {
			return &types.Measurement{
				Source: types.SourceLeadLag,
				Symbol: "BTC/USD",
				Peer:   peer,
				Metrics: map[string]types.MetricSample{
					"direction": {Raw: 1, Normalized: &normalized},
				},
			}
		}

		thesis.Measurements.Store(types.SourceLeadLag, []*types.Measurement{
			reading("ETH/USD"),
		})

		before := (&Solver{}).extractFeatures(thesis)

		thesis.Measurements.Store(types.SourceLeadLag, []*types.Measurement{
			reading("SOL/USD"),
		})

		after := (&Solver{}).extractFeatures(thesis)

		Convey("Then the feature identity should survive the rotation", func() {
			/*
				The peer is the counterpart a reading was taken against, not
				part of what is being measured. If it entered the identity,
				every rotation of the cross-section anchor would add an input
				dimension and reset the network that learns from them.
			*/
			So(after, ShouldResemble, before)
			So(after, ShouldResemble, map[string]map[string]float64{
				"BTC/USD": {
					"leadlag:BTC/USD:direction": normalized,
				},
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
		thesis.Tickers.Store("BTC/USD", kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       decimal.NewFromFloat64(99),
			Ask:       decimal.NewFromFloat64(101),
			Timestamp: time.Unix(1, 0),
		})

		err := solver.Update(thesis)

		Convey("Then surprise and energy are reported per input dimension", func() {
			So(err, ShouldBeNil)
			rowRaw, found := thesis.Resonance.Load("BTC/USD")
			So(found, ShouldBeTrue)

			row, ok := rowRaw.(map[string]any)
			So(ok, ShouldBeTrue)

			state := solver.state("BTC/USD")
			featureCount := float64(len(state.featureSchema))
			surprise, surpriseOK := row["surprise"]
			energy, energyOK := row["energy"]

			So(surpriseOK, ShouldBeTrue)
			So(energyOK, ShouldBeTrue)

			/*
				Surprise is an L2 norm over the input dimensions, so it grows as
				the square root of the feature count and takes the square root as
				its divisor. Energy is a sum of squared residuals, which grows
				linearly, so it takes the count itself. Each divisor has to match
				the units of what it normalizes, or the reading still carries the
				size of the schema.
			*/
			So(surprise, ShouldAlmostEqual,
				state.manifold.ReconstructionError()/math.Sqrt(featureCount))

			/*
				PredictionEnergy rather than Energy. The latter adds the latent
				decay and sparsity penalties, whose magnitudes are set by the
				learning pace, so publishing it would make the reported energy
				move whenever the controller retuned alpha with no change in how
				well the network predicts.
			*/
			So(energy, ShouldAlmostEqual,
				state.manifold.PredictionEnergy()/featureCount)
		})

		Convey("Then the next market epoch produces a visible forward return curve", func() {
			thesis.Tickers.Store("BTC/USD", kraken.TickerData{
				Symbol:    "BTC/USD",
				Bid:       decimal.NewFromFloat64(100),
				Ask:       decimal.NewFromFloat64(102),
				Timestamp: time.Unix(2, 0),
			})

			err = solver.Update(thesis)
			rowRaw, found := thesis.Resonance.Load("BTC/USD")
			So(found, ShouldBeTrue)

			row, ok := rowRaw.(map[string]any)
			So(ok, ShouldBeTrue)

			curve, curveOK := row["forwardCurve"]
			horizon, horizonOK := row["activeHorizon"]
			expectedReturn, expectedReturnOK := row["expectedReturn"]
			returnReady, returnReadyOK := row["returnReady"]
			state := solver.state("BTC/USD")

			So(err, ShouldBeNil)
			So(curveOK, ShouldBeTrue)
			So(horizonOK, ShouldBeTrue)
			So(expectedReturnOK, ShouldBeTrue)
			So(returnReadyOK, ShouldBeTrue)
			So(curve, ShouldHaveLength, horizon.(int))
			So(returnReady, ShouldEqual, true)
			So(expectedReturn, ShouldEqual, curve.([]float64)[0])
			So(state.targetSamples, ShouldEqual, 1)
		})
	})
}

func TestUpdateKeepsIndependentSymbolStates(t *testing.T) {
	Convey("Given a pending resonance sample from one symbol", t, func() {
		normalized := 0.25
		solver := NewSolver(make(chan []byte, 1), nil)

		first := types.NewThesis()
		first.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				"reading": {Normalized: &normalized},
			},
		}})
		first.Tickers.Store("BTC/USD", kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       decimal.NewFromFloat64(99),
			Ask:       decimal.NewFromFloat64(101),
			Timestamp: time.Unix(1, 0),
		})

		So(solver.Update(first), ShouldBeNil)
		So(solver.state("BTC/USD").pendingAt.IsZero(), ShouldBeFalse)

		Convey("Then a later tick on a different target symbol must not train the same head sample", func() {
			second := types.NewThesis()
			second.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
				Source: types.SourceLiquidity,
				Symbol: "ETH/USD",
				Metrics: map[string]types.MetricSample{
					"reading": {Normalized: &normalized},
				},
			}})
			second.Tickers.Store("ETH/USD", kraken.TickerData{
				Symbol:    "ETH/USD",
				Bid:       decimal.NewFromFloat64(199),
				Ask:       decimal.NewFromFloat64(201),
				Timestamp: time.Unix(2, 0),
			})

			So(solver.Update(second), ShouldBeNil)
			So(solver.state("BTC/USD").targetSamples, ShouldEqual, 0)
			So(solver.state("ETH/USD").targetSamples, ShouldEqual, 0)
			So(solver.state("ETH/USD").pendingAt.IsZero(), ShouldBeFalse)

			rowRaw, found := second.Resonance.Load("ETH/USD")
			So(found, ShouldBeTrue)

			row, ok := rowRaw.(map[string]any)
			So(ok, ShouldBeTrue)
			So(row["targetSymbol"], ShouldEqual, "ETH/USD")
		})
	})
}

/*
BenchmarkUpdate measures the two-symbol resonance pass at the feature width
observed in the full tick profile. It exposes whether per-symbol fan-out helps
or merely competes with Gonum's own parallel matrix work.
*/
func BenchmarkUpdate(b *testing.B) {
	const featureCount = 48

	thesis := types.NewThesis()
	solver := NewSolver(nil, nil)
	symbols := []string{"SIM1/USD", "SIM2/USD"}

	for symbolIndex, symbol := range symbols {
		metrics := make(map[string]types.MetricSample, featureCount)

		for featureIndex := range featureCount {
			normalized := float64(featureIndex+symbolIndex) / featureCount
			metrics[fmt.Sprintf("feature_%d", featureIndex)] = types.MetricSample{
				Normalized: &normalized,
			}
		}

		thesis.Measurements.Store(types.SourceLiquidity, append(
			utils.Measurements(thesis, types.SourceLiquidity),
			&types.Measurement{
				Source:  types.SourceLiquidity,
				Symbol:  symbol,
				Metrics: metrics,
			},
		))
	}

	epoch := int64(1)
	b.ResetTimer()

	for b.Loop() {
		for _, symbol := range symbols {
			thesis.Tickers.Store(symbol, kraken.TickerData{
				Symbol:    symbol,
				Bid:       decimal.NewFromFloat64(99),
				Ask:       decimal.NewFromFloat64(101),
				Timestamp: time.Unix(epoch, 0),
			})
		}

		if err := solver.Update(thesis); err != nil {
			b.Fatal(err)
		}

		epoch++
	}
}
