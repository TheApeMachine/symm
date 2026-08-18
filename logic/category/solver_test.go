package category

import (
	"math"
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
		bitcoin.AppendMeasurement(&types.Measurement{
			Source:   types.SourcePumpDump,
			Symbol:   "BTC/USD",
			Maturity: 0.75,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricRVOL, types.SideNone): {
					Normalized: &ignition,
				},
			},
		})
		ethereum := types.NewSymbol("ETH/USD", nil)
		ethereum.AppendMeasurement(&types.Measurement{
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
			bitcoinCategory := categoryAt(thesis, "BTC/USD")
			So(err, ShouldBeNil)
			So(bitcoinCategory, ShouldResemble, types.Category{
				At:         bitcoinCategory.At,
				Symbol:     "BTC/USD",
				Type:       types.VerticalIgnition,
				Confidence: bitcoinCategory.Confidence,
				Surprisal:  -math.Log2(bitcoinCategory.Confidence),
				Strength:   ignition,
				Maturity:   0.75,
				Supporting: []string{"pumpdump:rvol"},
			})
			So(categoryAt(thesis, "ETH/USD").Type, ShouldEqual, types.AggressiveDrive)
			So(categoryAt(thesis, "ETH/USD").Strength, ShouldEqual, drive)
		})
	})

	Convey("Given a symbol whose configured scores are not usable yet", t, func() {
		thesis := categoryThesis(t)
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.AppendMeasurement(&types.Measurement{
			Source: types.SourcePumpDump,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricRVOL, types.SideNone): {Raw: 1},
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
			So(category.Surprisal, ShouldAlmostEqual, -math.Log2(category.Confidence))
		})
	})

	Convey("Given directional evidence for one category", t, func() {
		thesis := categoryThesis(t)
		buy := 0.81
		sell := 0.49
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.AppendMeasurement(&types.Measurement{
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
			So(category.Surprisal, ShouldBeGreaterThan, 0)
			So(category.Supporting, ShouldHaveLength, 2)
		})
	})

	Convey("Given positive evidence for competing categories", t, func() {
		thesis := categoryThesis(t)
		ignition := 0.9
		drive := 0.4
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.AppendMeasurement(&types.Measurement{
			Source: types.SourcePumpDump,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricRVOL, types.SideNone): {
					Normalized: &ignition,
				},
			},
		})
		symbol.AppendMeasurement(&types.Measurement{
			Source: types.SourceCVD,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricDrive, types.SideNone): {
					Normalized: &drive,
				},
			},
		})
		thesis.Symbols.Store("BTC/USD", symbol)
		stampCategorySignals(thesis, "BTC/USD")

		err := NewSolver(nil, nil, nil).Update(thesis)

		Convey("It should derive every surprisal from its reported confidence", func() {
			stored, found := symbol.Categories.Load("BTC/USD")
			categories := stored.([]types.Category)

			So(err, ShouldBeNil)
			So(found, ShouldBeTrue)
			So(categories, ShouldHaveLength, 2)

			for _, category := range categories {
				So(category.Surprisal, ShouldAlmostEqual, -math.Log2(category.Confidence))
			}

			So(categories[0].Confidence, ShouldBeGreaterThan, categories[1].Confidence)
		})
	})

	Convey("Given the full corroborated ignition complex", t, func() {
		thesis := categoryThesis(t)
		ignition := 0.8
		spectralRadius := 0.9
		thin := 0.7
		drive := 0.6
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.AppendMeasurement(&types.Measurement{
			Source: types.SourcePumpDump,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricRVOL, types.SideNone): {
					Normalized: &ignition,
				},
			},
		})
		symbol.AppendMeasurement(&types.Measurement{
			Source: types.SourceHawkes,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricSpectralRadius, types.SideNone): {
					Normalized: &spectralRadius,
				},
			},
		})
		symbol.AppendMeasurement(&types.Measurement{
			Source: types.SourceDepthFlow,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricThinScore, types.SideNone): {
					Normalized: &thin,
				},
			},
		})
		symbol.AppendMeasurement(&types.Measurement{
			Source: types.SourceCVD,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricDrive, types.SideNone): {
					Normalized: &drive,
				},
			},
		})
		thesis.Symbols.Store("BTC/USD", symbol)
		stampCategorySignals(thesis, "BTC/USD")

		err := NewSolver(nil, nil, nil).Update(thesis)

		Convey("It should carry the four-leg conjunction as one evidence mass", func() {
			stored, found := symbol.Categories.Load("BTC/USD")
			So(err, ShouldBeNil)
			So(found, ShouldBeTrue)

			var composite *types.Category

			for index, category := range stored.([]types.Category) {
				if category.Type == types.VerticalIgnition {
					composite = &stored.([]types.Category)[index]
					break
				}
			}

			So(composite, ShouldNotBeNil)
			So(composite.Strength, ShouldAlmostEqual, 0.741559, 1e-5)
			So(composite.Supporting, ShouldHaveLength, 4)
		})
	})

	Convey("Given a weak corroborating leg beside a strong ignition", t, func() {
		thesis := categoryThesis(t)
		ignition := 0.9
		drive := 0.2
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.AppendMeasurement(&types.Measurement{
			Source: types.SourcePumpDump,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricRVOL, types.SideNone): {
					Normalized: &ignition,
				},
			},
		})
		symbol.AppendMeasurement(&types.Measurement{
			Source: types.SourceCVD,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricDrive, types.SideNone): {
					Normalized: &drive,
				},
			},
		})
		thesis.Symbols.Store("BTC/USD", symbol)
		stampCategorySignals(thesis, "BTC/USD")

		err := NewSolver(nil, nil, nil).Update(thesis)

		Convey("It should drag the composite below its strongest leg", func() {
			stored, found := symbol.Categories.Load("BTC/USD")
			So(err, ShouldBeNil)
			So(found, ShouldBeTrue)

			var composite *types.Category

			for index, category := range stored.([]types.Category) {
				if category.Type == types.VerticalIgnition {
					composite = &stored.([]types.Category)[index]
					break
				}
			}

			So(composite, ShouldNotBeNil)
			So(composite.Strength, ShouldAlmostEqual, math.Sqrt(0.18), 1e-9)
			So(composite.Strength, ShouldBeLessThan, ignition)
		})
	})

	Convey("Given a fitted Hawkes spectral radius", t, func() {
		thesis := categoryThesis(t)
		spectralRadius := 0.73
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.AppendMeasurement(&types.Measurement{
			Source:   types.SourceHawkes,
			Symbol:   "BTC/USD",
			Maturity: 0.9,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricSpectralRadius, types.SideNone): {
					Normalized: &spectralRadius,
				},
			},
		})
		thesis.Symbols.Store("BTC/USD", symbol)

		err := NewSolver(nil, nil, nil).Update(thesis)

		Convey("It should classify the volatility radar axis as turbulent", func() {
			category := categoryAt(thesis, "BTC/USD")

			So(err, ShouldBeNil)
			So(category.Type, ShouldEqual, types.CategoryTurbulent)
			So(category.Strength, ShouldEqual, spectralRadius)
		})
	})

	Convey("Given incomplete signal production", t, func() {
		thesis := types.NewThesis(t.Context(), nil)

		err := NewSolver(nil, nil, nil).Update(thesis)

		Convey("It should leave category readiness open", func() {
			So(err, ShouldBeNil)
		})
	})

	Convey("Given a streamed symbol without new evidence in this update", t, func() {
		thesis := categoryThesis(t)
		symbol := thesis.Symbol("BTC/USD")
		retained := []types.Category{{
			Symbol: "BTC/USD",
			Type:   types.VerticalIgnition,
		}}
		symbol.Categories.Store("BTC/USD", retained)

		err := NewSolver(nil, nil, nil).Update(thesis)

		Convey("It should preserve the last classified artifact", func() {
			stored, found := symbol.Categories.Load("BTC/USD")

			So(err, ShouldBeNil)
			So(found, ShouldBeTrue)
			So(stored, ShouldResemble, retained)
		})
	})

	Convey("Given retained lead-lag evidence from different anchor epochs", t, func() {
		thesis := categoryThesis(t)
		oldInefficient := 1.0
		currentSync := 0.8
		symbol := types.NewSymbol("ALT/USD", nil)
		symbol.AppendMeasurement(&types.Measurement{
			Source: types.SourceLeadLag,
			Symbol: "ALT/USD",
			Peer:   "UNFI/USD",
			At:     time.Unix(1, 0),
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricInefficient, types.SideNone): {
					Normalized: &oldInefficient,
				},
			},
		})
		symbol.AppendMeasurement(&types.Measurement{
			Source: types.SourceLeadLag,
			Symbol: "ALT/USD",
			Peer:   "SOSO/USD",
			At:     time.Unix(2, 0),
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricSync, types.SideNone): {
					Normalized: &currentSync,
				},
			},
		})
		thesis.Symbols.Store("ALT/USD", symbol)
		stampCategorySignals(thesis, "ALT/USD")

		err := NewSolver(nil, nil, nil).Update(thesis)

		Convey("It classifies the strongest retained lead-lag evidence", func() {
			category := categoryAt(thesis, "ALT/USD")

			So(err, ShouldBeNil)
			So(category.Type, ShouldEqual, types.InefficientLag)
			So(category.Strength, ShouldEqual, oldInefficient)
			So(category.Supporting, ShouldResemble, []string{"leadlag:inefficient"})
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
		symbol.AppendMeasurement(&types.Measurement{
			Source: types.SourcePumpDump,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricRVOL, types.SideNone): {
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
