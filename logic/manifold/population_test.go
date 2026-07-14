package manifold

import (
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

func (book testLevel3Book) InvalidReason(string) InvalidReason { return Valid }

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
		population := NewPopulation("BTC/USD", NewLifetimeEstimator(256), testBookDepth)
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

		population.Apply(row)

		Convey("It should retain exact order carriers", func() {
			So(population.Ready(), ShouldBeTrue)
			So(population.Orders(), ShouldHaveLength, 2)
			So(population.Accounting().Added, ShouldEqual, 0)
		})
	})
}

func TestPopulationApplyDepthBoundary(t *testing.T) {
	Convey("Given a population limited to two visible price levels per side", t, func() {
		at := time.Unix(1, 0)
		wire := func(event, orderID string, price, quantity float64) kraken.Level3Order {
			return kraken.Level3Order{
				Event: event, OrderID: orderID, LimitPrice: price,
				OrderQty: quantity, Timestamp: at,
			}
		}
		population := NewPopulation("BTC/USD", NewLifetimeEstimator(256), 2)
		population.Apply(kraken.Level3Data{
			Type: "snapshot", Timestamp: at,
			Bids: []kraken.Level3Order{
				wire("", "bid-1a", 100, 1), wire("", "bid-1b", 100, 1), wire("", "bid-2", 99, 2),
			},
			Asks: []kraken.Level3Order{
				wire("", "ask-1a", 110, 1), wire("", "ask-1b", 110, 1), wire("", "ask-2", 111, 2),
			},
		})

		Convey("When new best levels push the old boundary out and it later returns", func() {
			population.Apply(kraken.Level3Data{
				Type: "update", Timestamp: at,
				Bids: []kraken.Level3Order{wire("add", "bid-new", 101, 3)},
				Asks: []kraken.Level3Order{wire("add", "ask-new", 109, 3)},
			})
			accounting := population.Accounting()
			So(population.Orders(), ShouldHaveLength, 6)
			So(accounting.Initial, ShouldEqual, 8)
			So(accounting.Added, ShouldEqual, 6)
			So(accounting.ScopedOut, ShouldEqual, 4)
			So(accounting.Final(), ShouldEqual, 10)

			population.Apply(kraken.Level3Data{
				Type: "update", Timestamp: at,
				Bids: []kraken.Level3Order{wire("delete", "bid-new", 101, 3), wire("add", "bid-2", 99, 2)},
				Asks: []kraken.Level3Order{wire("delete", "ask-new", 109, 3), wire("add", "ask-2", 111, 2)},
			})
			accounting = population.Accounting()
			So(population.Ready(), ShouldBeTrue)
			So(population.Orders(), ShouldHaveLength, 6)
			So(accounting.Added, ShouldEqual, 10)
			So(accounting.Cancelled, ShouldEqual, 6)
			So(accounting.ScopedOut, ShouldEqual, 4)
			So(accounting.Final(), ShouldEqual, 8)
		})
	})
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

func TestPopulationTopOfBook(t *testing.T) {
	Convey("Given a population made one-sided by an update", t, func() {
		population := NewPopulation("BTC/USD", NewLifetimeEstimator(256), testBookDepth)

		population.Apply(kraken.Level3Data{
			Symbol: "BTC/USD", Type: "snapshot", Timestamp: time.Unix(1, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", LimitPrice: 99, OrderQty: 1, Timestamp: time.Unix(1, 0),
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "ask-1", LimitPrice: 101, OrderQty: 1, Timestamp: time.Unix(1, 0),
			}},
		})

		population.Apply(kraken.Level3Data{
			Symbol: "BTC/USD", Type: "update", Timestamp: time.Unix(2, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", Event: "delete", LimitPrice: 99, OrderQty: 1,
				Timestamp: time.Unix(2, 0),
			}},
		})

		Convey("A fresh snapshot should restore an executable top of book", func() {
			_, _, _, _, ready := population.TopOfBook()
			So(ready, ShouldBeFalse)

			population.Apply(kraken.Level3Data{
				Symbol: "BTC/USD", Type: "snapshot", Timestamp: time.Unix(3, 0),
				Bids: []kraken.Level3Order{{
					OrderID: "bid-2", LimitPrice: 98, OrderQty: 2, Timestamp: time.Unix(3, 0),
				}},
				Asks: []kraken.Level3Order{{
					OrderID: "ask-2", LimitPrice: 102, OrderQty: 2, Timestamp: time.Unix(3, 0),
				}},
			})

			bid, ask, bidQuantity, askQuantity, ready := population.TopOfBook()
			So(ready, ShouldBeTrue)
			So(bid, ShouldEqual, 98.0)
			So(ask, ShouldEqual, 102.0)
			So(bidQuantity, ShouldEqual, 2.0)
			So(askQuantity, ShouldEqual, 2.0)
		})
	})
}

func TestPopulationAccountingIdentity(t *testing.T) {
	Convey("Given a population with lifecycle events", t, func() {
		population := NewPopulation("ETH/USD", NewLifetimeEstimator(256), testBookDepth)

		population.Apply(kraken.Level3Data{
			Symbol: "ETH/USD", Type: "snapshot", Timestamp: time.Unix(1, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", LimitPrice: 100, OrderQty: 5, Timestamp: time.Unix(1, 0),
			}},
		})

		population.Apply(kraken.Level3Data{
			Symbol: "ETH/USD", Type: "update", Timestamp: time.Unix(2, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", Event: "modify", LimitPrice: 101, OrderQty: 3,
				Timestamp: time.Unix(2, 0),
			}},
		})

		population.Apply(kraken.Level3Data{
			Symbol: "ETH/USD", Type: "update", Timestamp: time.Unix(3, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", Event: "delete", LimitPrice: 101, OrderQty: 3,
				Timestamp: time.Unix(3, 0),
			}},
		})

		accounting := population.Accounting()

		Convey("It should satisfy the accounting identity", func() {
			So(accounting.Final(), ShouldEqual, 0)
			So(accounting.Amended, ShouldEqual, -2)
			So(accounting.Cancelled, ShouldEqual, 3)
			So(accounting.Filled, ShouldEqual, 0)
		})
	})
}
