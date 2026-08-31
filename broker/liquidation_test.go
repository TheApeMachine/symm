package broker

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func liquidationFee() *kraken.TradeVolumeFee {
	return &kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.25)}
}

func mustDecimalOrder(orderID, price, qty string) kraken.Level3Order {
	return kraken.Level3Order{
		OrderID:    orderID,
		LimitPrice: mustDecimal(price),
		OrderQty:   mustDecimal(qty),
	}
}

func snapshotL3(bids, asks []kraken.Level3Order) kraken.Level3Data {
	return kraken.Level3Data{
		Symbol: "EDGE/USD",
		Type:   "snapshot",
		Bids:   bids,
		Asks:   asks,
	}
}

func updateL3(bids, asks []kraken.Level3Order) kraken.Level3Data {
	return kraken.Level3Data{
		Symbol: "EDGE/USD",
		Type:   "update",
		Bids:   bids,
		Asks:   asks,
	}
}

/*
TestLiquidationSnapshotThenUpdates covers the streaming contract A: a genuine
snapshot followed by add/modify/delete keeps exact executable geometry.
*/
func TestLiquidationSnapshotThenUpdates(t *testing.T) {
	Convey("Given a reducer seeded from a genuine snapshot", t, func() {
		reducer := newLiquidationReducer("EDGE/USD")
		reducer.Apply(snapshotL3(
			[]kraken.Level3Order{
				mustDecimalOrder("b1", "100", "500"),
				mustDecimalOrder("b2", "99", "500"),
			},
			[]kraken.Level3Order{mustDecimalOrder("a1", "101", "1000")},
		), 1)

		surface := reducer.Surface(
			decimal.NewFromFloat64(1000), nil, liquidationFee(), time.Now(),
		)
		So(surface.FullyExecutable, ShouldBeTrue)
		So(surface.ExecutableVWAP.Cmp(mustDecimal("99.5")), ShouldEqual, 0)

		Convey("add", func() {
			reducer.Apply(updateL3(
				[]kraken.Level3Order{mustDecimalOrder("b3", "98", "500")},
				[]kraken.Level3Order{mustDecimalOrder("a1", "101", "1500")},
			), 1)

			surface = reducer.Surface(
				decimal.NewFromFloat64(1500), nil, liquidationFee(), time.Now(),
			)
			So(surface.FullyExecutable, ShouldBeTrue)
			// (100*500 + 99*500 + 98*500)/1500 = 99
			So(surface.ExecutableVWAP.Cmp(mustDecimal("99")), ShouldEqual, 0)
		})

		Convey("modify", func() {
			reducer.Apply(updateL3(
				[]kraken.Level3Order{mustDecimalOrder("b2", "101", "500")},
				[]kraken.Level3Order{mustDecimalOrder("a1", "102", "1000")},
			), 1)

			surface = reducer.Surface(
				decimal.NewFromFloat64(1000), nil, liquidationFee(), time.Now(),
			)
			So(surface.FullyExecutable, ShouldBeTrue)
			// (101*500 + 100*500)/1000 = 100.5
			So(surface.ExecutableVWAP.Cmp(mustDecimal("100.5")), ShouldEqual, 0)
		})

		Convey("delete", func() {
			reducer.Apply(updateL3(
				[]kraken.Level3Order{{Event: "delete", OrderID: "b2"}},
				nil,
			), 1)

			surface = reducer.Surface(
				decimal.NewFromFloat64(1000), nil, liquidationFee(), time.Now(),
			)

			So(surface.FullyExecutable, ShouldBeFalse)
			So(surface.BookComplete, ShouldBeTrue)
			So(surface.ExecutableQty.Cmp(decimal.NewFromFloat64(500)), ShouldEqual, 0)
		})
	})
}

/*
TestLiquidationMidStreamFirstUpdateNotSnapshot covers contract C: an update
observed before any genuine snapshot is never promoted to a baseline.
*/
func TestLiquidationMidStreamFirstUpdateNotSnapshot(t *testing.T) {
	Convey("Given a reducer with no valid snapshot", t, func() {
		reducer := newLiquidationReducer("EDGE/USD")

		Convey("an update containing both sides leaves the surface incomplete", func() {
			reducer.Apply(updateL3(
				[]kraken.Level3Order{mustDecimalOrder("b1", "100", "1000")},
				[]kraken.Level3Order{mustDecimalOrder("a1", "101", "1000")},
			), 1)

			surface := reducer.Surface(
				decimal.NewFromFloat64(1000), nil, liquidationFee(), time.Now(),
			)

			So(surface.BookComplete, ShouldBeFalse)
			So(surface.FullyExecutable, ShouldBeFalse)
			So(surface.ExecutableVWAP, ShouldBeNil)
		})
	})
}

/*
TestLiquidationReconnect covers contract D: a reconnect/epoch change invalidates
the old epoch's state until the new epoch's genuine snapshot seeds it.
*/
func TestLiquidationReconnect(t *testing.T) {
	Convey("Given a valid epoch followed by a reconnect", t, func() {
		reducer := newLiquidationReducer("EDGE/USD")
		reducer.Apply(snapshotL3(
			[]kraken.Level3Order{mustDecimalOrder("b1", "100", "1000")},
			[]kraken.Level3Order{mustDecimalOrder("a1", "101", "1000")},
		), 1)

		surface := reducer.Surface(
			decimal.NewFromFloat64(1000), nil, liquidationFee(), time.Now(),
		)
		So(surface.FullyExecutable, ShouldBeTrue)

		Convey("an update in the new epoch before its snapshot stays incomplete", func() {
			reducer.Apply(updateL3(
				[]kraken.Level3Order{mustDecimalOrder("b1", "99", "1000")},
				[]kraken.Level3Order{mustDecimalOrder("a1", "101", "1000")},
			), 2)

			surface = reducer.Surface(
				decimal.NewFromFloat64(1000), nil, liquidationFee(), time.Now(),
			)

			So(surface.BookComplete, ShouldBeFalse)
			So(surface.FullyExecutable, ShouldBeFalse)
			So(surface.ExecutableVWAP, ShouldBeNil)
		})

		Convey("the new epoch's genuine snapshot reseeds valid state", func() {
			reducer.Apply(snapshotL3(
				[]kraken.Level3Order{mustDecimalOrder("b1", "98", "1000")},
				[]kraken.Level3Order{mustDecimalOrder("a1", "99", "1000")},
			), 2)

			surface = reducer.Surface(
				decimal.NewFromFloat64(1000), nil, liquidationFee(), time.Now(),
			)

			So(surface.FullyExecutable, ShouldBeTrue)
			So(surface.ExecutableVWAP.Cmp(mustDecimal("98")), ShouldEqual, 0)
		})
	})
}

/*
TestLiquidationIncremental covers contracts E/F/G/H via one seeded reducer:
multi-level VWAP, insufficient quantity, floor coverage disappearance, and
crossed states.
*/
func TestLiquidationIncremental(t *testing.T) {
	Convey("Given a seeded liquidation reducer", t, func() {
		reducer := newLiquidationReducer("EDGE/USD")
		reducer.Apply(snapshotL3(
			[]kraken.Level3Order{
				mustDecimalOrder("b1", "100", "500"),
				mustDecimalOrder("b2", "99", "500"),
			},
			[]kraken.Level3Order{mustDecimalOrder("a1", "101", "1000")},
		), 1)

		Convey("a fill spanning multiple levels computes true VWAP (E)", func() {
			surface := reducer.Surface(
				decimal.NewFromFloat64(1000), nil, liquidationFee(), time.Now(),
			)

			So(surface.FullyExecutable, ShouldBeTrue)
			So(surface.ExecutableVWAP.Cmp(mustDecimal("99.5")), ShouldEqual, 0)
		})

		Convey("insufficient quantity reports no VWAP (F)", func() {
			surface := reducer.Surface(
				decimal.NewFromFloat64(1500), nil, liquidationFee(), time.Now(),
			)

			So(surface.BookComplete, ShouldBeTrue)
			So(surface.FullyExecutable, ShouldBeFalse)
			So(surface.ExecutableVWAP, ShouldBeNil)
		})

		Convey("floor coverage shrinking vanishes independently (G)", func() {
			// b2 at 99 is below the 99.5 floor, so only b1 covers it.
			surface := reducer.Surface(
				decimal.NewFromFloat64(1000),
				decimal.NewFromFloat64(99.5),
				liquidationFee(),
				time.Now(),
			)

			So(surface.FloorCoverageQty.Cmp(decimal.NewFromFloat64(500)), ShouldEqual, 0)
			So(surface.FullyExecutable, ShouldBeTrue)
		})

		Convey("a crossed update invalidates execution state (H)", func() {
			reducer.Apply(updateL3(
				[]kraken.Level3Order{mustDecimalOrder("b1", "102", "500")},
				[]kraken.Level3Order{mustDecimalOrder("a1", "101", "1000")},
			), 1)

			surface := reducer.Surface(
				decimal.NewFromFloat64(500), nil, liquidationFee(), time.Now(),
			)

			So(surface.BookComplete, ShouldBeFalse)
			So(surface.ExecutableVWAP, ShouldBeNil)
		})
	})
}

/*
TestLiquidationBoundedState covers contract J: state stays bounded by distinct
order identities, never by the stream length, and never grows.
*/
func TestLiquidationBoundedState(t *testing.T) {
	Convey("Given a reducer under a long update stream", t, func() {
		reducer := newLiquidationReducer("EDGE/USD")
		reducer.Apply(snapshotL3(
			[]kraken.Level3Order{mustDecimalOrder("b1", "100", "1000")},
			[]kraken.Level3Order{mustDecimalOrder("a1", "101", "1000")},
		), 1)

		for index := 0; index < 1000; index++ {
			reducer.Apply(updateL3(
				[]kraken.Level3Order{
					mustDecimalOrder("b1", "100", "1000"),
					mustDecimalOrder("b2", "99", "10"),
				},
				[]kraken.Level3Order{mustDecimalOrder("a1", "101", "1000")},
			), 1)
		}

		Convey("resident state is bounded by distinct identities", func() {
			reducer.mu.RLock()
			bidLen := reducer.bidLen
			askLen := reducer.askLen
			bidIdxLen := len(reducer.bidIdx)
			reducer.mu.RUnlock()

			So(bidLen, ShouldEqual, 2)
			So(askLen, ShouldEqual, 1)
			So(bidIdxLen, ShouldEqual, 2)
			So(bidLen, ShouldBeLessThan, 1000)
		})
	})
}

/*
TestDeskLevel3NoLifecycleAllocatesBoundedState covers the continuous-position
distinction: L3 for a symbol with no position still advances bounded execution
state (exactly one resident record), and no position is created.
*/
func TestDeskLevel3NoLifecycleAllocatesBoundedState(t *testing.T) {
	Convey("Given a desk with no position for a symbol", t, func() {
		conn := newExecutingConn(nil)
		desk, workload := newDeliveryDesk(t, conn)
		conn.workload = workload

		Convey("L3 traffic advances the continuous reducer without a position", func() {
			So(
				desk.StepLevel3(coherentL3("NO_POSITION/USD", "2.50", "2.51")),
				ShouldBeNil,
			)

			_, found := desk.positions.Load("NO_POSITION/USD")
			So(found, ShouldBeFalse)

			reducer := desk.executionReducer("NO_POSITION/USD")
			reducer.mu.RLock()
			seeded := reducer.seeded
			reducer.mu.RUnlock()

			So(seeded, ShouldBeTrue)
		})
	})
}

/*
TestLiquidationRace covers contract L: concurrent ingress and surface reads on
one reducer observe coherent committed states without panic.
*/
func TestLiquidationRace(t *testing.T) {
	Convey("Given concurrent ingress and surface reads on one reducer", t, func() {
		reducer := newLiquidationReducer("EDGE/USD")
		reducer.Apply(snapshotL3(
			[]kraken.Level3Order{
				mustDecimalOrder("b1", "100", "1000"),
				mustDecimalOrder("b2", "99", "1000"),
			},
			[]kraken.Level3Order{mustDecimalOrder("a1", "101", "1000")},
		), 1)

		done := make(chan struct{})

		go func() {
			defer close(done)

			for index := 0; index < 1000; index++ {
				reducer.Apply(updateL3(
					[]kraken.Level3Order{
						mustDecimalOrder("b1", "99", "1000"),
						mustDecimalOrder("b2", "100", "1000"),
					},
					[]kraken.Level3Order{mustDecimalOrder("a1", "101", "1000")},
				), 1)
			}
		}()

		Convey("surface reads observe coherent committed states without panic", func() {
			for index := 0; index < 1000; index++ {
				_ = reducer.Surface(
					decimal.NewFromFloat64(1000),
					nil,
					liquidationFee(),
					time.Now(),
				)
			}
		})

		<-done
	})
}

func BenchmarkLiquidationApply(b *testing.B) {
	reducer := newLiquidationReducer("EDGE/USD")
	reducer.Apply(snapshotL3(
		[]kraken.Level3Order{
			mustDecimalOrder("b1", "100", "40000"),
			mustDecimalOrder("b2", "99", "30000"),
			mustDecimalOrder("b3", "98", "30000"),
		},
		[]kraken.Level3Order{mustDecimalOrder("a1", "101", "100000")},
	), 1)

	update := updateL3(
		[]kraken.Level3Order{
			mustDecimalOrder("b2", "98.5", "30000"),
			mustDecimalOrder("b4", "97", "10000"),
		},
		[]kraken.Level3Order{mustDecimalOrder("a1", "101", "100000")},
	)

	b.ReportAllocs()

	for b.Loop() {
		reducer.Apply(update, 1)
	}
}

func BenchmarkLiquidationSurface(b *testing.B) {
	reducer := newLiquidationReducer("EDGE/USD")
	reducer.Apply(snapshotL3(
		[]kraken.Level3Order{
			mustDecimalOrder("b1", "100", "40000"),
			mustDecimalOrder("b2", "99", "30000"),
			mustDecimalOrder("b3", "98", "30000"),
		},
		[]kraken.Level3Order{mustDecimalOrder("a1", "101", "100000")},
	), 1)
	sellable := decimal.NewFromFloat64(100000)
	floor := decimal.NewFromFloat64(51)
	at := time.Now()
	fee := liquidationFee()
	b.ReportAllocs()

	for b.Loop() {
		reducer.Surface(sellable, floor, fee, at)
	}
}
