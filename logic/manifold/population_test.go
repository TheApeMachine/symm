package manifold

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

const testBookDepth = 10

func init() {
	viper.Set("market.l3_depth", testBookDepth)
}

func TestCoordinateMapper(t *testing.T) {
	Convey("Given a coordinate mapper with seeded scales", t, func() {
		lifetime := NewLifetimeEstimator(256)
		lifetime.RecordCompleted(time.Second)
		mapper := NewCoordinateMapper(time.Second, 1e-9, lifetime)
		order := &PhysicalOrder{
			Side:       OrderSideBid,
			LimitPrice: 99,
			Quantity:   2,
			AddedAt:    time.Unix(1, 0),
			UpdatedAt:  time.Unix(1, 0),
		}

		transform, ready := mapper.BeginEpoch([]*PhysicalOrder{order}, 100, time.Unix(1, 0))
		_, _ = mapper.MapOrder(order, 100, time.Unix(1, 0), transform)
		coordinate, ready := mapper.MapOrder(order, 100, time.Unix(2, 0), transform)

		Convey("It should emit signed price coordinates once scales are ready", func() {
			So(ready, ShouldBeTrue)
			So(coordinate.Price, ShouldBeLessThan, 0)
			So(coordinate.Size, ShouldAlmostEqual, 0)
		})
	})
}

func TestEngineConfig(t *testing.T) {
	Convey("Given the shared field engine", t, func() {
		engine := NewEngine()

		Convey("It should use price, size, and age grid axes", func() {
			So(engine.Config().GridX, ShouldEqual, testBookDepth)
			So(engine.Config().GridY, ShouldEqual, SizeBins)
			So(engine.Config().GridZ, ShouldEqual, AgeBins)
			So(engine.Config().Validate(), ShouldBeNil)
		})
	})
}

func TestCategoryOscillatorsRemoved(t *testing.T) {
	Convey("Given the field engine grid", t, func() {
		viper.Set("market.l3_depth", testBookDepth)
		engine := NewEngine()

		Convey("Then category lanes are not grid axes", func() {
			So(engine.Config().GridY, ShouldEqual, SizeBins)
			So(engine.Config().GridZ, ShouldEqual, AgeBins)
			So(engine.Config().GridY, ShouldNotEqual, len(types.CategoryOrder))
		})
	})
}

func TestModeExtractor(t *testing.T) {
	Convey("Given coherent cohort motion", t, func() {
		config := &pmanifold.Config{
			GridX: 4, GridY: 4, GridZ: 4,
			DomainX: 4, DomainY: 4, DomainZ: 4,
			DeltaT: 0.1, Gamma: 5.0 / 3.0, MaxModes: 4,
		}
		pmanifold.ApplyDerivedGasParams(config)
		extractor := NewModeExtractor(config)
		modes := extractor.Modes([]Cohort{{
			Mass:     1,
			Centroid: Coordinate{Price: 0.1, Size: 0.2, Age: 0.3},
			Velocity: Coordinate{Price: 0.4, Size: 0.1, Age: 0.0},
		}}, 0.1)

		Convey("It should emit at least one order-flow mode", func() {
			So(modes, ShouldNotBeEmpty)
			So(modes[0].Omega, ShouldBeGreaterThan, 0)
		})
	})
}

func TestCohortBuilderPreservesMass(t *testing.T) {
	Convey("Given mapped physical orders", t, func() {
		config := &pmanifold.Config{
			GridX: 4, GridY: 4, GridZ: 4,
			DomainX: 4, DomainY: 4, DomainZ: 1,
		}
		builder := NewCohortBuilder(config)
		orders := []*PhysicalOrder{
			{Side: OrderSideBid, Quantity: 2, Coordinate: Coordinate{Price: -0.1, Size: 0.2, Age: 0.1}},
			{Side: OrderSideAsk, Quantity: 3, Coordinate: Coordinate{Price: 0.1, Size: 0.3, Age: 0.2}},
		}

		cohorts := builder.Build(orders)

		Convey("It should preserve total carrier mass", func() {
			mass := 0.0

			for _, cohort := range cohorts {
				mass += cohort.Mass
			}

			So(mass, ShouldAlmostEqual, 5, 0.000001)
		})
	})
}

func TestCohortBuilderSeparatesSpatialCells(t *testing.T) {
	Convey("Given two same-side orders in different physical cells", t, func() {
		config := &pmanifold.Config{
			GridX: 4, GridY: 4, GridZ: 4,
			DomainX: 4, DomainY: 4, DomainZ: 1,
		}
		builder := NewCohortBuilder(config)
		orders := []*PhysicalOrder{
			{Side: OrderSideBid, Quantity: 2, Coordinate: Coordinate{Price: -1.5, Size: -1.5, Age: 0.1}},
			{Side: OrderSideBid, Quantity: 3, Coordinate: Coordinate{Price: 1.5, Size: 1.5, Age: 0.9}},
		}

		Convey("It should not collapse them into one side centroid", func() {
			So(builder.Build(orders), ShouldHaveLength, 2)
		})
	})
}

func TestMomentDepositor(t *testing.T) {
	Convey("Given one carrier cohort", t, func() {
		config := &pmanifold.Config{
			GridX: 4, GridY: 4, GridZ: 4,
			DomainX: 4, DomainY: 4, DomainZ: 4,
		}
		pmanifold.ApplyDerivedGasParams(config)
		depositor := NewMomentDepositor(config)
		cohorts := []Cohort{{
			Side:     OrderSideBid,
			Mass:     2,
			Centroid: Coordinate{Price: 0, Size: 0, Age: 0},
			Velocity: Coordinate{Price: 0.1, Size: 0, Age: 0},
		}}

		deposits := depositor.Deposits(cohorts)

		Convey("It should emit one conservative cell deposit", func() {
			So(deposits, ShouldHaveLength, 1)
			So(deposits[0].Rho, ShouldBeGreaterThan, 0)
			So(deposits[0].EInt, ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func TestStateIsFinite(t *testing.T) {
	Convey("Given a ready typed manifold state", t, func() {
		state := State{
			Ready:                true,
			VisibleMass:          1,
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

		Convey("It should be directly consumable by downstream logic", func() {
			So(state.IsFinite(), ShouldBeTrue)
		})
	})
}

func TestCoordinateVelocity(t *testing.T) {
	Convey("Given a mapped order with a prior coordinate", t, func() {
		lifetime := NewLifetimeEstimator(256)
		lifetime.RecordCompleted(time.Second)
		mapper := NewCoordinateMapper(time.Second, 1e-9, lifetime)
		order := &PhysicalOrder{
			Side:       OrderSideBid,
			LimitPrice: 99,
			Quantity:   2,
			AddedAt:    time.Unix(1, 0),
			UpdatedAt:  time.Unix(1, 0),
		}

		transform, _ := mapper.BeginEpoch([]*PhysicalOrder{order}, 100, time.Unix(1, 0))
		first, _ := mapper.MapOrder(order, 100, time.Unix(1, 0), transform)
		order.Coordinate = first
		order.MappedAt = time.Unix(1, 0)
		order.ScaleVersion = transform.Version
		order.ReferenceMid = 100

		transform, _ = mapper.BeginEpoch([]*PhysicalOrder{order}, 100, time.Unix(3, 0))
		second, ready := mapper.MapOrder(order, 100, time.Unix(3, 0), transform)

		Convey("It should derive nonzero velocity from coordinate change", func() {
			So(ready, ShouldBeTrue)
			mapper.UpdateVelocity(order, first, second, time.Unix(3, 0), transform, 100)
			So(order.Velocity.Age, ShouldNotEqual, 0)
		})
	})
}
