package execution

import (
	"context"
	"fmt"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/nomagique/financial/paper"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	marketfixture "github.com/theapemachine/symm/tests/market"
)

/* quotedTerms is a venue boundary fixture: 0.8 percent, 0.001 base, 0.01 quote. */
type quotedTerms struct{}

func (quotedTerms) Write(context.Context, kraken.Terms_write) error { return nil }
func (quotedTerms) Done(context.Context, kraken.Terms_done) error   { return nil }
func (quotedTerms) Balance(ctx context.Context, call kraken.Terms_balance) error {
	result, err := call.AllocResults()
	if err != nil {
		return err
	}
	return result.SetAvailable("200")
}
func (quotedTerms) Nonce(ctx context.Context, call kraken.Terms_nonce) error {
	return fmt.Errorf("paper fixture does not submit live orders")
}
func (quotedTerms) Normalize(ctx context.Context, call kraken.Terms_normalize) error {
	return fmt.Errorf("paper fixture does not submit live orders")
}
func (quotedTerms) Quote(ctx context.Context, call kraken.Terms_quote) error {
	result, err := call.AllocResults()

	if err != nil {
		return err
	}
	terms, err := result.NewTerms()

	if err != nil {
		return err
	}
	symbol, err := call.Args().Symbol()

	if err != nil {
		return err
	}
	terms.SetCostPlaces(8)
	for _, err := range []error{terms.SetSymbol(symbol), terms.SetTakerFee("0.008"), terms.SetMinimumQuantity("0.001"), terms.SetMinimumCost("0.01"), terms.SetQuantityIncrement("0.0001")} {
		if err != nil {
			return err
		}
	}
	return nil
}

/* accountReplay owns the real native book, fill and account nodes for a test. */
type accountReplay struct {
	live, authorized, durable bool
	orders                    kraken.Orders
	checkpoint                runtime.Checkpoint

	ctx         context.Context
	account     Account
	book        paper.Book
	sweep       paper.Sweep
	terms       kraken.Terms
	sequence    int64
	bidQuantity string
	askQuantity string
}

func newAccountReplay(t testing.TB) *accountReplay {
	replay := &accountReplay{ctx: context.Background(), account: Account_ServerToClient(NewAccount()), book: paper.Book_ServerToClient(paper.NewBook(context.Background())), sweep: paper.Sweep_ServerToClient(paper.NewSweep(context.Background())), terms: kraken.Terms_ServerToClient(quotedTerms{})}
	t.Cleanup(func() {
		replay.account.Release()
		replay.book.Release()
		replay.sweep.Release()
		replay.terms.Release()
		replay.orders.Release()
		replay.checkpoint.Release()
	})
	return replay
}

func (replay *accountReplay) step(symbol string, bid, ask float64, updated bool, flat, held string) (AccountState, func(), error) {
	frame := []byte(fmt.Sprintf(`{"channel":"trade","data":{"symbol":%q,"side":"buy","price":%g,"qty":0.001}}`, symbol, ask))

	if updated {
		bidQuantity, askQuantity := replay.bidQuantity, replay.askQuantity
		if bidQuantity == "" {
			bidQuantity = "2"
		}
		if askQuantity == "" {
			askQuantity = "2"
		}
		frame = marketfixture.Level3Frame("snapshot", symbol, [2][]marketfixture.Order{}, [2][]marketfixture.Order{
			{{Price: fmt.Sprint(bid), Quantity: bidQuantity, At: "2026-09-26T10:00:00Z"}},
			{{Price: fmt.Sprint(ask), Quantity: askQuantity, At: "2026-09-26T10:00:00Z"}},
		}, "")
	}

	if err := replay.book.Write(replay.ctx, func(params paper.Book_write_Params) error {
		params.SetDepth(10)
		frames, err := params.NewFrame(1)

		if err != nil {
			return err
		}
		return frames.Set(0, frame)
	}); err != nil {
		return AccountState{}, func() {}, err
	}

	if err := replay.book.WaitStreaming(); err != nil {
		return AccountState{}, func() {}, err
	}
	future, releaseBook := replay.book.Done(replay.ctx, nil)
	defer releaseBook()
	bookResult, err := future.Struct()

	if err != nil {
		return AccountState{}, func() {}, err
	}
	market, err := bookResult.Market()

	if err != nil {
		return AccountState{}, func() {}, err
	}
	err = replay.account.Write(replay.ctx, func(params Account_write_Params) error {
		params.SetEpoch(1)
		params.SetSequence(replay.sequence)
		params.SetTermsReady(true)
		params.SetSweepReady(true)
		params.SetLive(replay.live)
		params.SetAuthorized(replay.authorized)
		params.SetDurable(replay.durable)
		params.SetOrdersReady(replay.orders.IsValid())
		if replay.orders.IsValid() {
			if err := params.SetOrders(replay.orders.AddRef()); err != nil {
				return err
			}
		}
		if replay.checkpoint.IsValid() {
			if err := params.SetCheckpoint(replay.checkpoint.AddRef()); err != nil {
				return err
			}
			if err := params.SetCheckpointKey("live"); err != nil {
				return err
			}
		}

		for _, err := range []error{params.SetInitialCash("200"), params.SetTime("2026-09-26T10:00:00Z"), params.SetMarket(market), params.SetTerms(replay.terms.AddRef()), params.SetSweep(replay.sweep.AddRef())} {
			if err != nil {
				return err
			}
		}
		for _, choice := range []struct {
			action string
			alloc  func(int32) (capnp.DataList, error)
		}{{flat, params.NewFlat}, {held, params.NewHeld}} {
			if choice.action == "" {
				continue
			}
			values, err := choice.alloc(1)

			if err != nil {
				return err
			}

			if err := values.Set(0, []byte(choice.action)); err != nil {
				return err
			}
		}
		return nil
	})
	replay.sequence++

	if err != nil {
		return AccountState{}, func() {}, err
	}

	if err := replay.account.WaitStreaming(); err != nil {
		return AccountState{}, func() {}, err
	}
	completed, release := replay.account.Done(replay.ctx, nil)
	state, err := completed.Struct()
	return state, release, err
}

func TestAccountWrite(t *testing.T) {
	Convey("One native paper wallet executes causal model decisions", t, func() {
		replay := newAccountReplay(t)
		state, release, err := replay.step("BTC/USD", 99, 100, true, "ENTER", "")
		So(err, ShouldBeNil)
		cash, err := state.Cash()
		So(err, ShouldBeNil)
		So(cash, ShouldEqualMoney, "199.8992")
		So(state.Open(), ShouldEqual, 0)
		So(state.Outcomes(), ShouldEqual, 0)
		release()

		Convey("A duplicate capture stamp cannot consume a decision twice", func() {
			replay.sequence = 0
			_, release, err := replay.step("BTC/USD", 99, 100, true, "ENTER", "WAIT")
			defer release()
			So(err, ShouldNotBeNil)
		})

		Convey("A trade observation cannot fill against the unchanged book", func() {
			state, release, err := replay.step("BTC/USD", 99, 100, false, "ENTER", "")
			defer release()
			So(err, ShouldBeNil)
			So(state.Open(), ShouldEqual, 0)
		})

		Convey("A later book fills and held inference selects the exit", func() {
			state, release, err := replay.step("BTC/USD", 99, 100, true, "ENTER", "WAIT")
			So(err, ShouldBeNil)
			So(state.Open(), ShouldEqual, 1)
			release()
			state, release, err = replay.step("BTC/USD", 110, 111, false, "WAIT", "EXIT")
			So(err, ShouldBeNil)
			So(state.Open(), ShouldEqual, 1)
			release()
			state, release, err = replay.step("BTC/USD", 110, 111, true, "WAIT", "WAIT")
			defer release()
			So(err, ShouldBeNil)
			So(state.Open(), ShouldEqual, 0)
			So(state.Outcomes(), ShouldEqual, 1)
			So(state.Positives(), ShouldEqual, 1)
			closed, err := state.Closed()
			So(err, ShouldBeNil)
			profit, err := closed.Pnl()
			So(err, ShouldBeNil)
			So(profit, ShouldEqualMoney, "0.008320000000")
			So(state.EdgeDefined(), ShouldBeTrue)
			So(state.UncertaintyDefined(), ShouldBeFalse)
		})

		Convey("Partial execution retains exact basis even below the submitted order minimum", func() {
			state, release, err := replay.step("BTC/USD", 99, 100, true, "WAIT", "EXIT")
			So(err, ShouldBeNil)
			So(state.Open(), ShouldEqual, 1)
			release()
			replay.bidQuantity = "0.0004"
			state, release, err = replay.step("BTC/USD", 110, 111, true, "WAIT", "WAIT")
			defer release()
			So(err, ShouldBeNil)
			So(state.Outcomes(), ShouldEqual, 0)
			positions, err := state.Positions()
			So(err, ShouldBeNil)
			So(positions.Len(), ShouldEqual, 1)
			quantity, err := positions.At(0).Quantity()
			So(err, ShouldBeNil)
			So(quantity, ShouldEqualMoney, "0.000600000000")
			basis, err := positions.At(0).Basis()
			So(err, ShouldBeNil)
			So(basis, ShouldEqualMoney, "0.060480000000")
			pnl, err := state.Pnl()
			So(err, ShouldBeNil)
			So(pnl, ShouldEqualMoney, "0.003328000000")
		})

		Convey("Another market spends the same remaining cash", func() {
			state, release, err := replay.step("ETH/USD", 199, 200, true, "ENTER", "")
			defer release()
			So(err, ShouldBeNil)
			cash, err := state.Cash()
			So(err, ShouldBeNil)
			So(cash, ShouldEqualMoney, "199.6976")
		})
	})
}

func TestAccountSnapshot(t *testing.T) {
	Convey("An idle account checkpoint restores as an idle graph node", t, func() {
		replay := newAccountReplay(t)
		future, release := runtime.Snapshot(replay.account).Snapshot(replay.ctx, nil)
		defer release()
		state, err := future.Struct()
		So(err, ShouldBeNil)
		encoded, err := state.Data()
		So(err, ShouldBeNil)
		So(encoded, ShouldBeEmpty)
		restored, releaseRestore := runtime.Snapshot(replay.account).Restore(replay.ctx, func(params runtime.Snapshot_restore_Params) error { return params.SetData(encoded) })
		defer releaseRestore()
		_, err = restored.Struct()
		So(err, ShouldBeNil)
		account, releaseAccount, err := replay.step("BTC/USD", 99, 100, true, "WAIT", "WAIT")
		defer releaseAccount()
		So(err, ShouldBeNil)
		cash, err := account.Cash()
		So(err, ShouldBeNil)
		So(cash, ShouldEqualMoney, "200")
	})
	Convey("A pending native paper order survives checkpoint restoration", t, func() {
		replay := newAccountReplay(t)
		_, release, err := replay.step("BTC/USD", 99, 100, true, "ENTER", "")
		So(err, ShouldBeNil)
		release()
		state := runtime.Snapshot(replay.account)
		future, releaseSnapshot := state.Snapshot(replay.ctx, nil)
		defer releaseSnapshot()
		result, err := future.Struct()
		So(err, ShouldBeNil)
		encoded, err := result.Data()
		So(err, ShouldBeNil)
		replacement := Account_ServerToClient(NewAccount())
		restored, releaseRestore := runtime.Snapshot(replacement).Restore(replay.ctx, func(params runtime.Snapshot_restore_Params) error { return params.SetData(encoded) })
		_, err = restored.Struct()
		So(err, ShouldBeNil)
		releaseRestore()
		replay.account.Release()
		replay.account = replacement
		account, release, err := replay.step("BTC/USD", 99, 100, true, "WAIT", "WAIT")
		defer release()
		So(err, ShouldBeNil)
		So(account.Open(), ShouldEqual, 1)
	})
}

func BenchmarkAccountWrite(b *testing.B) {
	replay := newAccountReplay(b)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		flat, held := "WAIT", "WAIT"

		if index%4 == 0 {
			flat = "ENTER"
		}

		if index%4 == 2 {
			held = "EXIT"
		}
		_, release, err := replay.step("BTC/USD", 99+float64(index%7), 100+float64(index%7), true, flat, held)

		if err != nil {
			b.Fatal(err)
		}
		release()
	}
}

func TestAccountWriteReversal(t *testing.T) {
	Convey("Paper outcomes follow fee-inclusive rising, declining and recovering market legs", t, func() {
		replay := newAccountReplay(t)
		var outcomes uint64
		var negative bool
		for index, price := range marketfixture.Reversal() {
			flat, held := "WAIT", "WAIT"
			if index%8 == 0 {
				flat = "ENTER"
			}
			if index%8 == 6 {
				held = "EXIT"
			}
			state, release, err := replay.step("BTC/USD", price-0.01, price+0.01, true, flat, held)
			So(err, ShouldBeNil)
			if state.Outcomes() > outcomes {
				completed, err := state.Closed()
				So(err, ShouldBeNil)
				if completed.Edge() < 0 {
					negative = true
				}
				outcomes = state.Outcomes()
			}
			if index == len(marketfixture.Reversal())-1 {
				So(state.UncertaintyDefined(), ShouldBeTrue)
				So(state.StandardError(), ShouldBeGreaterThan, 0)
				So(state.Positives(), ShouldBeGreaterThan, 0)
			}
			release()
		}
		So(outcomes, ShouldBeGreaterThan, 1)
		So(negative, ShouldBeTrue)
	})
}

/* ShouldEqualMoney compares exact decimal values without requiring cosmetic trailing zeros. */
func ShouldEqualMoney(actual any, expected ...any) string {
	left, err := decimal.NewFromString(actual.(string))
	if err != nil {
		return err.Error()
	}
	right, err := decimal.NewFromString(expected[0].(string))
	if err != nil {
		return err.Error()
	}
	if left.Cmp(right) != 0 {
		return fmt.Sprintf("expected exact money %s, got %s", right.String(), left.String())
	}
	return ""
}
