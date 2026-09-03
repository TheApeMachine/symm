package broker

import (
	"strconv"
	"sync"
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
		reducer := newLiquidationReducer("EDGE/USD", 10)
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
		reducer := newLiquidationReducer("EDGE/USD", 10)

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
		reducer := newLiquidationReducer("EDGE/USD", 10)
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
		reducer := newLiquidationReducer("EDGE/USD", 10)
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
		reducer := newLiquidationReducer("EDGE/USD", 10)
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

			reducer, _ := desk.executionReducer("NO_POSITION/USD")
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
		reducer := newLiquidationReducer("EDGE/USD", 10)
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
	reducer := newLiquidationReducer("EDGE/USD", 10)
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
	reducer := newLiquidationReducer("EDGE/USD", 10)
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

/*
TestLiquidationDepthIsPriceLevels proves the L3 depth contract: depth bounds
the number of distinct PRICE LEVELS per side, not the number of individual
orders. A depth-2 reducer retains many orders spread across two levels, and
admits a third level by evicting the worst.
*/
func TestLiquidationDepthIsPriceLevels(t *testing.T) {
	Convey("Given a depth-2 reducer", t, func() {
		reducer := newLiquidationReducer("EDGE/USD", 2)

		Convey("many orders within <=2 price levels stay valid", func() {
			reducer.Apply(snapshotL3(
				[]kraken.Level3Order{
					mustDecimalOrder("b1", "100", "100"),
					mustDecimalOrder("b2", "100", "100"),
					mustDecimalOrder("b3", "100", "100"),
					mustDecimalOrder("b4", "99", "100"),
				},
				[]kraken.Level3Order{mustDecimalOrder("a1", "101", "400")},
			), 1)

			reducer.mu.RLock()
			valid := reducer.valid
			bidLen := reducer.bidLen
			reducer.mu.RUnlock()

			So(valid, ShouldBeTrue)
			So(bidLen, ShouldEqual, 4)
		})

		Convey("a third distinct level evicts the full worst level", func() {
			reducer.Apply(snapshotL3(
				[]kraken.Level3Order{
					mustDecimalOrder("b1", "100", "100"),
					mustDecimalOrder("b2", "99", "100"),
					mustDecimalOrder("b3", "99", "100"),
					mustDecimalOrder("b4", "98", "300"),
				},
				[]kraken.Level3Order{mustDecimalOrder("a1", "101", "600")},
			), 1)

			// 98 is level 3 and must be evicted with all its orders.
			surface := reducer.Surface(
				decimal.NewFromFloat64(300), nil, liquidationFee(), time.Now(),
			)

			So(surface.ExecutableQty.Cmp(decimal.NewFromFloat64(300)), ShouldEqual, 0)
			So(surface.FullyExecutable, ShouldBeTrue)

			reducer.mu.RLock()
			_, found98 := reducer.bidIdx["b4"]
			_, found99 := reducer.bidIdx["b2"]
			bidLen := reducer.bidLen
			reducer.mu.RUnlock()

			So(found98, ShouldBeFalse)
			So(found99, ShouldBeTrue)
			So(bidLen, ShouldEqual, 3)
		})

		Convey("a level forced out by a better new level can be admitted again", func() {
			reducer.Apply(snapshotL3(
				[]kraken.Level3Order{
					mustDecimalOrder("b1", "100", "100"),
					mustDecimalOrder("b2", "99", "100"),
				},
				[]kraken.Level3Order{mustDecimalOrder("a1", "101", "300")},
			), 1)

			// b3 at 101 pushes level 99 out of the top-2 window.
			reducer.Apply(updateL3(
				[]kraken.Level3Order{mustDecimalOrder("b3", "101", "100")},
				nil,
			), 1)

			reducer.mu.RLock()
			_, level99Out := reducer.bidIdx["b2"]
			reducer.mu.RUnlock()
			So(level99Out, ShouldBeFalse)

			// The better level leaves the window, so 99 is again within the
			// top-2 subscription. The venue may then re-add the 99 order; it
			// must be admitted again as a fresh identity.
			reducer.Apply(updateL3(
				[]kraken.Level3Order{{Event: "delete", OrderID: "b3"}},
				nil,
			), 1)
			reducer.Apply(updateL3(
				[]kraken.Level3Order{mustDecimalOrder("b2", "99", "100")},
				nil,
			), 1)

			reducer.mu.RLock()
			_, level99Back := reducer.bidIdx["b2"]
			reducer.mu.RUnlock()
			So(level99Back, ShouldBeTrue)
		})
	})
}

/*
TestLiquidationEvictionRemovesStaleLiquidity proves evicted out-of-window
orders never leak into ExecutableQty, FloorCoverageQty, ExecutableVWAP, or
ExecutableValue.
*/
func TestLiquidationEvictionRemovesStaleLiquidity(t *testing.T) {
	Convey("Given a depth-1 reducer seeded with two levels", t, func() {
		reducer := newLiquidationReducer("EDGE/USD", 1)
		reducer.Apply(snapshotL3(
			[]kraken.Level3Order{
				mustDecimalOrder("b1", "100", "100"),
				mustDecimalOrder("b2", "99", "5000"),
			},
			[]kraken.Level3Order{mustDecimalOrder("a1", "101", "6000")},
		), 1)

		Convey("the out-of-window level contributes nothing to the surface", func() {
			surface := reducer.Surface(
				decimal.NewFromFloat64(1000),
				decimal.NewFromFloat64(99),
				liquidationFee(),
				time.Now(),
			)

			So(surface.ExecutableQty.Cmp(decimal.NewFromFloat64(100)), ShouldEqual, 0)
			So(surface.FloorCoverageQty.Cmp(decimal.NewFromFloat64(100)), ShouldEqual, 0)
			So(surface.FullyExecutable, ShouldBeFalse)
			So(surface.ExecutableVWAP, ShouldBeNil)
			So(surface.ExecutableValue, ShouldBeNil)
		})
	})
}

/*
TestLiquidationModifyAtCapacity proves a modify to an already-resident order is
applied even when the side's identity storage is otherwise full, while a
genuinely new order at capacity triggers virtual aggregation to preserve execution
validity and top-of-book geometry.
*/
func TestLiquidationModifyAtCapacity(t *testing.T) {
	Convey("Given a reducer with depth large enough to hold two levels", t, func() {
		reducer := newLiquidationReducer("EDGE/USD", 2)
		// Fill the bid identity capacity to the maxResidentOrdersPerSide.
		reducer.mu.Lock()
		reducer.bids = make([]liquidationOrder, 0, maxResidentOrdersPerSide)
		for index := 0; index < maxResidentOrdersPerSide; index++ {
			orderID := "fill-" + strconv.Itoa(index)
			price := 100 - index
			reducer.bids = append(reducer.bids, liquidationOrder{
				orderID:    orderID,
				limitPrice: mustDecimal(strconv.Itoa(price)),
				orderQty:   mustDecimal("1"),
			})
			reducer.bidIdx[orderID] = index
		}
		reducer.bidLen = maxResidentOrdersPerSide
		reducer.asks = append(reducer.asks, liquidationOrder{
			orderID:    "a1",
			limitPrice: mustDecimal("999"),
			orderQty:   mustDecimal("1"),
		})
		reducer.askIdx["a1"] = 0
		reducer.askLen = 1
		reducer.seeded = true
		reducer.valid = true
		reducer.mu.Unlock()

		Convey("a modify to the resident order is applied", func() {
			reducer.Apply(updateL3(
				[]kraken.Level3Order{mustDecimalOrder("fill-0", "200", "1000")},
				nil,
			), 0)

			reducer.mu.RLock()
			modifiedPrice := reducer.bids[reducer.bidIdx["fill-0"]].limitPrice
			overflow := reducer.overflow
			reducer.mu.RUnlock()

			So(modifiedPrice.Cmp(mustDecimal("200")), ShouldEqual, 0)
			So(overflow, ShouldBeFalse)
		})

		Convey("a genuinely new order at capacity triggers virtual aggregation and preserves execution validity", func() {
			reducer.Apply(updateL3(
				[]kraken.Level3Order{mustDecimalOrder("brand-new", "50", "1")},
				nil,
			), 0)

			reducer.mu.RLock()
			overflow := reducer.overflow
			valid := reducer.valid
			bidLen := reducer.bidLen
			bestBid := reducer.bids[0].limitPrice
			reducer.mu.RUnlock()

			So(overflow, ShouldBeFalse)
			So(valid, ShouldBeTrue)
			So(bidLen, ShouldBeLessThan, maxResidentOrdersPerSide)
			So(bestBid.Cmp(mustDecimal("100")), ShouldEqual, 0)

			surface := reducer.Surface(
				decimal.NewFromFloat64(1), nil, liquidationFee(), time.Now(),
			)
			So(surface.BookComplete, ShouldBeTrue)
			So(surface.FullyExecutable, ShouldBeTrue)
		})
	})
}

/*
TestExecutionReducerCreateOnce proves exactly one resident reducer exists per
symbol even when ingress and position construction race.
*/
func TestExecutionReducerCreateOnce(t *testing.T) {
	Convey("Given a desk constructed without any L3 traffic", t, func() {
		conn := newExecutingConn(nil)
		desk, workload := newDeliveryDesk(t, conn)
		conn.workload = workload

		Convey("concurrent ingress and position construction obtain one reducer", func() {
			const goroutines = 8
			results := make(chan *liquidationReducer, goroutines*2)
			start := make(chan struct{})
			var waited sync.WaitGroup
			waited.Add(goroutines * 2)

			for index := 0; index < goroutines; index++ {
				go func() {
					defer waited.Done()
					<-start
					reducer, err := desk.executionReducer("RACE/USD")
					if err != nil {
						results <- nil
						return
					}
					results <- reducer
				}()
				go func() {
					defer waited.Done()
					<-start
					_ = desk.StepLevel3(coherentL3("RACE/USD", "2.50", "2.51"))
					reducer, _ := desk.executionReducer("RACE/USD")
					results <- reducer
				}()
			}
			close(start)
			waited.Wait()
			close(results)

			// Every caller already completed before the assertions below run,
			// so no goroutine mutates the reducer while it is inspected.
			first := <-results
			So(first, ShouldNotBeNil)

			for winner := range results {
				So(winner == first, ShouldBeTrue)
			}
		})
	})
}
