package logic

import (
	"strconv"
	"testing"
	"time"

	"github.com/spf13/viper"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

const bookDepth = 10

func init() {
	viper.Set("market.l3_depth", bookDepth)
	viper.Set("trading.edge.forward_return_horizon", 5*time.Minute)
}

var config = newTestConfig()

func newTestConfig() pmanifold.Config {
	config := pmanifold.Config{
		GridX:    uint32(bookDepth),
		GridY:    uint32(len(types.CategoryOrder)),
		GridZ:    uint32(len(analyzerSources)),
		DomainX:  float64(bookDepth),
		DomainY:  float64(len(types.CategoryOrder)),
		DomainZ:  float64(len(analyzerSources)),
		DeltaT:   types.Unit,
		Gamma:    idealGasGamma,
		MaxModes: uint32(len(types.CategoryOrder)),
	}

	pmanifold.ApplyDerivedGasParams(&config)

	return config
}

func TestNewAnalyzerConfig(t *testing.T) {
	Convey("Given analyzer manifold configuration inputs", t, func() {
		Convey("When the analyzer config is built", func() {
			Convey("Then it should allocate category lanes and source lanes", func() {
				So(config.GridX, ShouldEqual, bookDepth)
				So(config.GridY, ShouldEqual, len(types.CategoryOrder))
				So(config.GridZ, ShouldEqual, len(analyzerSources))
				So(config.MaxModes, ShouldEqual, len(types.CategoryOrder))
				So(config.Validate(), ShouldBeNil)
			})
		})
	})
}

func TestAnalyzerUpdate(t *testing.T) {
	Convey("Given analyzer measurements without a symbol", t, func() {
		analyzer := NewAnalyzer(nil, nil)

		Convey("When the analyzer updates", func() {
			theses := analyzer.Update([]*types.Measurement{{}})

			Convey("Then no unscoped thesis is created", func() {
				So(theses, ShouldBeEmpty)
			})
		})
	})

	Convey("Given the first analyzer measurement for a symbol", t, func() {
		analyzer := NewAnalyzer(nil, nil)
		measurement := &types.Measurement{
			Source: types.SourcePumpDump,
			Stream: "ticker",
			Symbol: "BTC/USD",
			At:     time.Unix(1, 0),
			Categories: []types.Category{{
				Type:       types.VerticalIgnition,
				Confidence: 1,
				Strength:   1,
			}},
			Metrics: map[string]float64{
				"rvol":        1,
				"spread":      1,
				"precursor":   1,
				"compression": 1,
			},
		}

		Convey("When the analyzer updates", func() {
			theses := analyzer.Update([]*types.Measurement{measurement})

			Convey("Then symbol state deposits into the manifold from the first reading", func() {
				So(theses, ShouldHaveLength, 1)

				manifoldFound, ok := analyzer.manifolds.Load("BTC/USD")
				So(ok, ShouldBeTrue)

				manifold := manifoldFound.(*Manifold)
				So(manifold, ShouldNotBeNil)
				So(manifold.solver, ShouldNotBeNil)

				resonanceFound, ok := analyzer.resonances.Load("BTC/USD")
				So(ok, ShouldBeTrue)
				So(resonanceFound, ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkAnalyzerUpdateColdSymbols(b *testing.B) {
	analyzer := NewAnalyzer(nil, nil)

	for index := 0; index < b.N; index++ {
		analyzer.Update([]*types.Measurement{{
			Source: types.SourcePumpDump,
			Stream: "ticker",
			Symbol: "BTC/USD-" + strconv.Itoa(index),
			At:     time.Unix(int64(index)+1, 0),
			Categories: []types.Category{{
				Type:       types.VerticalIgnition,
				Confidence: 1,
				Strength:   1,
			}},
			Metrics: map[string]float64{
				"rvol":        1,
				"spread":      1,
				"precursor":   1,
				"compression": 1,
			},
		}})
	}
}

func TestCategoryOscillators(t *testing.T) {
	Convey("Given an analyzer manifold grid", t, func() {
		config.GridX = bookDepth
		config.GridY = uint32(len(types.CategoryOrder))
		config.GridZ = uint32(len(analyzerSources))

		Convey("When category oscillators are created", func() {
			oscillators := make([]pmanifold.Oscillator, len(types.CategoryOrder))

			for index := range types.CategoryOrder {
				oscillators[index] = pmanifold.Oscillator{
					Phase:     0,
					Omega:     types.Unit,
					Amplitude: types.Unit,
					PosX:      float64(config.GridX) - float64(1)/2,
					PosY:      float64(index),
					PosZ:      float64(config.GridZ) - float64(1)/2,
					Heat:      types.Unit,
				}
			}

			Convey("Then one oscillator should occupy each category lane", func() {
				So(oscillators, ShouldHaveLength, len(types.CategoryOrder))
				So(oscillators[0].PosX, ShouldEqual, float64(config.GridX)-0.5)
				So(oscillators[0].PosY, ShouldEqual, 0)
				So(oscillators[0].PosZ, ShouldEqual, float64(config.GridZ)-0.5)
				So(oscillators[len(oscillators)-1].PosY, ShouldEqual, len(types.CategoryOrder)-1)
			})

			Convey("Then the initial phase driver should be stationary", func() {
				for _, oscillator := range oscillators {
					So(oscillator.Phase, ShouldEqual, 0)
					So(oscillator.Omega, ShouldEqual, types.Unit)
					So(oscillator.Amplitude, ShouldEqual, types.Unit)
					So(oscillator.Heat, ShouldEqual, types.Unit)
					So(oscillator.VelX, ShouldEqual, 0)
					So(oscillator.VelY, ShouldEqual, 0)
					So(oscillator.VelZ, ShouldEqual, 0)
				}
			})
		})
	})
}
