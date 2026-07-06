package trader

import (
	"strings"
	"testing"
	"time"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCortexMeasure(testingTB *testing.T) {
	Convey("Given live measurement and decision-stage frames", testingTB, func() {
		tree := dmt.NewTree("")
		cortex := newCortex(tree)
		at := time.Date(2026, 7, 6, 10, 15, 0, 0, time.UTC)

		Convey("When the cortex measures the market sequence", func() {
			readings, err := cortex.Measure(
				cortexMeasurements("BTC/USD", at),
				cortexBatch("BTC/USD"),
			)

			Convey("Then it should train DMT and return a frontend-ready reading", func() {
				So(err, ShouldBeNil)
				So(readings, ShouldContainKey, "BTC/USD")

				reading := readings["BTC/USD"]

				So(reading.Scope, ShouldEqual, "BTC/USD")
				So(reading.Sequence, ShouldContainSubstring, "fluid-laminar")
				So(reading.WinnerClass, ShouldEqual, "endogenous-alpha")
				So(reading.ClassConfidence, ShouldBeGreaterThan, 0)
				So(reading.Branches, ShouldNotBeEmpty)
				So(reading.Beams, ShouldNotBeEmpty)
				So(reading.Classes, ShouldNotBeEmpty)
				So(reading.NodeCount, ShouldEqual, len(reading.Branches))
				So(reading.Branches[0].Token, ShouldEqual, "root")
				So(reading.Branches[0].Prefix, ShouldEqual, "")

				observations := cortex.observations(
					cortexMeasurements("BTC/USD", at),
					cortexBatch("BTC/USD"),
				)
				sequence, err := cortex.topology.Sequence(observations["BTC/USD"])

				So(err, ShouldBeNil)

				treeSequence := strings.Join(
					append([]string{sequence.Symbol}, sequence.Tree...),
					"_",
				)
				scoped := tree.GetSensoryWeight([]byte(treeSequence))
				readableScoped := tree.GetSensoryWeight([]byte("btc-usd_" + reading.Sequence))
				unscoped := tree.GetSensoryWeight([]byte(reading.Sequence))

				So(scoped.Count, ShouldBeGreaterThan, 0)
				So(readableScoped.Count, ShouldEqual, uint64(0))
				So(unscoped.Count, ShouldEqual, uint64(0))
			})
		})
	})
}

func BenchmarkCortexMeasure(benchmarkTB *testing.B) {
	tree := dmt.NewTree("")
	cortex := newCortex(tree)
	at := time.Date(2026, 7, 6, 10, 15, 0, 0, time.UTC)
	measurements := cortexMeasurements("BTC/USD", at)
	batch := cortexBatch("BTC/USD")

	benchmarkTB.ReportAllocs()

	for benchmarkTB.Loop() {
		if _, err := cortex.Measure(measurements, batch); err != nil {
			benchmarkTB.Fatal(err)
		}
	}
}

func cortexMeasurements(symbol string, at time.Time) []*types.Measurement {
	return []*types.Measurement{
		{
			Source: types.SourceFluid,
			Symbol: symbol,
			At:     at,
			Categories: []types.Category{
				{
					Type:       types.CategoryLaminar,
					Confidence: 0.72,
					Strength:   0.66,
				},
			},
		},
		{
			Source: types.SourceHawkes,
			Symbol: symbol,
			At:     at,
			Categories: []types.Category{
				{
					Type:       types.CategoryFrenzy,
					Confidence: 0.42,
					Strength:   0.36,
				},
			},
		},
	}
}

func cortexBatch(symbol string) logic.Batch {
	return logic.Batch{
		Manifold: []*logic.ManifoldFrame{
			{
				Symbol:   symbol,
				Category: types.CategoryLaminarResonance,
				Strength: 0.61,
			},
		},
		Resonance: []*logic.ResonanceFrame{
			{
				Symbol:     symbol,
				Category:   types.CategoryLaminarResonance,
				Confidence: 0.63,
			},
		},
		Causal: []*logic.CausalFrame{
			{
				Symbol:     symbol,
				Category:   types.CategoryEndogenousAlpha,
				Confidence: 0.71,
			},
		},
	}
}
