package stack_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestBooter_Test proves the injected paper Conn closes the real production
Desk, Position, execution, and Balance loop against the simulated market.
*/
func TestBooter_Test(t *testing.T) {
	Convey("Given the complete production graph on a simulated market", t, func() {
		market := tests.NewMarket(t.Context(), 1)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})
		So(market.Warmup(tests.Idle), ShouldBeNil)
		symbol := market.Symbols[0]
		quantity := decimal.NewFromInt64(1)

		Convey("A Desk entry fills at the simulated ask and updates inventory", func() {
			position, err := wired.Desk.Buy(
				types.NewHolding(t.Context(), symbol, quantity),
				true,
			)
			So(err, ShouldBeNil)
			So(position.Status(), ShouldEqual, types.PENDING)
			So(market.Paper.Drain(), ShouldBeNil)
			So(position.Status(), ShouldEqual, types.OPEN)
			holding, err := wired.Balance.Holding(symbol)
			So(err, ShouldBeNil)
			So(holding.Qty.Cmp(quantity), ShouldEqual, 0)
			So(holding.EntryPrice, ShouldNotBeNil)
			So(holding.EntryFee, ShouldNotBeNil)

			Convey("A Desk exit fills at the simulated bid and clears inventory", func() {
				So(wired.Desk.Sell(symbol), ShouldBeNil)
				So(market.Paper.Drain(), ShouldBeNil)
				So(wired.Desk.OpenPositions(), ShouldEqual, 0)
				So(position.Status(), ShouldEqual, types.CLOSED)
			})
		})
	})
}

/*
TestBooter_AuditHotPath proves the runtime recorder is wired through Tick so
phase breadcrumbs and ingress drops land in runtime-audit.jsonl.
*/
func TestBooter_AuditHotPath(t *testing.T) {
	Convey("Given a warmed production graph with a live audit recorder", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)

		auditFile := filepath.Join(
			viper.GetString("system.data_path"), "runtime-audit.jsonl",
		)

		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})

		So(wired.Recorder, ShouldNotBeNil)
		So(market.Warmup(tests.Idle), ShouldBeNil)

		Convey("When a pump tick completes and the recorder flushes", func() {
			So(market.Transition(tests.MarketStateFastPump, func() error {
				return nil
			}), ShouldBeNil)

			So(wired.Recorder.Close(), ShouldBeNil)
			wired.Recorder = nil

			body, readErr := os.ReadFile(auditFile)
			So(readErr, ShouldBeNil)
			log := string(body)

			Convey("It records the Crypto and Analyzer phase spine", func() {
				for _, phase := range []string{
					"tick_begin",
					"measure_end",
					"analyze_begin",
					"decide_begin",
					"decide_end",
					"desk",
					"tick_end",
				} {
					So(log, ShouldContainSubstring, `"phase":"`+phase+`"`)
				}
			})
		})
	})
}

/*
TestBooter_TickStrategy proves Crypto.Tick → Decide → Desk fill against a
fixture pump tape on the production graph, then exits the filled lot.
*/
func TestBooter_TickStrategy(t *testing.T) {
	Convey("Given a warmed production graph on a three-symbol tape", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})

		So(market.Warmup(tests.Idle), ShouldBeNil)

		Convey("When a fast pump plays through Tick", func() {
			entered := false
			var thesis *types.Thesis

			So(market.Transition(tests.MarketStateFastPump, func() error {
				next := wired.Thesis

				if next == nil {
					return nil
				}

				thesis = next
				So(thesis.Incomplete(), ShouldBeFalse)
				So(market.Paper.Drain(), ShouldBeNil)

				if entered {
					return nil
				}

				// Enter may leave Thesis before the next Transition sample; a live
				// OPEN balance lot is the durable proof Crypto applied Decide.
				for open := range wired.Balance.Holdings() {
					if open.Status != types.OPEN ||
						open.Qty == nil || open.Qty.Sign() <= 0 ||
						open.EntryPrice == nil || open.EntryPrice.Sign() <= 0 {
						continue
					}

					entered = true
					return nil
				}

				for _, decision := range thesis.Decisions {
					if decision.Action != types.ActionEnter {
						continue
					}

					holding, holdErr := wired.Balance.Holding(decision.Symbol)

					if holdErr != nil || holding.Qty == nil || holding.Qty.Sign() <= 0 {
						continue
					}

					entered = true
					return nil
				}

				return nil
			}), ShouldBeNil)

			Convey("It opens inventory from strategy decisions and can exit it", func() {
				So(thesis, ShouldNotBeNil)
				snapshot := thesis.CutSnapshot()
				So(snapshot.Forecasts, ShouldNotBeEmpty)
				So(entered, ShouldBeTrue)
				So(wired.Desk.OpenPositions(), ShouldBeGreaterThan, 0)

				sold := 0

				for holding := range wired.Balance.Holdings() {
					So(wired.Desk.Sell(holding.Symbol), ShouldBeNil)
					sold++
				}

				So(sold, ShouldBeGreaterThan, 0)
				So(market.Paper.Drain(), ShouldBeNil)
				So(wired.Desk.OpenPositions(), ShouldEqual, 0)
			})
		})
	})
}
