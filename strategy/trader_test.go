package strategy

import (
	"fmt"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
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

func TestTraderObjective(t *testing.T) {
	testConcurrentObjective(t)

	Convey("A held wallet waits for its book to reseed without inventing feedback", t, func() {
		learner, conn := learningFixture(t)
		tape := market.NewLevel3Tape("BTC/USD", time.Now())

		for _, message := range tape.Messages[:4] {
			conn.ApplyLevel3(message)
		}
		trader := learner.Traders[0]
		member := learner.Agent
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
						surface, err := trader.price.Surface(regulator.Holding.Symbol, regulator.Holding.Qty, trader.At)
						So(err, ShouldBeNil)
						expected = expected.Add(surface.ExecutableValue)
					}
					boundary := &objectiveConn{Conn: conn, entered: make(chan string, 8), release: make(chan struct{})}
					workers := len(trader.Positions)

					if missing {
						boundary.missing = "ASSET0/USD"
						workers--
					}
					trader.price.Books = boundary
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

func TestTraderFeasible(t *testing.T) {
	Convey("Only owned resources and venue rules constrain choices", t, func() {
		learner, conn := learningFixture(t)
		for _, message := range market.NewLevel3Tape("BTC/USD", time.Now()).Messages[:4] {
			conn.ApplyLevel3(message)
		}
		trader := extraTrader(t, learner)
		trader.At = time.Now()
		Convey("Ten other positions do not prohibit another entry", func() {
			for index := range 10 {
				symbol := fmt.Sprintf("HELD%d/USD", index)
				regulator := position.NewRegulator(trader.api, trader.price, symbol, nil)
				regulator.Holding.Qty = venue.Decimal("1")
				trader.Positions[symbol] = regulator
			}
			actions, _, err := trader.Feasible("BTC/USD")
			So(err, ShouldBeNil)
			So(actions, ShouldContain, Action{Kind: "enter"})
		})
		Convey("A position may develop beyond twenty minutes without forced liquidation", func() {
			_, _, err := trader.Feasible("BTC/USD")
			So(err, ShouldBeNil)
			So(trader.Execute(&agent.Decision[Action]{ID: 1, Label: "BTC/USD", At: trader.At, Action: Action{Kind: "enter", Power: 1}}), ShouldBeNil)
			quantity := trader.Positions["BTC/USD"].Holding.Qty
			trader.At = trader.At.Add(21 * time.Minute)
			actions, state, err := trader.Feasible("BTC/USD")
			So(err, ShouldBeNil)
			So(actions, ShouldContain, Action{Kind: "hold"})
			So(actions, ShouldContain, Action{Kind: "scale"})
			So(actions, ShouldContain, Action{Kind: "exit", Reduce: true})
			So(state, ShouldResemble, []uint64{FlatPositionContext + 1})
			So(trader.Positions["BTC/USD"].Holding.Qty.Cmp(quantity), ShouldEqual, 0)
		})
		Convey("Missing book does not manufacture wait evidence", func() {
			conn.ApplyLevel3(kraken.Level3Data{Type: "snapshot", Symbol: "BTC/USD"})
			So(trader.Ready("BTC/USD"), ShouldBeFalse)
			actions, _, err := trader.Feasible("BTC/USD")
			So(err, ShouldNotBeNil)
			So(actions, ShouldBeNil)
		})
	})
}

func TestTraderEnd(t *testing.T) {
	Convey("Evaluation ends the owned position using a real executable book", t, func() {
		learner, connection := learningFixture(t)
		tape := market.NewLevel3Tape("BTC/USD", time.Now())
		for _, message := range tape.Messages[:4] {
			connection.ApplyLevel3(message)
		}
		trader := extraTrader(t, learner)
		trader.At = time.Now()
		_, _, err := trader.Feasible("BTC/USD")
		So(err, ShouldBeNil)
		So(trader.Execute(&agent.Decision[Action]{ID: 1, Label: "BTC/USD", At: trader.At,
			Action: Action{Kind: "enter", Power: 1}}), ShouldBeNil)
		quantity := trader.Positions["BTC/USD"].Holding.Qty
		through := trader.At.Add(time.Second)

		Convey("An older completed opportunity cannot close a newer position", func() {
			So(trader.End("BTC/USD", trader.At.Add(-time.Second)), ShouldBeNil)
			So(trader.Positions["BTC/USD"].Holding.Qty.Cmp(quantity), ShouldEqual, 0)
		})
		Convey("Missing book defers release without a fill or a learning decision", func() {
			connection.ApplyLevel3(kraken.Level3Data{Type: "snapshot", Symbol: "BTC/USD"})
			So(trader.End("BTC/USD", through), ShouldBeNil)
			So(trader.Ending["BTC/USD"], ShouldEqual, through)
			So(trader.Fills, ShouldEqual, 1)
			for _, message := range tape.Messages[:4] {
				connection.ApplyLevel3(message)
			}
			So(trader.End("BTC/USD", through), ShouldBeNil)
			So(trader.Positions["BTC/USD"].Holding.Qty.Sign(), ShouldEqual, 0)
			So(trader.Ending, ShouldBeEmpty)
			So(trader.Fills, ShouldEqual, 2)
			So(len(trader.Evaluations["BTC/USD"]), ShouldEqual, 1)
			So(trader.End("BTC/USD", through), ShouldBeNil)
			So(trader.Fills, ShouldEqual, 2)
		})
	})
}
