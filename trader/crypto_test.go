package trader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

func holdingsFrame(holdings ...map[string]any) map[string]any {
	return map[string]any{"channel": "holdings", "holdings": holdings}
}

func newTestCrypto() *Crypto {
	return &Crypto{
		streams:          focus.NewSet(),
		inventory:        map[string]float64{},
		avgEntry:         map[string]float64{},
		pending:          map[string]perspectives.Action{},
		lastDecision:     map[string]string{},
		positionFraction: 1.0,
	}
}

func buyExec(symbol string, qty, price float64) map[string]any {
	return map[string]any{
		"channel": "executions", "symbol": symbol,
		"side": string(trading.Buy), "qty": qty, "price": price,
	}
}

func sellExec(symbol string, qty float64) map[string]any {
	return map[string]any{
		"channel": "executions", "symbol": symbol,
		"side": string(trading.Sell), "qty": qty, "price": 1.0,
	}
}

func TestCleanReason(t *testing.T) {
	Convey("cleanReason strips internal prefixes and the symbol suffix", t, func() {
		So(cleanReason("preflight: no quote for BTC/EUR"), ShouldEqual, "no quote")
		So(cleanReason("paper balances: insufficient funds"), ShouldEqual, "insufficient funds")
		So(cleanReason("paper fill: no quote for ETH/GBP"), ShouldEqual, "no quote")
		So(
			cleanReason("preflight: toxic regime blocks discretionary entry for AUD/USD"),
			ShouldEqual,
			"toxic regime blocks discretionary entry",
		)
		So(cleanReason("order still resolving"), ShouldEqual, "order still resolving")
	})
}

func TestQuoteCurrency(t *testing.T) {
	Convey("quoteCurrency reads the part after the slash", t, func() {
		So(quoteCurrency("BTC/EUR"), ShouldEqual, "EUR")
		So(quoteCurrency("ETH/BTC"), ShouldEqual, "BTC")
		So(quoteCurrency("BTCEUR"), ShouldEqual, "BTCEUR")
	})
}

func TestFundableSymbol(t *testing.T) {
	Convey("Given an EUR wallet", t, func() {
		crypto := newTestCrypto()
		crypto.walletCurrency = "EUR"

		So(crypto.fundableSymbol("BTC/EUR"), ShouldBeTrue)
		So(crypto.fundableSymbol("ETH/BTC"), ShouldBeFalse)
		So(crypto.fundableSymbol("AUD/USD"), ShouldBeFalse)

		Convey("An unset wallet currency funds anything (test default)", func() {
			crypto.walletCurrency = ""
			So(crypto.fundableSymbol("ETH/BTC"), ShouldBeTrue)
		})
	})
}

func TestSizeEntry(t *testing.T) {
	Convey("Given a trader sizing entries from a known capital base", t, func() {
		crypto := newTestCrypto()
		crypto.capitalBase = 200
		crypto.availableQuote = 1000 // abundant, so slot sizing is not cash-bounded here

		Convey("It refuses to size without a capital base — no substitute for unknown capital", func() {
			crypto.capitalBase = 0
			So(crypto.sizeEntry(100), ShouldEqual, 0)
		})

		Convey("It sizes nothing for a non-positive price", func() {
			So(crypto.sizeEntry(0), ShouldEqual, 0)
		})

		Convey("It sizes nothing when the wallet cannot fund anything", func() {
			crypto.availableQuote = 0
			So(crypto.sizeEntry(100), ShouldEqual, 0)
		})

		Convey("At full deployment (fraction 1.0) one entry deploys a whole slot of the base", func() {
			crypto.positionFraction = 1.0
			So(crypto.sizeEntry(100), ShouldAlmostEqual, 2, 1e-9) // (1.0*200)/100
		})

		Convey("At fraction 0.1 each entry deploys a tenth of the BASE, not of the remaining cash", func() {
			crypto.positionFraction = 0.1
			// a tenth of the 200 base = 0.2 qty; a tenth of the 1000 cash would be 1.0
			So(crypto.sizeEntry(100), ShouldAlmostEqual, 0.2, 1e-9)
		})

		Convey("It never commits more than the wallet can fund, fee included", func() {
			crypto.capitalBase = 10000 // a full slot would dwarf the wallet
			crypto.availableQuote = 200
			crypto.positionFraction = 1.0

			fee := broker.MakerFeePctFromViper() / 100
			cost := crypto.sizeEntry(100) * 100 * (1 + fee)

			So(cost, ShouldAlmostEqual, 200, 1e-6) // spends the wallet, never more
		})
	})
}

func TestSizeEntryConcurrentCap(t *testing.T) {
	Convey("position_fraction caps concurrent positions at round(1/fraction)", t, func() {
		crypto := newTestCrypto()
		crypto.capitalBase = 200
		crypto.availableQuote = 200

		Convey("At fraction 1.0 a second entry is refused while one position is held", func() {
			crypto.positionFraction = 1.0
			crypto.inventory["BTC/EUR"] = 2 // the single allowed slot is taken

			So(crypto.sizeEntry(100), ShouldEqual, 0)
		})

		Convey("An in-flight entry counts toward capacity (no over-commit before the fill)", func() {
			crypto.positionFraction = 1.0
			crypto.pending["BTC/EUR"] = perspectives.Action{} // order placed, not yet filled

			So(crypto.sizeEntry(100), ShouldEqual, 0)
		})

		Convey("At fraction 0.1 the eleventh position is refused (ten already committed)", func() {
			crypto.positionFraction = 0.1
			for i := 0; i < 10; i++ {
				crypto.inventory[fmt.Sprintf("C%d/EUR", i)] = 1
			}

			So(crypto.sizeEntry(100), ShouldEqual, 0)
		})
	})
}

func TestObserveExecution(t *testing.T) {
	Convey("Given a flat trader", t, func() {
		crypto := newTestCrypto()
		crypto.pending["BTC/EUR"] = perspectives.Action{}

		Convey("A buy fill opens the position and marks it held", func() {
			crypto.observeExecution(buyExec("BTC/EUR", 0.5, 100))

			So(crypto.inventory["BTC/EUR"], ShouldEqual, 0.5)
			So(crypto.avgEntry["BTC/EUR"], ShouldEqual, 100)
			So(crypto.streams.Has("BTC/EUR"), ShouldBeTrue)
			_, stillPending := crypto.pending["BTC/EUR"]
			So(stillPending, ShouldBeFalse)
		})

		Convey("A settling sell closes the position and clears holding", func() {
			crypto.observeExecution(buyExec("BTC/EUR", 0.5, 100))
			crypto.observeExecution(sellExec("BTC/EUR", 0.5))

			_, held := crypto.inventory["BTC/EUR"]
			So(held, ShouldBeFalse)
			So(crypto.streams.Has("BTC/EUR"), ShouldBeFalse)
		})

		Convey("A zero-quantity execution only clears the in-flight marker", func() {
			crypto.observeExecution(map[string]any{
				"channel": "executions", "symbol": "BTC/EUR",
				"side": string(trading.Buy), "qty": 0.0, "price": 0.0,
			})

			_, held := crypto.inventory["BTC/EUR"]
			So(held, ShouldBeFalse)
			_, stillPending := crypto.pending["BTC/EUR"]
			So(stillPending, ShouldBeFalse)
		})
	})
}

func TestSubmitGate(t *testing.T) {
	Convey("Given the not-holding gate", t, func() {
		crypto := newTestCrypto()
		entry := perspectives.Action{
			Type: perspectives.ActionLimit, Symbol: "BTC/EUR",
			Side: trading.Buy, Quantity: 1, Price: 100,
		}

		Convey("An entry is skipped while the symbol is already held", func() {
			crypto.inventory["BTC/EUR"] = 1
			crypto.streams.Add("BTC/EUR")

			// desk is nil: reaching it would panic, so a clean return proves the gate.
			crypto.submit(entry)

			So(crypto.streams.Snapshot(), ShouldResemble, []string{"BTC/EUR"})
		})

		Convey("An entry is skipped while an order is in flight", func() {
			crypto.pending["BTC/EUR"] = perspectives.Action{}

			crypto.submit(entry)

			So(crypto.inventory["BTC/EUR"], ShouldEqual, 0)
		})

		Convey("An exit is skipped when nothing is held", func() {
			exit := perspectives.Action{
				Type: perspectives.ActionSettlePosition, Symbol: "BTC/EUR",
			}

			crypto.submit(exit)

			_, stillPending := crypto.pending["BTC/EUR"]
			So(stillPending, ShouldBeFalse)
		})
	})
}

func TestReconcilePositions(t *testing.T) {
	Convey("Given a trader reconciling against an exchange balance snapshot", t, func() {
		crypto := newTestCrypto()

		Convey("It adopts a holding it is not already tracking", func() {
			crypto.observeBalances(holdingsFrame(
				map[string]any{"symbol": "ETC/EUR", "qty": 5.0},
			))

			So(crypto.inventory["ETC/EUR"], ShouldEqual, 5.0)
			So(crypto.streams.Has("ETC/EUR"), ShouldBeTrue)
		})

		Convey("It leaves a tracked position untouched (no entry-price clobber on reconnect)", func() {
			crypto.inventory["BTC/EUR"] = 0.5
			crypto.avgEntry["BTC/EUR"] = 100
			crypto.streams.Add("BTC/EUR")

			crypto.observeBalances(holdingsFrame(
				map[string]any{"symbol": "BTC/EUR", "qty": 0.5},
			))

			So(crypto.inventory["BTC/EUR"], ShouldEqual, 0.5)
			So(crypto.avgEntry["BTC/EUR"], ShouldEqual, 100)
		})

		Convey("It closes a tracked position the exchange no longer shows", func() {
			crypto.inventory["BTC/EUR"] = 0.5
			crypto.avgEntry["BTC/EUR"] = 100
			crypto.streams.Add("BTC/EUR")

			crypto.observeBalances(holdingsFrame()) // empty snapshot: nothing held

			_, held := crypto.inventory["BTC/EUR"]
			So(held, ShouldBeFalse)
			So(crypto.streams.Has("BTC/EUR"), ShouldBeFalse)
		})

		Convey("An empty snapshot on a flat trader changes nothing (paper fresh-start)", func() {
			crypto.observeBalances(holdingsFrame())
			So(len(crypto.inventory), ShouldEqual, 0)
		})

		Convey("A holdings snapshot does not double-count a position the fill already opened", func() {
			crypto.pending["ETC/EUR"] = perspectives.Action{}
			crypto.observeExecution(buyExec("ETC/EUR", 5, 36.0)) // session fill: 5 @ 36
			crypto.observeBalances(holdingsFrame(                // snapshot agrees: 5 held
				map[string]any{"symbol": "ETC/EUR", "qty": 5.0},
			))

			So(crypto.inventory["ETC/EUR"], ShouldEqual, 5.0) // not 10
			So(crypto.avgEntry["ETC/EUR"], ShouldEqual, 36.0) // fill entry kept, not re-marked
		})
	})
}

func TestAdoptPositionMarksAtBid(t *testing.T) {
	Convey("Given a trader with a live quote for the symbol", t, func() {
		quotes := broker.NewQuoteCache(t.Context(), nil)
		quotes.InstallQuoteForTest(broker.Quote{
			Symbol: "ETC/EUR", Bid: 36.97, Ask: 37.00, Last: 36.99,
		})

		crypto := newTestCrypto()
		crypto.quotes = quotes

		Convey("Adopting an unknown-entry holding marks its basis at the bid (the exit price)", func() {
			crypto.observeBalances(holdingsFrame(
				map[string]any{"symbol": "ETC/EUR", "qty": 5.0},
			))

			So(crypto.inventory["ETC/EUR"], ShouldEqual, 5.0)
			So(crypto.avgEntry["ETC/EUR"], ShouldEqual, 36.97)
		})

		Convey("A holding adopted before its quote was ready backfills its basis from the mark", func() {
			crypto.inventory["XLM/EUR"] = 100 // adopted with no quote yet: avg_entry 0
			crypto.avgEntry["XLM/EUR"] = 0
			quotes.InstallQuoteForTest(broker.Quote{Symbol: "XLM/EUR", Bid: 0.17, Ask: 0.172})

			crypto.publishMarks()

			So(crypto.avgEntry["XLM/EUR"], ShouldEqual, 0.17)
		})
	})
}

func TestPublishDecisionAudit(t *testing.T) {
	Convey("Given a trader without a UI subscriber", t, func() {
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		viper.Set("trading.audit.file", path)
		defer viper.Set("trading.audit.file", "")

		writer, err := audit.OpenWriter()
		So(err, ShouldBeNil)

		crypto := newTestCrypto()
		crypto.audit = writer
		action := perspectives.Action{
			Type:   perspectives.ActionLimit,
			Symbol: "BTC/EUR",
			Side:   trading.Buy,
			Price:  100,
		}

		crypto.publishDecision(action, "rejected", "not holding")
		crypto.publishDecision(action, "rejected", "not holding")
		So(writer.Close(), ShouldBeNil)

		Convey("It should write one deduped trade decision audit row", func() {
			raw, readErr := os.ReadFile(path)

			So(readErr, ShouldBeNil)
			So(strings.Count(string(raw), "\n"), ShouldEqual, 1)
			So(string(raw), ShouldContainSubstring, `"audit_event":"trade_decision"`)
			So(string(raw), ShouldContainSubstring, `"block_reason":"not holding"`)
		})
	})
}
