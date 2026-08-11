package category

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestUpdate(t *testing.T) {
	Convey("Given completed signals with different measurement sets per symbol", t, func() {
		thesis := categoryThesis(t)
		ignition := 0.9
		drive := 0.8
		bitcoin := types.NewSymbol("BTC/USD", nil)
		bitcoin.AddMeasurement(&types.Measurement{
			Source:   types.SourcePumpDump,
			Symbol:   "BTC/USD",
			Maturity: 0.75,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricIgnition, types.SideNone): {
					Normalized: &ignition,
				},
			},
		})
		ethereum := types.NewSymbol("ETH/USD", nil)
		ethereum.AddMeasurement(&types.Measurement{
			Source:   types.SourceCVD,
			Symbol:   "ETH/USD",
			Maturity: 0.5,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricDrive, types.SideNone): {
					Normalized: &drive,
				},
			},
		})
		thesis.Symbols.Store("BTC/USD", bitcoin)
		thesis.Symbols.Store("ETH/USD", ethereum)
		stampCategorySignals(thesis, "BTC/USD")
		stampCategorySignals(thesis, "ETH/USD")
		solver := NewSolver(nil, nil, nil)

		err := solver.Update(thesis)

		Convey("It should classify every symbol from the evidence it actually has", func() {
			So(err, ShouldBeNil)
			So(thesis.Stamped("BTC/USD", types.SourceCategory), ShouldBeTrue)
			So(thesis.Stamped("ETH/USD", types.SourceCategory), ShouldBeTrue)
			So(categoryAt(thesis, "BTC/USD"), ShouldResemble, types.Category{
				Symbol:     "BTC/USD",
				Type:       types.VerticalIgnition,
				Confidence: categoryAt(thesis, "BTC/USD").Confidence,
				Strength:   ignition,
				Maturity:   0.75,
				Supporting: []string{"pumpdump:ignition"},
			})
			So(categoryAt(thesis, "ETH/USD").Type, ShouldEqual, types.AggressiveDrive)
			So(categoryAt(thesis, "ETH/USD").Strength, ShouldEqual, drive)
		})
	})

	Convey("Given a symbol whose configured scores are not usable yet", t, func() {
		thesis := categoryThesis(t)
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.AddMeasurement(&types.Measurement{
			Source: types.SourcePumpDump,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricIgnition, types.SideNone): {Raw: 1},
			},
		})
		thesis.Symbols.Store("BTC/USD", symbol)
		stampCategorySignals(thesis, "BTC/USD")

		err := NewSolver(nil, nil, nil).Update(thesis)

		Convey("It should still emit an explicitly weak category artifact", func() {
			category := categoryAt(thesis, "BTC/USD")

			So(err, ShouldBeNil)
			So(category.Type, ShouldEqual, types.CategoryTypeNone)
			So(category.Strength, ShouldEqual, 0)
			So(category.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given directional evidence for one category", t, func() {
		thesis := categoryThesis(t)
		buy := 0.81
		sell := 0.49
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.AddMeasurement(&types.Measurement{
			Source:   types.SourceExhaustion,
			Symbol:   "BTC/USD",
			Maturity: 0.6,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricMechanical, types.SideBuy): {
					Normalized: &buy,
				},
				types.MetricKey(types.MetricMechanical, types.SideSell): {
					Normalized: &sell,
				},
			},
		})
		thesis.Symbols.Store("BTC/USD", symbol)
		stampCategorySignals(thesis, "BTC/USD")

		err := NewSolver(nil, nil, nil).Update(thesis)

		Convey("It should delegate evidence combination to nomagique", func() {
			category := categoryAt(thesis, "BTC/USD")

			So(err, ShouldBeNil)
			So(category.Type, ShouldEqual, types.MechanicalCollapse)
			So(category.Strength, ShouldAlmostEqual, 0.63)
			So(category.Supporting, ShouldHaveLength, 2)
		})
	})

	Convey("Given incomplete signal production", t, func() {
		thesis := types.NewThesis(t.Context(), nil)

		err := NewSolver(nil, nil, nil).Update(thesis)

		Convey("It should leave category readiness open", func() {
			So(err, ShouldBeNil)
			So(thesis.Stamped("BTC/USD", types.SourceCategory), ShouldBeFalse)
		})
	})

	Convey("Given retained lead-lag evidence from different anchor epochs", t, func() {
		thesis := categoryThesis(t)
		oldInefficient := 1.0
		currentSync := 0.8
		symbol := types.NewSymbol("ALT/USD", nil)
		symbol.Measurements = append(symbol.Measurements,
			&types.Measurement{
				Source: types.SourceLeadLag,
				Symbol: "ALT/USD",
				Peer:   "UNFI/USD",
				At:     time.Unix(1, 0),
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricInefficient, types.SideNone): {
						Normalized: &oldInefficient,
					},
				},
			},
			&types.Measurement{
				Source: types.SourceLeadLag,
				Symbol: "ALT/USD",
				Peer:   "SOSO/USD",
				At:     time.Unix(2, 0),
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricSync, types.SideNone): {
						Normalized: &currentSync,
					},
				},
			},
		)
		thesis.Symbols.Store("ALT/USD", symbol)
		stampCategorySignals(thesis, "ALT/USD")

		err := NewSolver(nil, nil, nil).Update(thesis)

		Convey("It classifies only the newest source epoch", func() {
			category := categoryAt(thesis, "ALT/USD")

			So(err, ShouldBeNil)
			So(category.Type, ShouldEqual, types.SynchronizedDrift)
			So(category.Strength, ShouldEqual, currentSync)
			So(category.Supporting, ShouldResemble, []string{"leadlag:sync"})
		})
	})
}

func categoryThesis(t *testing.T) *types.Thesis {
	return types.NewThesis(t.Context(), nil)
}

func stampCategorySignals(thesis *types.Thesis, symbol string) {
	value, found := thesis.Symbols.Load(symbol)

	if !found || value == nil {
		return
	}

	state := value.(*types.Symbol)
	for _, source := range []types.SourceType{
		types.SourceCorrelation,
		types.SourceCVD,
		types.SourceDepthFlow,
		types.SourceExhaustion,
		types.SourceHawkes,
		types.SourceLeadLag,
		types.SourceLiquidity,
		types.SourcePumpDump,
		types.SourceSentiment,
		types.SourceToxicity,
	} {
		state.Stamp(source)
	}
}

func categoryAt(thesis *types.Thesis, symbol string) types.Category {
	value, found := thesis.Symbols.Load(symbol)

	if !found || value == nil {
		return types.Category{}
	}

	symbolState := value.(*types.Symbol)
	categoriesValue, found := symbolState.Categories.Load(symbol)

	if !found {
		return types.Category{}
	}

	return categoriesValue.([]types.Category)[0]
}

func BenchmarkUpdate(b *testing.B) {
	solver := NewSolver(nil, nil, nil)
	strength := 0.9

	for b.Loop() {
		thesis := types.NewThesis(b.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.AddMeasurement(&types.Measurement{
			Source: types.SourcePumpDump,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricIgnition, types.SideNone): {
					Normalized: &strength,
				},
			},
		})
		thesis.Symbols.Store("BTC/USD", symbol)
		stampCategorySignals(thesis, "BTC/USD")

		if err := solver.Update(thesis); err != nil {
			b.Fatal(err)
		}
	}
}
