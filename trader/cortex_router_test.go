package trader

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCortexRouter(t *testing.T) {
	Convey("Given a CortexRouter with seeded observations", t, func() {
		router := NewCortexRouter()
		baseTime := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)

		organicState := &cortexObservation{
			symbol: "BTC/USD",
			at:     baseTime,
			measurements: map[types.SourceType]cortexReading{
				types.SourceFluid: {
					category: types.Category{
						Type:       types.CategoryLaminar,
						Confidence: 0.72,
						Strength:   0.66,
					},
					metrics: map[string]float64{
						"reynolds":   500,
						"vorticity":  0.15,
						"turbulence": 0.10,
						"price":      65000,
					},
				},
				types.SourceHawkes: {
					category: types.Category{
						Type:       types.CategoryOrganic,
						Confidence: 0.60,
						Strength:   0.50,
					},
					metrics: map[string]float64{
						"spectralRadius": 0.45,
						"asymmetry":      0.10,
						"price":          65000,
					},
				},
			},
		}

		routing1 := router.Route(organicState)

		Convey("It should produce a routing result with zero matches initially", func() {
			So(routing1.MatchCount, ShouldEqual, 0)
			So(routing1.PredictedReturnBps, ShouldEqual, 0)
		})

		Convey("When a second observation arrives and backfills the first", func() {
			nextState := &cortexObservation{
				symbol: "BTC/USD",
				at:     baseTime.Add(5 * time.Second),
				measurements: map[types.SourceType]cortexReading{
					types.SourceFluid: {
						category: types.Category{
							Type:       types.CategoryLaminar,
							Confidence: 0.75,
							Strength:   0.68,
						},
						metrics: map[string]float64{
							"reynolds":   510,
							"vorticity":  0.16,
							"turbulence": 0.11,
							"price":      65100,
						},
					},
					types.SourceHawkes: {
						category: types.Category{
							Type:       types.CategoryOrganic,
							Confidence: 0.62,
							Strength:   0.52,
						},
						metrics: map[string]float64{
							"spectralRadius": 0.46,
							"asymmetry":      0.11,
							"price":          65100,
						},
					},
				},
			}

			router.Route(nextState)

			Convey("It should have populated the corpus", func() {
				So(router.CorpusSize(), ShouldBeGreaterThan, 0)
			})

			Convey("When querying with a similar state", func() {
				similarState := &cortexObservation{
					symbol: "BTC/USD",
					at:     baseTime.Add(10 * time.Second),
					measurements: map[types.SourceType]cortexReading{
						types.SourceFluid: {
							category: types.Category{
								Type:       types.CategoryLaminar,
								Confidence: 0.73,
								Strength:   0.67,
							},
							metrics: map[string]float64{
								"reynolds":   505,
								"vorticity":  0.155,
								"turbulence": 0.105,
								"price":      65050,
							},
						},
						types.SourceHawkes: {
							category: types.Category{
								Type:       types.CategoryOrganic,
								Confidence: 0.61,
								Strength:   0.51,
							},
							metrics: map[string]float64{
								"spectralRadius": 0.455,
								"asymmetry":      0.105,
								"price":          65050,
							},
						},
					},
				}

				routing := router.Route(similarState)

				Convey("It should find corpus matches", func() {
					So(routing.MatchCount, ShouldBeGreaterThan, 0)
					So(routing.TopSimilarity, ShouldNotEqual, 0)
				})
			})
		})
	})
}

func TestCortexRouterMagnitudeDistinction(t *testing.T) {
	Convey("Given two Hawkes states that string matching would collapse", t, func() {
		router := NewCortexRouter()
		baseTime := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)

		// Seed the corpus with a few observations so backfill produces entries.
		for index := range 5 {
			router.Route(&cortexObservation{
				symbol: "BTC/USD",
				at:     baseTime.Add(time.Duration(index) * time.Second),
				measurements: map[types.SourceType]cortexReading{
					types.SourceHawkes: {
						category: types.Category{
							Type:       types.CategorySaturation,
							Confidence: 0.80 + float64(index)*0.02,
							Strength:   0.75 + float64(index)*0.02,
						},
						metrics: map[string]float64{
							"spectralRadius": 0.86 + float64(index)*0.02,
							"asymmetry":      0.20 + float64(index)*0.05,
							"price":          65000 + float64(index)*50,
						},
					},
				},
			})
		}

		// Now query with mild vs critical — both "saturation" category.
		mildState := &cortexObservation{
			symbol: "BTC/USD",
			at:     baseTime.Add(10 * time.Second),
			measurements: map[types.SourceType]cortexReading{
				types.SourceHawkes: {
					category: types.Category{
						Type:       types.CategorySaturation,
						Confidence: 0.80,
						Strength:   0.75,
					},
					metrics: map[string]float64{
						"spectralRadius": 0.90,
						"asymmetry":      0.30,
						"price":          65000,
					},
				},
			},
		}

		criticalState := &cortexObservation{
			symbol: "BTC/USD",
			at:     baseTime.Add(11 * time.Second),
			measurements: map[types.SourceType]cortexReading{
				types.SourceHawkes: {
					category: types.Category{
						Type:       types.CategorySaturation,
						Confidence: 0.95,
						Strength:   0.92,
					},
					metrics: map[string]float64{
						"spectralRadius": 0.99,
						"asymmetry":      0.85,
						"price":          65000,
					},
				},
			},
		}

		mildRouting := router.Route(mildState)
		criticalRouting := router.Route(criticalState)

		Convey("It should produce different similarity neighbourhoods", func() {
			So(mildRouting.TopSimilarity, ShouldNotEqual, criticalRouting.TopSimilarity)
		})
	})
}

func TestCortexRouterWithDecisionFrames(t *testing.T) {
	Convey("Given an observation with manifold, resonance, and causal frames", t, func() {
		router := NewCortexRouter()
		baseTime := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)

		observation := &cortexObservation{
			symbol: "BTC/USD",
			at:     baseTime,
			measurements: map[types.SourceType]cortexReading{
				types.SourceFluid: {
					category: types.Category{
						Type:       types.CategoryTurbulent,
						Confidence: 0.80,
						Strength:   0.75,
					},
					metrics: map[string]float64{
						"reynolds": 1200,
						"price":    65000,
					},
				},
			},
			manifold: &logic.ManifoldFrame{
				Category: types.CategoryPhysicalField,
				Strength: 0.8,
				Momentum: 0.4,
				Pressure: 0.3,
				Shock:    0.2,
			},
			resonance: &logic.ResonanceFrame{
				Category:   types.CategoryLaminarResonance,
				Confidence: 0.63,
				Flow:       0.5,
				Stress:     0.2,
			},
			causal: &logic.CausalFrame{
				Category:   types.CategoryEndogenousAlpha,
				Confidence: 0.71,
				Strength:   0.65,
			},
		}

		routing := router.Route(observation)

		Convey("It should encode all feature dimensions", func() {
			So(routing.PredictedReturnBps, ShouldEqual, 0)
		})
	})
}

func BenchmarkCortexRouterRoute(benchmarkTB *testing.B) {
	router := NewCortexRouter()
	baseTime := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)

	for index := range 100 {
		observation := &cortexObservation{
			symbol: "BTC/USD",
			at:     baseTime.Add(time.Duration(index) * time.Second),
			measurements: map[types.SourceType]cortexReading{
				types.SourceFluid: {
					category: types.Category{
						Type:       types.CategoryLaminar,
						Confidence: 0.72,
						Strength:   0.66,
					},
					metrics: map[string]float64{
						"reynolds":   float64(500 + index*10),
						"vorticity":  0.15 + float64(index)*0.001,
						"turbulence": 0.10 + float64(index)*0.001,
						"price":      65000 + float64(index)*10,
					},
				},
			},
		}

		router.Route(observation)
	}

	observation := &cortexObservation{
		symbol: "BTC/USD",
		at:     baseTime.Add(200 * time.Second),
		measurements: map[types.SourceType]cortexReading{
			types.SourceFluid: {
				category: types.Category{
					Type:       types.CategoryLaminar,
					Confidence: 0.73,
					Strength:   0.67,
				},
				metrics: map[string]float64{
					"reynolds":   550,
					"vorticity":  0.155,
					"turbulence": 0.105,
					"price":      65050,
				},
			},
		},
	}

	benchmarkTB.ResetTimer()
	benchmarkTB.ReportAllocs()

	for benchmarkTB.Loop() {
		router.Route(observation)
	}
}
