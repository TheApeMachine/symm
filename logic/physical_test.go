package logic

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPhysicalManifoldEvidence(testingTB *testing.T) {
	Convey("Given manifold readback features", testingTB, func() {
		physical := &physicalManifold{}
		reading := pmanifold.Reading{
			PressureGradNorm: 12,
			CoherenceMag2:    0.2,
			GuidanceSpeed:    0.5,
			ViscosityProxy:   0.5,
		}
		rho := rhoEvidence{
			mass:     10,
			peak:     3,
			entropy:  1.5,
			gradient: 2.5,
		}
		oscillators := oscillatorEvidence{
			coherence: 0.8,
			kinetic:   0.7,
			thermal:   0.6,
		}
		particles := []pmanifold.Oscillator{
			{PosX: 3, PosY: 1, PosZ: 4, Phase: 0.7, Amplitude: 2, VelX: 0.3},
		}

		Convey("When physical evidence is assembled", func() {
			evidence, err := physical.evidence(
				reading,
				pmanifold.Reading{PressureGradNorm: 4, ViscosityProxy: 5},
				[][]float64{{1, 0}, {0, 3}},
				rho,
				oscillators,
				particles,
			)

			Convey("Then it should preserve field state instead of classifying it", func() {
				So(err, ShouldBeNil)
				So(evidence.category, ShouldEqual, types.CategoryPhysicalField)
				So(evidence.rho.mass, ShouldEqual, rho.mass)
				So(evidence.rho.gradient, ShouldEqual, rho.gradient)
				So(evidence.oscillators.coherence, ShouldEqual, oscillators.coherence)
				So(evidence.particles, ShouldResemble, particles)
				So(evidence.strength, ShouldEqual, rho.gradient)
				So(evidence.shock, ShouldEqual, 4)
				So(evidence.resistance, ShouldEqual, 5)
			})
		})
	})
}

func TestPhysicalManifoldCell(testingTB *testing.T) {
	Convey("Given a normalized clamp position", testingTB, func() {
		physical := &physicalManifold{}

		Convey("When it is mapped to solver cells", func() {
			Convey("Then the edge positions should map to grid edges", func() {
				So(physical.cell(0, 8), ShouldEqual, 0)
				So(physical.cell(1, 8), ShouldEqual, 7)
			})

			Convey("Then the midpoint should map to the nearest semantic middle cell", func() {
				So(physical.cell(0.5, 8), ShouldEqual, 4)
			})
		})
	})
}

func TestPhysicalManifoldWrap(testingTB *testing.T) {
	Convey("Given decision oscillators", testingTB, func() {
		physical := &physicalManifold{
			config: pmanifold.Config{
				GridX: 8,
				GridY: 4,
				GridZ: 8,
				Gamma: 5.0 / 3.0,
			},
		}
		oscillators := []pmanifold.Oscillator{
			{PosX: 1, PosY: 7, PosZ: 0.5, Omega: 0.72, Heat: 0.1},
		}

		Convey("When they are wrapped for the manifold solver", func() {
			wrapped := physical.wrap(oscillators)

			Convey("Then pressure should become ideal-gas internal energy", func() {
				So(wrapped, ShouldHaveLength, 1)
				So(wrapped[0].PosX, ShouldEqual, 7)
				So(wrapped[0].PosY, ShouldEqual, 3)
				So(wrapped[0].PosZ, ShouldEqual, 4)
				So(wrapped[0].Heat, ShouldAlmostEqual, 1.08)
			})
		})
	})
}

func TestPhysicalManifoldSettle(testingTB *testing.T) {
	Convey("Given a full live boundary frame", testingTB, func() {
		restoreDecisionConfig()
		viper.Set("market.book.depth", 25)
		at := time.Date(2026, 7, 7, 15, 45, 42, 0, time.UTC)
		boundaries := newBoundaryClamps()
		frame, frameErr := boundaries.Frame(
			"BTC/USD",
			fullBoundaryMeasurements("BTC/USD", at),
		)
		decision := NewDecision(nil)
		defer decision.Close()
		state := decision.state("BTC/USD")
		runtime, runtimeErr := decision.runtime(1, "BTC/USD", state)
		advanceErr := runtime.Advance(frame.eventAt)
		physical, physicalErr := newPhysicalManifold()

		if physical != nil {
			defer physical.Close()
		}

		Convey("When the physical manifold settles the frame and intervention", func() {
			var controlsErr error
			var evidence physicalEvidence
			var settleErr error
			var intervened physicalEvidence
			var interveneErr error

			if physical != nil {
				controlsErr = physical.SetControls(runtime)
				evidence, settleErr = physical.Settle(frame)
				intervened, interveneErr = physical.Settle(frame.Intervene())
			}

			Convey("Then rho and particle readback should stay finite", func() {
				So(frameErr, ShouldBeNil)
				So(runtimeErr, ShouldBeNil)
				So(advanceErr, ShouldBeNil)
				So(physicalErr, ShouldBeNil)
				So(controlsErr, ShouldBeNil)
				So(settleErr, ShouldBeNil)
				So(interveneErr, ShouldBeNil)
				So(evidence.rho.mass, ShouldBeGreaterThan, 0)
				So(intervened.rho.mass, ShouldBeGreaterThan, 0)
				So(evidence.particles, ShouldHaveLength, len(frame.oscillators))
				So(intervened.particles, ShouldHaveLength, len(frame.oscillators))

				for _, particle := range evidence.particles {
					So(finite(particle.Phase), ShouldBeTrue)
					So(finite(particle.PosX), ShouldBeTrue)
					So(finite(particle.PosY), ShouldBeTrue)
					So(finite(particle.PosZ), ShouldBeTrue)
					So(finite(particle.VelX), ShouldBeTrue)
					So(finite(particle.VelY), ShouldBeTrue)
					So(finite(particle.VelZ), ShouldBeTrue)
				}

				for _, particle := range intervened.particles {
					So(finite(particle.Phase), ShouldBeTrue)
					So(finite(particle.PosX), ShouldBeTrue)
					So(finite(particle.PosY), ShouldBeTrue)
					So(finite(particle.PosZ), ShouldBeTrue)
					So(finite(particle.VelX), ShouldBeTrue)
					So(finite(particle.VelY), ShouldBeTrue)
					So(finite(particle.VelZ), ShouldBeTrue)
				}
			})
		})
	})
}

func TestPhysicalManifoldSparseSettle(testingTB *testing.T) {
	Convey("Given a sparse startup boundary frame", testingTB, func() {
		restoreDecisionConfig()
		viper.Set("market.book.depth", 25)
		at := time.Date(2026, 7, 7, 16, 17, 56, 0, time.UTC)
		boundaries := newBoundaryClamps()
		frame, frameErr := boundaries.Frame(
			"BTC/USD",
			map[types.SourceType]*types.Measurement{
				types.SourceHawkes: hawkesMeasurement("BTC/USD", at, 0),
			},
		)
		physical, physicalErr := newPhysicalManifold()

		if physical != nil {
			defer physical.Close()
		}

		Convey("When the physical manifold settles before every source is present", func() {
			var evidence physicalEvidence
			var settleErr error

			if physical != nil {
				evidence, settleErr = physical.Settle(frame)
			}

			Convey("Then clamp deposits should produce rho mass", func() {
				So(frameErr, ShouldBeNil)
				So(physicalErr, ShouldBeNil)
				So(settleErr, ShouldBeNil)
				So(evidence.rho.mass, ShouldBeGreaterThan, 0)
				So(evidence.particles, ShouldHaveLength, len(frame.oscillators))
			})
		})
	})
}

func TestPhysicalFieldRho(testingTB *testing.T) {
	Convey("Given a rho projection", testingTB, func() {
		field := newPhysicalField()
		rows := [][]float64{
			{1, 0},
			{0, 3},
		}

		Convey("When rho evidence is measured", func() {
			rho, err := field.Rho(rows)

			Convey("Then it should expose field mass and shape", func() {
				So(err, ShouldBeNil)
				So(rho.mass, ShouldEqual, 4)
				So(rho.peak, ShouldEqual, 3)
				So(rho.entropy, ShouldBeGreaterThan, 0)
				So(rho.gradient, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func fullBoundaryMeasurements(
	symbol string,
	at time.Time,
) map[types.SourceType]*types.Measurement {
	return map[types.SourceType]*types.Measurement{
		types.SourceHawkes:      hawkesMeasurement(symbol, at, 0),
		types.SourceCVD:         cvdMeasurement(symbol, at, 0),
		types.SourceFluid:       fluidMeasurement(symbol, at, 0),
		types.SourceDepthFlow:   depthFlowMeasurement(symbol, at),
		types.SourceLiquidity:   liquidityMeasurement(symbol, at, 0),
		types.SourcePumpDump:    pumpDumpMeasurement(symbol, at),
		types.SourceExhaustion:  exhaustionMeasurement(symbol, at),
		types.SourceToxicity:    toxicityMeasurement(symbol, at),
		types.SourceLeadLag:     leadLagMeasurement(symbol, at),
		types.SourceCorrelation: correlationMeasurement(symbol, at),
		types.SourceSentiment:   sentimentMeasurement(symbol, at),
	}
}

func depthFlowMeasurement(symbol string, at time.Time) *types.Measurement {
	return boundaryMeasurement(
		types.SourceDepthFlow,
		symbol,
		at,
		[]types.Category{
			{Type: types.CategoryLoadedImbalance, Confidence: 0.46, Strength: 0.31},
			{Type: types.CategoryBookThinning, Confidence: 0.36, Strength: 0.18},
			{Type: types.CategoryDenseNeutrality, Confidence: 0.18, Strength: 0.11},
		},
		map[string]float64{
			"loadedScore":  0.31,
			"spoofScore":   0.07,
			"thinScore":    0.18,
			"neutralScore": 0.11,
		},
	)
}

func pumpDumpMeasurement(symbol string, at time.Time) *types.Measurement {
	return boundaryMeasurement(
		types.SourcePumpDump,
		symbol,
		at,
		[]types.Category{
			{Type: types.CategoryVerticalIgnition, Confidence: 0.42, Strength: 0.29},
			{Type: types.CategoryCoiledCompression, Confidence: 0.34, Strength: 0.24},
			{Type: types.CategoryFadedExhaustion, Confidence: 0.24, Strength: 0.16},
		},
		map[string]float64{
			"ignition":    0.29,
			"compression": 0.24,
			"trend":       0.21,
			"exhaustion":  0.16,
			"rvol":        0.37,
			"spread":      0.04,
		},
	)
}

func exhaustionMeasurement(symbol string, at time.Time) *types.Measurement {
	return boundaryMeasurement(
		types.SourceExhaustion,
		symbol,
		at,
		[]types.Category{
			{Type: types.CategoryFragileExpansion, Confidence: 0.38, Strength: 0.26},
			{Type: types.CategoryActiveReversal, Confidence: 0.34, Strength: 0.22},
			{Type: types.CategoryThermalExhaustion, Confidence: 0.28, Strength: 0.19},
		},
		map[string]float64{
			"fragile":    0.26,
			"reversal":   0.22,
			"mechanical": 0.17,
			"thermal":    0.19,
			"urgency":    0.21,
		},
	)
}

func toxicityMeasurement(symbol string, at time.Time) *types.Measurement {
	return boundaryMeasurement(
		types.SourceToxicity,
		symbol,
		at,
		[]types.Category{
			{Type: types.CategoryHardSupport, Confidence: 0.40, Strength: 0.20},
			{Type: types.CategoryToxicBluff, Confidence: 0.34, Strength: 0.18},
			{Type: types.CategoryLiquidityVacuum, Confidence: 0.26, Strength: 0.14},
		},
		map[string]float64{
			"supportScore": 0.20,
			"bluffScore":   0.18,
			"vacuumScore":  0.14,
		},
	)
}

func leadLagMeasurement(symbol string, at time.Time) *types.Measurement {
	return boundaryMeasurement(
		types.SourceLeadLag,
		symbol,
		at,
		[]types.Category{
			{Type: types.CategoryInefficientLag, Confidence: 0.44, Strength: 0.27},
			{Type: types.CategoryAnchorStall, Confidence: 0.30, Strength: 0.16},
			{Type: types.CategoryDecoupledMove, Confidence: 0.26, Strength: 0.13},
		},
		map[string]float64{
			"sampleSupport": 0.31,
			"lagFraction":   0.27,
			"stall":         0.16,
			"decoupled":     0.13,
			"inefficient":   0.27,
		},
	)
}

func correlationMeasurement(symbol string, at time.Time) *types.Measurement {
	return boundaryMeasurement(
		types.SourceCorrelation,
		symbol,
		at,
		[]types.Category{
			{Type: types.CategoryDecoupledAlpha, Confidence: 0.36, Strength: 0.23},
			{Type: types.CategoryDivergentStress, Confidence: 0.33, Strength: 0.21},
			{Type: types.CategorySystemicBeta, Confidence: 0.31, Strength: 0.19},
		},
		map[string]float64{
			"relativeEnergy": 0.25,
			"signed":         0.04,
			"noiseScore":     0.11,
			"alphaScore":     0.23,
			"herdScore":      0.19,
			"stressScore":    0.21,
		},
	)
}

func sentimentMeasurement(symbol string, at time.Time) *types.Measurement {
	return boundaryMeasurement(
		types.SourceSentiment,
		symbol,
		at,
		[]types.Category{
			{Type: types.CategoryRiskOnSurge, Confidence: 0.42, Strength: 0.24},
			{Type: types.CategorySystemicSlump, Confidence: 0.36, Strength: 0.20},
			{Type: types.CategoryDivergentMove, Confidence: 0.22, Strength: 0.12},
		},
		map[string]float64{
			"breadth":        0.24,
			"leaderEvidence": 0.21,
			"leaderStrength": 0.19,
			"slumpScore":     0.20,
			"divergentScore": 0.12,
			"surgeScore":     0.24,
		},
	)
}

func boundaryMeasurement(
	source types.SourceType,
	symbol string,
	at time.Time,
	categories []types.Category,
	metrics map[string]float64,
) *types.Measurement {
	metrics["price"] = 100

	return &types.Measurement{
		Source:        source,
		Symbol:        symbol,
		At:            at,
		EntryBaseline: 0.2,
		ExitBaseline:  0.1,
		Categories:    categories,
		Metrics:       metrics,
	}
}

func TestPhysicalFieldOscillators(testingTB *testing.T) {
	Convey("Given oscillator readback", testingTB, func() {
		field := newPhysicalField()
		oscillators := []pmanifold.Oscillator{
			{Amplitude: 1, Phase: 0, VelX: 1, Heat: 2, Omega: 3},
			{Amplitude: 1, Phase: 0, VelX: 2, Heat: 4, Omega: 5},
		}

		Convey("When oscillator evidence is measured", func() {
			evidence, err := field.Oscillators(oscillators)

			Convey("Then it should expose carrier coherence and energy", func() {
				So(err, ShouldBeNil)
				So(evidence.coherence, ShouldEqual, 1)
				So(evidence.kinetic, ShouldEqual, 1.5)
				So(evidence.thermal, ShouldEqual, 3)
				So(evidence.omega, ShouldEqual, 4)
			})
		})
	})
}
