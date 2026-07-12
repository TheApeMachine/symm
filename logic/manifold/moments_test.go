package manifold

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

func TestMomentDepositorConservation(t *testing.T) {
	Convey("Given equal quantities accumulated in different binary64 groupings", t, func() {
		config := &pmanifold.Config{
			GridX: 2, GridY: 1, GridZ: 1,
			DomainX: 2, DomainY: 1, DomainZ: 1,
		}
		builder := NewCohortBuilder(config)
		depositor := NewMomentDepositor(config)
		accounting := PopulationAccounting{}
		accounting.recordInitial(math.Exp2(53))
		accounting.recordInitial(1)
		accounting.recordInitial(1)
		orders := []*PhysicalOrder{
			{Side: OrderSideBid, Quantity: 1},
			{Side: OrderSideBid, Quantity: 1},
			{Side: OrderSideAsk, Quantity: math.Exp2(53)},
		}
		measurement := depositor.Conservation(accounting, builder.Build(orders))
		state := State{
			Ready: true, VisibleMass: measurement.VisibleMass,
			ConservationResidual: measurement.Residual,
			ConservationBound:    measurement.Bound,
			DeltaT:               1, Subdivisions: 1, PriceScale: 1, SizeScale: 1,
		}

		Convey("It should admit the grouping residual enclosed by the operation bound", func() {
			So(measurement.Residual, ShouldEqual, 2)
			So(measurement.Bound, ShouldBeGreaterThanOrEqualTo, math.Abs(measurement.Residual))
			So(state.GasReady(), ShouldBeTrue)
		})

		Convey("It should reject a materially different visible ledger", func() {
			orders = append(orders, &PhysicalOrder{Side: OrderSideBid, Quantity: 16})
			measurement = depositor.Conservation(accounting, builder.Build(orders))
			state.VisibleMass = measurement.VisibleMass
			state.ConservationResidual = measurement.Residual
			state.ConservationBound = measurement.Bound
			So(math.Abs(measurement.Residual), ShouldBeGreaterThan, measurement.Bound)
			So(state.GasReady(), ShouldBeFalse)
		})
	})
}

func BenchmarkMomentDepositorConservation(b *testing.B) {
	config := &pmanifold.Config{
		GridX: testBookDepth, GridY: SizeBins, GridZ: 1,
		DomainX: testBookDepth, DomainY: SizeBins, DomainZ: 1,
	}
	depositor := NewMomentDepositor(config)
	cohorts := make([]Cohort, 0, testBookDepth*SizeBins*2)
	accounting := PopulationAccounting{}

	for range cap(cohorts) {
		cohorts = append(cohorts, Cohort{Mass: 1})
		accounting.recordInitial(1)
	}

	measurement := ConservationMeasurement{}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		measurement = depositor.Conservation(accounting, cohorts)
	}

	if measurement.Residual != 0 {
		b.Fatal(measurement.Residual)
	}
}
