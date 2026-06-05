package trader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

func newTestCrypto() *Crypto {
	return &Crypto{
		streams:      focus.NewSet(),
		inventory:    map[string]float64{},
		avgEntry:     map[string]float64{},
		pending:      map[string]perspectives.Action{},
		lastDecision: map[string]string{},
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
	Convey("Given the wallet balance the API published", t, func() {
		crypto := newTestCrypto()

		Convey("It deploys the available cash (no fees configured in test)", func() {
			crypto.availableQuote = 200
			So(crypto.sizeEntry(100), ShouldAlmostEqual, 2, 1e-9)
		})

		Convey("It sizes nothing when the wallet is empty", func() {
			crypto.availableQuote = 0
			So(crypto.sizeEntry(100), ShouldEqual, 0)
		})

		Convey("It sizes nothing for a non-positive price", func() {
			crypto.availableQuote = 200
			So(crypto.sizeEntry(0), ShouldEqual, 0)
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
