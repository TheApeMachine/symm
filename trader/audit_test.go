package trader

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

func TestWriteDecisionAuditQuoteFields(t *testing.T) {
	Convey("Given a trader with quotes and audit enabled", t, func() {
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		viper.Set("trading.audit.file", path)
		defer viper.Set("trading.audit.file", "")

		writer, err := audit.OpenWriter()
		So(err, ShouldBeNil)

		quotes := broker.NewQuoteCache(t.Context(), nil)
		quotes.InstallQuoteForTest(broker.Quote{
			Symbol: "PEPE/EUR",
			Bid:    0.000010,
			Ask:    0.000011,
			Last:   0.0000105,
		})

		crypto := newTestCrypto()
		crypto.audit = writer
		crypto.quotes = quotes

		action := reasoning.Action{
			Type:     reasoning.ActionMarket,
			Symbol:   "PEPE/EUR",
			Side:     trading.Buy,
			Quantity: 1_000_000,
		}

		crypto.publishDecision(action, "rejected", "spread 150bps exceeds max 120bps")
		So(writer.Close(), ShouldBeNil)

		Convey("It records preflight gate and quote quality on trade decisions", func() {
			raw, readErr := os.ReadFile(path)

			So(readErr, ShouldBeNil)

			row := map[string]any{}
			So(json.Unmarshal(raw, &row), ShouldBeNil)
			So(row["preflight_gate"], ShouldEqual, "spread")
			So(row["spread_bps"], ShouldNotBeNil)
			So(row["depth_coverage"], ShouldNotBeNil)
		})
	})
}

func TestWriteFillAndPositionOpenAudit(t *testing.T) {
	Convey("Given a trader with audit enabled", t, func() {
		path := filepath.Join(t.TempDir(), "audit.jsonl")
		viper.Set("trading.audit.file", path)
		defer viper.Set("trading.audit.file", "")

		writer, err := audit.OpenWriter()
		So(err, ShouldBeNil)

		crypto := newTestCrypto()
		crypto.audit = writer

		action := reasoning.Action{
			Type:   reasoning.ActionMarket,
			Symbol: "BTC/EUR",
			Side:   trading.Buy,
		}

		crypto.pending["BTC/EUR"] = action

		crypto.observeExecution(map[string]any{
			"channel": "executions",
			"symbol":  "BTC/EUR",
			"side":    string(trading.Buy),
			"qty":     0.25,
			"price":   50_000.0,
			"fee":     0.32,
		})

		So(writer.Close(), ShouldBeNil)

		Convey("It writes fill and position_open audit rows", func() {
			raw, readErr := os.ReadFile(path)

			So(readErr, ShouldBeNil)
			So(string(raw), ShouldContainSubstring, `"audit_event":"fill"`)
			So(string(raw), ShouldContainSubstring, `"audit_event":"position_open"`)
			So(strings.Count(string(raw), "\n"), ShouldBeGreaterThanOrEqualTo, 2)
		})
	})
}

func TestPreflightGateFromReason(t *testing.T) {
	Convey("Given broker rejection reasons", t, func() {
		So(preflightGateFromReason("spread 150bps exceeds max 120bps"), ShouldEqual, "spread")
		So(preflightGateFromReason("stale quote 20s exceeds max 15s"), ShouldEqual, "stale_quote")
		So(preflightGateFromReason("projected slippage 90bps exceeds max 80bps"), ShouldEqual, "slippage")
		So(preflightGateFromReason("depth coverage 0.4 below min 1.0"), ShouldEqual, "depth")
		So(preflightGateFromReason("not holding"), ShouldEqual, "")
	})
}
