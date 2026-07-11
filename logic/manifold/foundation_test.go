package manifold

import (
	"math"
	"testing"
	"time"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLifetimeEstimator(t *testing.T) {
	Convey("Given completed and censored lifetimes", t, func() {
		estimator := NewLifetimeEstimator()
		estimator.RecordCompleted(2 * time.Second)
		estimator.RecordCompleted(4 * time.Second)
		estimator.Censor(3 * time.Second)

		Convey("It should compute right-censored survival fractions", func() {
			So(estimator.SurvivalFraction(time.Second), ShouldEqual, 1)
			So(estimator.SurvivalFraction(3*time.Second), ShouldAlmostEqual, 0.333333, 0.0001)
		})
	})
}

func TestEpochTransformFreezesCoordinates(t *testing.T) {
	Convey("Given one epoch transform", t, func() {
		lifetime := NewLifetimeEstimator()
		lifetime.RecordCompleted(time.Second)
		mapper := NewCoordinateMapper(time.Second, 1e-9, lifetime)
		order := &PhysicalOrder{
			Side:       OrderSideBid,
			LimitPrice: 99,
			Quantity:   2,
			AddedAt:    time.Unix(1, 0),
		}

		transform, ready := mapper.BeginEpoch([]*PhysicalOrder{order}, 100, time.Unix(2, 0))
		first, _ := mapper.MapOrder(order, 100, time.Unix(2, 0), transform)
		second, _ := mapper.MapOrder(order, 101, time.Unix(2, 0), transform)

		Convey("It should keep age coordinates stable within the epoch", func() {
			So(ready, ShouldBeTrue)
			So(first.Age, ShouldEqual, second.Age)
		})
	})
}

func TestReferenceDepositsParity(t *testing.T) {
	Convey("Given cohort deposits", t, func() {
		config := &pmanifold.Config{
			GridX: 4, GridY: 4, GridZ: 4,
			DomainX: 4, DomainY: 4, DomainZ: 4,
			DeltaT: 0.1,
			Gamma:  5.0 / 3.0,
		}
		pmanifold.ApplyDerivedGasParams(config)
		depositor := NewMomentDepositor(config)
		cohorts := []Cohort{{
			Side:         OrderSideBid,
			Mass:         2,
			Centroid:     Coordinate{Price: 0, Size: 0, Age: 0},
			Velocity:     Coordinate{Price: 0.1, Size: 0, Age: 0},
			SecondMoment: Coordinate{Price: 0.01, Size: 0.02, Age: 0.03},
			CrossMoment: VelocityCross{
				PriceSize: 0.001,
				PriceAge:  0.002,
				SizeAge:   0.003,
			},
		}}

		reference := ReferenceDeposits(config, cohorts)
		depositorDeposits := depositor.Deposits(cohorts)

		Convey("It should match the moment depositor oracle", func() {
			So(DepositsEqual(reference, depositorDeposits, 1e-12), ShouldBeTrue)
		})
	})
}

func TestEventSubdivisions(t *testing.T) {
	Convey("Given fast cohort motion", t, func() {
		config := &pmanifold.Config{
			GridX: 8, GridY: 4, GridZ: 4,
			DomainX: 8, DomainY: 4, DomainZ: 4,
			DeltaT: 0.1,
			Gamma:  5.0 / 3.0,
		}
		pmanifold.ApplyDerivedGasParams(config)
		cohorts := []Cohort{{
			Mass:     1,
			Velocity: Coordinate{Price: 100, Size: 0, Age: 0},
		}}

		subdivisions := EventSubdivisions(config, 1.0, cohorts)

		Convey("It should require multiple substeps", func() {
			So(subdivisions, ShouldBeGreaterThan, 1)
		})
	})
}

func TestStateGasReady(t *testing.T) {
	Convey("Given a finite state with conservation residual", t, func() {
		readyState := State{
			Ready:                true,
			VisibleMass:          5,
			ConservationResidual: 0,
			DeltaT:               0.1,
			Subdivisions:         1,
			PriceScale:           0.01,
			SizeScale:            0.5,
			Reading: pmanifold.Reading{
				PressureGradNorm: 0.1,
				Divergence:       0.2,
				CoherenceMag2:    0.3,
				GuidanceSpeed:    0.4,
				ViscosityProxy:   0.5,
			},
		}
		brokenState := readyState
		brokenState.ConservationResidual = 1

		Convey("It should gate on conservation and event-time controls", func() {
			So(readyState.GasReady(), ShouldBeTrue)
			So(brokenState.GasReady(), ShouldBeFalse)
		})
	})
}

func TestTouchMassDensity(t *testing.T) {
	Convey("Given touch and off-touch bid orders", t, func() {
		orders := []*PhysicalOrder{
			{Side: OrderSideBid, Quantity: 2, Coordinate: Coordinate{Price: 0.1}},
			{Side: OrderSideBid, Quantity: 3, Coordinate: Coordinate{Price: 2}},
			{Side: OrderSideAsk, Quantity: 4, Coordinate: Coordinate{Price: 0.1}},
		}

		Convey("It should weight touch density by mass", func() {
			So(touchMassDensity(orders, OrderSideBid, 0.5), ShouldAlmostEqual, 0.4, 0.0001)
		})
	})
}

func BenchmarkLifetimeSurvivalFraction(b *testing.B) {
	estimator := NewLifetimeEstimator()

	for index := 0; index < 256; index++ {
		estimator.RecordCompleted(time.Duration(index) * time.Millisecond)
	}

	for b.Loop() {
		_ = estimator.SurvivalFraction(500 * time.Millisecond)
	}
}

func TestModeExtractorSpectrumAnchor(t *testing.T) {
	Convey("Given stationary cohorts", t, func() {
		config := &pmanifold.Config{
			GridX: 4, GridY: 4, GridZ: 4,
			DomainX: 4, DomainY: 4, DomainZ: 4,
			DeltaT: 0.1, Gamma: 5.0 / 3.0, MaxModes: 4,
		}
		pmanifold.ApplyDerivedGasParams(config)
		extractor := NewModeExtractor(config)
		cohorts := []Cohort{{
			Side:     OrderSideBid,
			Mass:     2,
			Centroid: Coordinate{Price: -0.1, Size: 0.2, Age: 0.3},
		}, {
			Side:     OrderSideAsk,
			Mass:     1,
			Centroid: Coordinate{Price: 0.1, Size: 0.2, Age: 0.4},
		}}

		modes := extractor.Modes(cohorts, 0.2)

		if len(modes) == 0 {
			modes = extractor.SpectrumAnchor(cohorts, 0.2)
		}

		Convey("It should derive event-spectrum oscillators from population geometry", func() {
			So(modes, ShouldNotBeEmpty)
			So(modes[0].Omega, ShouldAlmostEqual, 2*math.Pi/0.2, 0.0001)
		})
	})
}

func BenchmarkReferenceDeposits(b *testing.B) {
	config := &pmanifold.Config{
		GridX: 8, GridY: 4, GridZ: 4,
		DomainX: 8, DomainY: 4, DomainZ: 4,
		DeltaT: 0.1,
		Gamma:  5.0 / 3.0,
	}
	pmanifold.ApplyDerivedGasParams(config)
	cohorts := []Cohort{{
		Mass:     2,
		Centroid: Coordinate{Price: 0.1, Size: 0.2, Age: 0.3},
		Velocity: Coordinate{Price: 0.4, Size: 0.1, Age: 0.0},
	}}

	for b.Loop() {
		_ = ReferenceDeposits(config, cohorts)
	}
}
