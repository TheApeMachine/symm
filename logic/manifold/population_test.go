package manifold

import (
	"math"
	"testing"
	"time"

	"github.com/spf13/viper"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

const testBookDepth = 10

type testLevel3Book struct {
	bid float64
	ask float64
}

func (book testLevel3Book) Apply(kraken.Level3Data, int, int) bool { return true }

func (book testLevel3Book) Invalid(string) bool { return false }

func (book testLevel3Book) TopOfBook(string) (float64, float64, bool) {
	if book.bid <= 0 || book.ask <= 0 {
		return 0, 0, false
	}

	return book.bid, book.ask, true
}

func init() {
	viper.Set("market.l3_depth", testBookDepth)
}

func TestPopulationApplySnapshot(t *testing.T) {
	Convey("Given a population ledger", t, func() {
		population := NewPopulation("BTC/USD", NewLifetimeEstimator())
		row := kraken.Level3Data{
			Symbol:    "BTC/USD",
			Type:      "snapshot",
			Timestamp: time.Unix(1, 0),
			Checksum:  1063832831,
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", LimitPrice: 99, OrderQty: 2,
				Timestamp: time.Unix(1, 0),
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "ask-1", LimitPrice: 101, OrderQty: 3,
				Timestamp: time.Unix(1, 0),
			}},
		}

		population.Apply(row, 100)

		Convey("It should retain exact order carriers", func() {
			So(population.Ready(), ShouldBeTrue)
			So(population.Orders(), ShouldHaveLength, 2)
			So(population.Accounting().Added, ShouldEqual, 0)
		})
	})
}

func TestCoordinateMapper(t *testing.T) {
	Convey("Given a coordinate mapper with seeded scales", t, func() {
		lifetime := NewLifetimeEstimator()
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
			So(coordinate.Size, ShouldBeGreaterThan, 0)
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
		builder := NewCohortBuilder(64)
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

func TestStateFromEvidence(t *testing.T) {
	Convey("Given typed state evidence", t, func() {
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

		Convey("It should decode for downstream resonance", func() {
			decoded, ok := StateFromEvidence(state)
			So(ok, ShouldBeTrue)
			So(decoded.IsFinite(), ShouldBeTrue)
		})
	})
}

func TestForecaster(t *testing.T) {
	Convey("Given a finite state with touch density", t, func() {
		forecaster := NewForecaster()
		state := State{
			At:                   time.Unix(1, 0),
			Ready:                true,
			VisibleMass:          1,
			ConservationResidual: 0,
			DeltaT:               0.1,
			Subdivisions:         1,
			PriceScale:           0.01,
			SizeScale:            0.5,
			BidTouchDensity:      0.6,
			AskTouchDensity:      0.4,
			Reading: pmanifold.Reading{
				Divergence:       0.1,
				PressureGradNorm: 0.2,
				CoherenceMag2:    0.5,
				GuidanceSpeed:    0.05,
				ViscosityProxy:   0.01,
			},
			StressAnisotropy: 0.05,
		}

		forecasts := forecaster.Forecast(state)

		Convey("It should emit bid and ask touch survival", func() {
			So(forecasts.BidTouchSurvival, ShouldBeGreaterThan, 0)
			So(forecasts.AskTouchSurvival, ShouldBeGreaterThan, 0)
			So(forecasts.BidTouchSurvival, ShouldBeGreaterThan, forecasts.AskTouchSurvival)
		})

		Convey("It should derive executable return from guidance and impact", func() {
			So(forecasts.MidMove, ShouldBeGreaterThan, 0)
			So(math.IsNaN(forecasts.ExecutableReturn), ShouldBeFalse)
		})
	})
}

func TestCoordinateVelocity(t *testing.T) {
	Convey("Given a mapped order with a prior coordinate", t, func() {
		lifetime := NewLifetimeEstimator()
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

		transform, _ = mapper.BeginEpoch([]*PhysicalOrder{order}, 100, time.Unix(3, 0))
		second, ready := mapper.MapOrder(order, 100, time.Unix(3, 0), transform)

		Convey("It should derive nonzero velocity from coordinate change", func() {
			So(ready, ShouldBeTrue)
			mapper.UpdateVelocity(order, first, second, time.Unix(3, 0))
			So(order.Velocity.Age, ShouldNotEqual, 0)
		})
	})
}

func TestPopulationRecoversAfterInvalidMid(t *testing.T) {
	Convey("Given a population invalidated by a one-sided update", t, func() {
		population := NewPopulation("BTC/USD", NewLifetimeEstimator())

		population.Apply(kraken.Level3Data{
			Symbol: "BTC/USD", Type: "snapshot", Timestamp: time.Unix(1, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", LimitPrice: 99, OrderQty: 1, Timestamp: time.Unix(1, 0),
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "ask-1", LimitPrice: 101, OrderQty: 1, Timestamp: time.Unix(1, 0),
			}},
		}, 100)

		population.Apply(kraken.Level3Data{
			Symbol: "BTC/USD", Type: "update", Timestamp: time.Unix(2, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", Event: "delete", LimitPrice: 99, OrderQty: 1,
				Timestamp: time.Unix(2, 0),
			}},
		}, 0)

		Convey("A fresh snapshot should clear invalid state", func() {
			So(population.Ready(), ShouldBeFalse)

			population.Apply(kraken.Level3Data{
				Symbol: "BTC/USD", Type: "snapshot", Timestamp: time.Unix(3, 0),
				Bids: []kraken.Level3Order{{
					OrderID: "bid-2", LimitPrice: 98, OrderQty: 2, Timestamp: time.Unix(3, 0),
				}},
				Asks: []kraken.Level3Order{{
					OrderID: "ask-2", LimitPrice: 102, OrderQty: 2, Timestamp: time.Unix(3, 0),
				}},
			}, 100)

			So(population.Ready(), ShouldBeTrue)
		})
	})
}

func TestPopulationAccountingIdentity(t *testing.T) {
	Convey("Given a population with lifecycle events", t, func() {
		population := NewPopulation("ETH/USD", NewLifetimeEstimator())

		population.Apply(kraken.Level3Data{
			Symbol: "ETH/USD", Type: "snapshot", Timestamp: time.Unix(1, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", LimitPrice: 100, OrderQty: 5, Timestamp: time.Unix(1, 0),
			}},
		}, 100)

		population.Apply(kraken.Level3Data{
			Symbol: "ETH/USD", Type: "update", Timestamp: time.Unix(2, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", Event: "modify", LimitPrice: 101, OrderQty: 3,
				Timestamp: time.Unix(2, 0),
			}},
		}, 100)

		population.Apply(kraken.Level3Data{
			Symbol: "ETH/USD", Type: "update", Timestamp: time.Unix(3, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", Event: "delete", LimitPrice: 101, OrderQty: 3,
				Timestamp: time.Unix(3, 0),
			}},
		}, 100)

		accounting := population.Accounting()

		Convey("It should satisfy the accounting identity", func() {
			So(accounting.Final(), ShouldEqual, 0)
			So(accounting.Amended, ShouldEqual, -2)
			So(accounting.Cancelled, ShouldEqual, 3)
			So(accounting.Filled, ShouldEqual, 0)
		})
	})
}
