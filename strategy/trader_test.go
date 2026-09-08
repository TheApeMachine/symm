package strategy

import (
	"fmt"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/kraken/websocket"
	"testing"
	"testing/synctest"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
	"github.com/theapemachine/symm/nomagique/learning/associative/reward"
	"github.com/theapemachine/symm/tests/market"
	"github.com/theapemachine/symm/tests/venue"
)

// extraTrader builds an independent funded wallet outside the consolidated
// learner. Explorer-specific tests use it directly instead of reaching for a
// second live member that the forward-canary learner no longer creates.
func extraTrader(t testing.TB, learner *Learner) *Trader {
	t.Helper()

	trader, err := NewTrader(
		learner.Traders[0].api,
		learner.price,
		learner.Traders[0].Balance.Quote,
		learner.Traders[0].Initial,
	)

	if err != nil {
		t.Fatal(err)
	}
	trader.ID, trader.Recorder = 1, learner.recorder

	return trader
}

func TestTraderExecute(t *testing.T) {
	Convey("Independent accounts use the real regulator for entry, reduction and exit", t, func() {
		learner, conn := learningFixture(t)
		tape := market.NewLevel3Tape("BTC/USD", time.Now())

		for _, message := range tape.Messages[:4] {
			conn.ApplyLevel3(message)
		}
		trader := extraTrader(t, learner)
		trader.At, trader.Version = time.Now(), 1
		actions, _, err := trader.Feasible("BTC/USD")
		So(err, ShouldBeNil)
		So(len(actions), ShouldBeGreaterThan, 1)
		chosen := Action{Kind: "enter", Power: 1}
		So(trader.Quantities[chosen], ShouldNotBeNil)
		So(trader.Execute(&agent.Decision[Action]{ID: 1, Label: "BTC/USD", At: trader.At, Action: chosen}), ShouldBeNil)
		position := trader.Positions["BTC/USD"]
		So(position.Guardian, ShouldBeNil) // Synchronous fills need no background event listener.
		originalQuantity := position.Holding.Qty
		So(originalQuantity.Sign(), ShouldEqual, 1)
		So(trader.Balance.Cash().Cmp(trader.Initial), ShouldBeLessThan, 0)
		So(learner.Traders[0].Balance.Cash().Cmp(learner.Traders[0].Initial), ShouldEqual, 0)
		_, err = trader.Objective()
		So(err, ShouldBeNil)
		So(trader.Wealth, ShouldBeLessThan, 0)

		_, _, err = trader.Feasible("BTC/USD")
		So(err, ShouldBeNil)
		So(trader.Execute(&agent.Decision[Action]{ID: 2, Label: "BTC/USD", At: trader.At, Action: Action{Kind: "scale", Power: 1, Reduce: true}}), ShouldBeNil)
		So(position.Holding.Qty.Sign(), ShouldEqual, 1)
		So(position.Holding.Qty.Cmp(originalQuantity), ShouldBeLessThan, 0)

		_, _, err = trader.Feasible("BTC/USD")
		So(err, ShouldBeNil)
		So(trader.Execute(&agent.Decision[Action]{ID: 3, Label: "BTC/USD", At: trader.At, Action: Action{Kind: "exit", Reduce: true}}), ShouldBeNil)
		So(position.Holding.Qty.Sign(), ShouldEqual, 0)
		So(position.Holding.Basis.Sign(), ShouldEqual, 0)
		So(position.Holding.EntryFee.Sign(), ShouldEqual, 0)
		So(trader.Fills, ShouldEqual, 3)
		So(trader.Balance.Cash().Sub(trader.Initial).Cmp(position.Holding.RealizedPnL), ShouldEqual, 0)
	})
}

func TestTraderCheckAndReset(t *testing.T) {
	Convey("An exhausted explorer wallet is renewed into a new episode", t, func() {
		learner, conn := learningFixture(t)
		tape := market.NewLevel3Tape("BTC/USD", time.Now())

		for _, message := range tape.Messages[:4] {
			conn.ApplyLevel3(message)
		}
		trader := extraTrader(t, learner)
		trader.At, trader.Version = time.Now(), 1
		funding := trader.Initial

		Convey("A funded wallet is left alone", func() {
			So(trader.CheckAndReset(funding), ShouldBeFalse)
			So(trader.Episode, ShouldEqual, 0)
		})

		Convey("Given a position opened with the whole account", func() {
			_, _, err := trader.Feasible("BTC/USD")
			So(err, ShouldBeNil)
			So(trader.Execute(&agent.Decision[Action]{
				ID: 1, Label: "BTC/USD", At: trader.At, Action: Action{Kind: "enter", Power: 1},
			}), ShouldBeNil)
			_, err = trader.Objective()
			So(err, ShouldBeNil)
			So(trader.Wealth, ShouldBeLessThan, 0)

			// Drain the remaining cash so no feasible order size is left.
			So(trader.Balance.Settle(kraken.ExecutionData{
				Side: "buy", Timestamp: trader.At,
				CumCost: trader.Balance.Cash(), FeeUsdEquiv: decimal.NewFromInt64(0),
			}), ShouldBeNil)
			So(trader.Balance.Cash().Sign(), ShouldEqual, 0)
			holding := trader.Positions["BTC/USD"].Holding
			So(holding.Qty.Sign(), ShouldEqual, 1)

			Convey("Inventory still inside its holding time defers the renewal", func() {
				So(trader.CheckAndReset(funding), ShouldBeFalse)
				So(trader.Episode, ShouldEqual, 0)
				So(holding.Qty.Sign(), ShouldEqual, 1)
			})

			Convey("Inventory past its holding time is liquidated and the wallet refunded", func() {
				// Opened, not Holding.EntryAt, is the clock: EntryAt survives a
				// close and would age a position that no longer exists.
				trader.Opened["BTC/USD"] = trader.At.Add(-MaximumHoldingTime - time.Second)
				fills := trader.Fills

				So(trader.CheckAndReset(funding), ShouldBeTrue)
				So(trader.Fills, ShouldEqual, fills+1)
				So(holding.Qty.Sign(), ShouldEqual, 0)
				So(trader.Episode, ShouldEqual, 1)
				So(trader.Status, ShouldEqual, "replenished")
				So(trader.Balance.Cash().Cmp(funding), ShouldEqual, 0)
				So(trader.Positions, ShouldBeEmpty)
				So(trader.Wealth, ShouldEqual, 0)

				Convey("The objective stays continuous across the renewal", func() {
					So(trader.Carried, ShouldBeLessThan, 0)
					mark, err := trader.Objective()
					So(err, ShouldBeNil)
					So(mark.Value, ShouldEqual, trader.Carried)

					Convey("And the renewed wallet trades again", func() {
						actions, _, err := trader.Feasible("BTC/USD")
						So(err, ShouldBeNil)
						So(len(actions), ShouldBeGreaterThan, 1)
						So(trader.Execution.Balance, ShouldEqual, trader.Balance)
					})
				})
			})
		})
	})
}

func TestTraderObjective(t *testing.T) {
	testConcurrentObjective(t)

	Convey("A held wallet waits for its book to reseed without inventing feedback", t, func() {
		learner, conn := learningFixture(t)
		tape := market.NewLevel3Tape("BTC/USD", time.Now())

		for _, message := range tape.Messages[:4] {
			conn.ApplyLevel3(message)
		}
		trader := learner.Traders[0]
		member := learner.Population.Agents[0]
		trader.At, trader.Version = time.Now(), 1
		_, _, err := trader.Feasible("BTC/USD")
		So(err, ShouldBeNil)
		So(trader.Execute(&agent.Decision[Action]{
			ID: 1, Label: "BTC/USD", At: trader.At,
			Action: Action{Kind: "enter", Power: 1},
		}), ShouldBeNil)
		So(member.Measure(), ShouldBeNil)
		previous := member.Reward
		equity := trader.Equity

		conn.ApplyLevel3(kraken.Level3Data{Type: "snapshot", Symbol: "BTC/USD"})
		trader.At = trader.At.Add(time.Second)
		trader.Version++
		mark, err := trader.Objective()
		So(err, ShouldBeNil)
		So(mark, ShouldBeNil)
		So(trader.Status, ShouldEqual, "awaiting book: BTC/USD")
		So(member.Measure(), ShouldBeNil)
		So(member.Reward, ShouldResemble, previous)
		So(trader.Equity, ShouldEqual, equity)

		for _, message := range tape.Messages[:4] {
			conn.ApplyLevel3(message)
		}
		trader.At = trader.At.Add(time.Second)
		trader.Version++
		So(member.Measure(), ShouldBeNil)
		So(member.Reward.Through.Version, ShouldEqual, trader.Version)
		So(member.Reward.Elapsed, ShouldEqual, 2*time.Second)
		So(member.Reward.Transitions, ShouldEqual, 1)
		So(trader.Status, ShouldEqual, "learning")
	})
}

// objectiveConn gates resident-book access, preserving the real book and pricing path.
type objectiveConn struct {
	*venue.Conn
	entered chan string
	release chan struct{}
	missing string
}

func (conn *objectiveConn) Book(symbol string, read func(*spotbook.Book)) {
	if symbol == conn.missing {
		read(nil)
		return
	}
	conn.entered <- symbol
	<-conn.release
	conn.Conn.Book(symbol, read)
}

func testConcurrentObjective(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing=%t", missing), func(t *testing.T) {
			// The fixture is built outside the bubble on purpose. Funding a
			// position records it, and recording now writes Parquet through
			// the Iceberg writer's own goroutines, which a synctest bubble
			// cannot own. Only the concurrent mark below is under fake time.
			trader, conn := fundedPositions(t, 4)

			synctest.Test(t, func(t *testing.T) {
				Convey("Distinct positions mark concurrently before committing wallet totals", t, func() {
					expected := trader.Balance.Cash()

					for _, regulator := range trader.Positions {
						surface, err := trader.Execution.Price.Surface(regulator.Holding.Symbol, regulator.Holding.Qty, trader.At)
						So(err, ShouldBeNil)
						expected = expected.Add(surface.ExecutableValue)
					}
					boundary := &objectiveConn{Conn: conn, entered: make(chan string, 8), release: make(chan struct{})}
					workers := len(trader.Positions)

					if missing {
						boundary.missing = "ASSET0/USD"
						workers--
					}
					trader.api = websocket.NewAPI(t.Context(), boundary, boundary)
					previous := trader.Equity
					done := make(chan error, 1)
					var mark *reward.Mark
					go func() {
						var err error
						mark, err = trader.Objective()
						done <- err
					}()
					synctest.Wait()
					started, returned := len(boundary.entered), len(done)
					unchanged := trader.Equity == previous
					close(boundary.release)
					So(<-done, ShouldBeNil)
					So(started, ShouldEqual, workers)
					So(returned, ShouldEqual, 0)
					So(unchanged, ShouldBeTrue)

					if missing {
						So(mark, ShouldBeNil)
						So(trader.Status, ShouldEqual, "awaiting book: ASSET0/USD")
						So(trader.Equity, ShouldEqual, previous)
						return
					}
					So(mark, ShouldNotBeNil)
					So(trader.Equity.Cmp(expected), ShouldEqual, 0)
					So(trader.Profit.Cmp(expected.Sub(trader.Initial)), ShouldEqual, 0)
					So(trader.Unrealized.Cmp(trader.Profit.Sub(trader.Realized)), ShouldEqual, 0)
				})
			})
		})
	}
}

// fundedPositions opens real simulated fills across distinct multi-leg books.
func fundedPositions(t testing.TB, count int) (*Trader, *venue.Conn) {
	t.Helper()
	learner, conn := learningFixture(t)
	trader := learner.Traders[0]
	trader.At, trader.Version = time.Now(), 1

	for index := range count {
		symbol := fmt.Sprintf("ASSET%d/USD", index)
		trader.api.Normalizer().Update(&spot.AssetsManagerUpdate{
			NewPairs: map[string]spot.AssetPair{symbol: {
				WSName: symbol, Base: fmt.Sprintf("ASSET%d", index), Quote: "USD",
				PairDecimals: 2, LotDecimals: 8, LotMultiplier: 1,
			}},
		})
		learner.price.SetFee(symbol, kraken.TradeVolumeFee{Fee: venue.Decimal("0.25")})

		for _, message := range market.NewLevel3Tape(symbol, trader.At).Messages[:4] {
			conn.ApplyLevel3(message)
		}
		// A quarter unit per symbol keeps the four-position fixture within $200.
		chosen := Action{Kind: "enter"}
		trader.Alternatives = []Action{chosen, {Kind: "wait"}}
		trader.Quantities = map[Action]*decimal.Decimal{chosen: venue.Decimal("0.25")}

		if err := trader.Execute(&agent.Decision[Action]{
			ID: uint64(index + 1), Label: symbol, At: trader.At, Action: chosen,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return trader, conn
}

func BenchmarkTraderObjective(b *testing.B) {
	for _, count := range []int{1, 4} {
		b.Run(fmt.Sprintf("positions=%d", count), func(b *testing.B) {
			trader, _ := fundedPositions(b, count)
			b.ReportAllocs()

			for b.Loop() {
				mark, err := trader.Objective()

				if err != nil || mark == nil {
					b.Fatalf("objective mark=%v err=%v", mark, err)
				}
			}
		})
	}
}

func TestTraderPositionCapacity(t *testing.T) {
	Convey("A wallet may only open as many symbols as it can sustain", t, func() {
		learner, conn := learningFixture(t)

		for _, message := range market.NewLevel3Tape("BTC/USD", time.Now()).Messages[:4] {
			conn.ApplyLevel3(message)
		}
		trader := extraTrader(t, learner)
		trader.At, trader.Version = time.Now(), 1

		kinds := func(symbol string) map[string]bool {
			actions, _, err := trader.Feasible(symbol)
			So(err, ShouldBeNil)
			found := map[string]bool{}

			for _, action := range actions {
				found[action.Kind] = true
			}

			return found
		}
		So(kinds("BTC/USD")["enter"], ShouldBeTrue)

		// Stand in for positions the wallet already carries in other symbols.
		for index := range MaxConcurrentPositions {
			symbol := fmt.Sprintf("HELD%d/USD", index)
			regulator := position.NewRegulator(trader.api, trader.Execution.Price, symbol, nil)
			regulator.Holding.Qty = venue.Decimal("1")
			trader.Positions[symbol] = regulator
		}
		So(trader.open(), ShouldEqual, MaxConcurrentPositions)

		Convey("A new symbol is no longer enterable", func() {
			available := kinds("BTC/USD")
			So(available["enter"], ShouldBeFalse)
			So(available["wait"], ShouldBeTrue)
			So(trader.Status, ShouldEqual, "position capacity")
		})

		Convey("Closing one frees the capacity again", func() {
			trader.Positions["HELD0/USD"].Holding.Qty = venue.Decimal("0")
			So(trader.open(), ShouldEqual, MaxConcurrentPositions-1)
			So(kinds("BTC/USD")["enter"], ShouldBeTrue)
		})
	})
}

func TestTraderStalePosition(t *testing.T) {
	Convey("A position that stopped developing loses the option to sit on it", t, func() {
		learner, conn := learningFixture(t)

		for _, message := range market.NewLevel3Tape("BTC/USD", time.Now()).Messages[:4] {
			conn.ApplyLevel3(message)
		}
		trader := extraTrader(t, learner)
		trader.At, trader.Version = time.Now(), 1
		_, _, err := trader.Feasible("BTC/USD")
		So(err, ShouldBeNil)
		So(trader.Execute(&agent.Decision[Action]{
			ID: 1, Label: "BTC/USD", At: trader.At, Action: Action{Kind: "enter", Power: 1},
		}), ShouldBeNil)

		opened, tracked := trader.Opened["BTC/USD"]
		So(tracked, ShouldBeTrue)

		Convey("A fresh position may still be held", func() {
			actions, state, err := trader.Feasible("BTC/USD")
			So(err, ShouldBeNil)
			So(actions[0].Kind, ShouldEqual, "hold")
			So(state[0], ShouldEqual, uint64(1)<<63|1)
		})

		Convey("Past the horizon only capital-returning moves remain", func() {
			trader.At = opened.Add(StalePositionHorizon)
			actions, state, err := trader.Feasible("BTC/USD")
			So(err, ShouldBeNil)
			So(trader.Status, ShouldEqual, "stale position")
			So(len(actions), ShouldBeGreaterThan, 0)

			for _, action := range actions {
				So(action.Reduce, ShouldBeTrue)
				So(action.Kind, ShouldNotEqual, "hold")
			}
			So(state[0], ShouldEqual, uint64(1)<<63|3) // held and stale

			Convey("Exiting clears the age so a re-entry starts fresh", func() {
				So(trader.Execute(&agent.Decision[Action]{
					ID: 2, Label: "BTC/USD", At: trader.At, Action: actions[0],
				}), ShouldBeNil)
				So(trader.Positions["BTC/USD"].Holding.Qty.Sign(), ShouldEqual, 0)
				_, tracked := trader.Opened["BTC/USD"]
				So(tracked, ShouldBeFalse)
				So(trader.stale("BTC/USD"), ShouldBeFalse)
			})
		})
	})
}
