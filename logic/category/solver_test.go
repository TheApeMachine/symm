package category

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
lastCategories drains the category output column, polling until the self-run
solver goroutine has produced a batch or the deadline passes.
*/
func lastCategories(symbol *types.Symbol) []types.Category {
	var categories []types.Category

	deadline := time.Now().Add(3 * time.Second)

	for len(categories) == 0 && time.Now().Before(deadline) {
		categories = nil

		for batch := range symbol.MarketCategories(types.SourceGraph) {
			categories = append(categories, batch...)
		}

		if len(categories) == 0 {
			time.Sleep(time.Millisecond)
		}
	}

	return categories
}

func categoryMeasurement(
	source types.SourceType,
	symbol string,
	maturity float64,
) *nmtypes.Measurement {
	measurement := nmtypes.NewMeasurement("", string(source), 0, 0)
	measurement.Symbol = symbol
	measurement.Maturity = maturity

	return measurement
}

func normalizedMetric(
	measurement *nmtypes.Measurement,
	metric types.MetricType,
	side types.MeasurementSide,
	value float64,
) {
	normalized := value
	measurement.Metrics[types.MetricKey(metric, side)] = &nmtypes.Metric[float64]{
		Name:       "",
		Raw:        value,
		Normalized: &normalized,
		Unit:       nmtypes.UnitDimensionless,
	}
}

func rawMetric(
	measurement *nmtypes.Measurement,
	metric types.MetricType,
	side types.MeasurementSide,
	value float64,
) {
	measurement.Metrics[types.MetricKey(metric, side)] = &nmtypes.Metric[float64]{
		Name: "",
		Raw:  value,
		Unit: nmtypes.UnitDimensionless,
	}
}

func TestUpdate(t *testing.T) {
	Convey("Given completed signals with different measurement sets per symbol", t, func() {
		thesis := categoryThesis(t)
		ignition := 0.9
		drive := 0.8
		bitcoin := types.NewSymbol("BTC/USD")
		bitcoinMeasurement := categoryMeasurement(types.SourcePumpDump, "BTC/USD", 0.75)
		normalizedMetric(bitcoinMeasurement, types.MetricRVOL, types.SideNone, ignition)
		bitcoin.AppendMeasurement(bitcoinMeasurement)
		ethereum := types.NewSymbol("ETH/USD")
		ethereumMeasurement := categoryMeasurement(types.SourceCVD, "ETH/USD", 0.5)
		normalizedMetric(ethereumMeasurement, types.MetricDrive, types.SideNone, drive)
		ethereum.AppendMeasurement(ethereumMeasurement)
		thesis.Symbols.Store("BTC/USD", bitcoin)
		thesis.Symbols.Store("ETH/USD", ethereum)
		stampCategorySignals(thesis, "BTC/USD")
		stampCategorySignals(thesis, "ETH/USD")
		NewSolver(t.Context(), thesis, nil, nil, nil)

		Convey("It should classify every symbol from the evidence it actually has", func() {
			bitcoinCategory := categoryAt(thesis, "BTC/USD")
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
			ethereumCategory := categoryAt(thesis, "ETH/USD")
			So(ethereumCategory.Type, ShouldEqual, types.AggressiveDrive)
			So(ethereumCategory.Strength, ShouldEqual, drive)
		})
	})

	Convey("Given a symbol whose configured scores are not usable yet", t, func() {
		thesis := categoryThesis(t)
		symbol := types.NewSymbol("BTC/USD")
		weak := categoryMeasurement(types.SourcePumpDump, "BTC/USD", 0)
		rawMetric(weak, types.MetricRVOL, types.SideNone, 1)
		symbol.AppendMeasurement(weak)
		thesis.Symbols.Store("BTC/USD", symbol)
		stampCategorySignals(thesis, "BTC/USD")

		NewSolver(t.Context(), thesis, nil, nil, nil)

		Convey("It should still emit an explicitly weak category artifact", func() {
			category := categoryAt(thesis, "BTC/USD")

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
		symbol := types.NewSymbol("BTC/USD")
		directional := categoryMeasurement(types.SourceExhaustion, "BTC/USD", 0.6)
		normalizedMetric(directional, types.MetricMechanical, types.SideBuy, buy)
		normalizedMetric(directional, types.MetricMechanical, types.SideSell, sell)
		symbol.AppendMeasurement(directional)
		thesis.Symbols.Store("BTC/USD", symbol)
		stampCategorySignals(thesis, "BTC/USD")

		NewSolver(t.Context(), thesis, nil, nil, nil)

		Convey("It should delegate evidence combination to nomagique", func() {
			category := categoryAt(thesis, "BTC/USD")

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
		symbol := types.NewSymbol("BTC/USD")
		competingIgnition := categoryMeasurement(types.SourcePumpDump, "BTC/USD", 0)
		normalizedMetric(competingIgnition, types.MetricRVOL, types.SideNone, ignition)
		symbol.AppendMeasurement(competingIgnition)
		competingDrive := categoryMeasurement(types.SourceCVD, "BTC/USD", 0)
		normalizedMetric(competingDrive, types.MetricDrive, types.SideNone, drive)
		symbol.AppendMeasurement(competingDrive)
		thesis.Symbols.Store("BTC/USD", symbol)
		stampCategorySignals(thesis, "BTC/USD")

		NewSolver(t.Context(), thesis, nil, nil, nil)

		Convey("It should derive every surprisal from its reported confidence", func() {
			categories := lastCategories(symbol)

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
		symbol := types.NewSymbol("BTC/USD")
		c1 := categoryMeasurement(types.SourcePumpDump, "BTC/USD", 0)
		normalizedMetric(c1, types.MetricRVOL, types.SideNone, ignition)
		symbol.AppendMeasurement(c1)
		c2 := categoryMeasurement(types.SourceHawkes, "BTC/USD", 0)
		normalizedMetric(c2, types.MetricSpectralRadius, types.SideNone, spectralRadius)
		symbol.AppendMeasurement(c2)
		c3 := categoryMeasurement(types.SourceDepthFlow, "BTC/USD", 0)
		normalizedMetric(c3, types.MetricThinScore, types.SideNone, thin)
		symbol.AppendMeasurement(c3)
		c4 := categoryMeasurement(types.SourceCVD, "BTC/USD", 0)
		normalizedMetric(c4, types.MetricDrive, types.SideNone, drive)
		symbol.AppendMeasurement(c4)
		thesis.Symbols.Store("BTC/USD", symbol)
		stampCategorySignals(thesis, "BTC/USD")

		NewSolver(t.Context(), thesis, nil, nil, nil)

		Convey("It should carry the four-leg conjunction as one evidence mass", func() {
			categories := lastCategories(symbol)
			So(len(categories), ShouldBeGreaterThan, 0)

			var composite *types.Category

			for index := range categories {
				if categories[index].Type == types.VerticalIgnition {
					composite = &categories[index]
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
		symbol := types.NewSymbol("BTC/USD")
		w1 := categoryMeasurement(types.SourcePumpDump, "BTC/USD", 0)
		normalizedMetric(w1, types.MetricRVOL, types.SideNone, ignition)
		symbol.AppendMeasurement(w1)
		w2 := categoryMeasurement(types.SourceCVD, "BTC/USD", 0)
		normalizedMetric(w2, types.MetricDrive, types.SideNone, drive)
		symbol.AppendMeasurement(w2)
		thesis.Symbols.Store("BTC/USD", symbol)
		stampCategorySignals(thesis, "BTC/USD")

		NewSolver(t.Context(), thesis, nil, nil, nil)

		Convey("It should drag the composite below its strongest leg", func() {
			categories := lastCategories(symbol)
			So(len(categories), ShouldBeGreaterThan, 0)

			var composite *types.Category

			for index := range categories {
				if categories[index].Type == types.VerticalIgnition {
					composite = &categories[index]
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
		symbol := types.NewSymbol("BTC/USD")
		sr := categoryMeasurement(types.SourceHawkes, "BTC/USD", 0.9)
		normalizedMetric(sr, types.MetricSpectralRadius, types.SideNone, spectralRadius)
		symbol.AppendMeasurement(sr)
		thesis.Symbols.Store("BTC/USD", symbol)

		NewSolver(t.Context(), thesis, nil, nil, nil)

		Convey("It should classify the volatility radar axis as turbulent", func() {
			category := categoryAt(thesis, "BTC/USD")

			So(category.Type, ShouldEqual, types.CategoryTurbulent)
			So(category.Strength, ShouldEqual, spectralRadius)
		})
	})

	Convey("Given incomplete signal production", t, func() {
		thesis := types.NewThesis(t.Context(), nil)

		NewSolver(t.Context(), thesis, nil, nil, nil)

		Convey("It should leave category readiness open with no artifacts", func() {
			thesis.Symbols.Range(func(_, value any) bool {
				symbol, _ := value.(*types.Symbol)

				if symbol != nil {
					So(len(lastCategories(symbol)), ShouldEqual, 0)
				}

				return true
			})
		})
	})

	Convey("Given a streamed symbol without new evidence in this update", t, func() {
		thesis := categoryThesis(t)
		symbol := thesis.Symbol("BTC/USD")
		retained := []types.Category{{
			Symbol: "BTC/USD",
			Type:   types.VerticalIgnition,
		}}
		symbol.Categories.Push(retained)

		NewSolver(t.Context(), thesis, nil, nil, nil)

		Convey("It should preserve the last classified artifact", func() {
			categories := lastCategories(symbol)

			So(categories, ShouldResemble, retained)
		})
	})

	Convey("Given retained lead-lag evidence from different anchor epochs", t, func() {
		thesis := categoryThesis(t)
		oldInefficient := 1.0
		currentSync := 0.8
		symbol := types.NewSymbol("ALT/USD")
		oldLead := categoryMeasurement(types.SourceLeadLag, "ALT/USD", 0)
		oldLead.Peer = "UNFI/USD"
		oldLead.At = time.Unix(1, 0)
		normalizedMetric(oldLead, types.MetricInefficient, types.SideNone, oldInefficient)
		symbol.AppendMeasurement(oldLead)
		currentLead := categoryMeasurement(types.SourceLeadLag, "ALT/USD", 0)
		currentLead.Peer = "SOSO/USD"
		currentLead.At = time.Unix(2, 0)
		normalizedMetric(currentLead, types.MetricSync, types.SideNone, currentSync)
		symbol.AppendMeasurement(currentLead)
		thesis.Symbols.Store("ALT/USD", symbol)
		stampCategorySignals(thesis, "ALT/USD")

		NewSolver(t.Context(), thesis, nil, nil, nil)

		Convey("It classifies the strongest retained lead-lag evidence", func() {
			category := categoryAt(thesis, "ALT/USD")

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
	categories := lastCategories(symbolState)

	if len(categories) == 0 {
		return types.Category{}
	}

	return categories[0]
}

func BenchmarkCategoryRun(b *testing.B) {
	thesis := types.NewThesis(b.Context(), nil)
	solver := NewSolver(b.Context(), thesis, nil, nil, nil)
	defer solver.Close()
	strength := 0.9

	for b.Loop() {
		symbol := types.NewSymbol("BTC/USD")
		bench := categoryMeasurement(types.SourcePumpDump, "BTC/USD", 0)
		normalizedMetric(bench, types.MetricRVOL, types.SideNone, strength)
		symbol.AppendMeasurement(bench)
		thesis.Symbols.Store("BTC/USD", symbol)
		stampCategorySignals(thesis, "BTC/USD")
		b.StopTimer()
		lastCategories(symbol)
		b.StartTimer()
		thesis.Symbols.Delete("BTC/USD")
	}
}
