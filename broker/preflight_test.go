package broker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
	. "github.com/theapemachine/symm/signal"
)

func TestQuoteCacheReadsTreeTicker(testingTB *testing.T) {
	Convey("Given ticker rows in the shared tree", testingTB, func() {
		tree := NewTestTree()
		cache := NewQuoteCache(tree)

		insertIngest(tree, "ticker", "BTC/EUR", []byte(
			`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/EUR","last":100,"bid":99,"ask":101}]}`,
		))

		Convey("QuoteForSymbol should merge bid/ask/last", func() {
			quote, ok := cache.QuoteForSymbol("BTC/EUR")

			So(ok, ShouldBeTrue)
			So(quote.Bid, ShouldEqual, 99)
			So(quote.Ask, ShouldEqual, 101)
			So(quote.Last, ShouldEqual, 100)
		})
	})
}

func TestQuoteCacheUsesArtifactTimestampWhenPayloadLacksTimestamp(testingTB *testing.T) {
	Convey("Given a ticker row without JSON timestamp", testingTB, func() {
		tree := NewTestTree()
		cache := NewQuoteCache(tree)
		staleAt := time.Now().UTC().Add(-2 * time.Minute)

		insertIngestAt(tree, "ticker", "STALE/EUR", []byte(
			`{"channel":"ticker","type":"update","data":[{"symbol":"STALE/EUR","last":100,"bid":99,"ask":101}]}`,
		), staleAt)

		viper.Set("trading.max_quote_age", time.Second)

		Convey("QuoteForSymbol should expose ingest time for freshness gates", func() {
			quote, ok := cache.QuoteForSymbol("STALE/EUR")

			So(ok, ShouldBeTrue)
			So(quote.UpdatedAt.UnixNano(), ShouldEqual, staleAt.UnixNano())

			request := PreflightRequest{
				Quote:      quote,
				Side:       trading.Buy,
				Quantity:   1,
				OrderType:  trading.Market,
				ActionType: logic.ActionMarket,
			}

			So(PreflightGatesAt(request, time.Now().UTC()), ShouldNotBeNil)
		})
	})
}

func TestPreflightGatesRejectsWideSpread(testingTB *testing.T) {
	Convey("Given max spread bps configured", testingTB, func() {
		viper.Set("trading.max_spread_bps", 50.0)

		quote := Quote{
			Symbol:    "BTC/EUR",
			Bid:       90,
			Ask:       110,
			Last:      100,
			UpdatedAt: time.Now().UTC(),
		}

		request := PreflightRequest{
			Quote:      quote,
			Side:       trading.Buy,
			Quantity:   1,
			OrderType:  trading.Market,
			ActionType: logic.ActionMarket,
		}

		Convey("It should reject quotes wider than the limit", func() {
			So(PreflightGates(request), ShouldNotBeNil)
		})
	})
}

func TestPreflightGatesRejectsStaleQuote(testingTB *testing.T) {
	Convey("Given max quote age configured", testingTB, func() {
		viper.Set("trading.max_quote_age", time.Second)

		quote := Quote{
			Symbol:    "BTC/EUR",
			Bid:       99,
			Ask:       101,
			Last:      100,
			UpdatedAt: time.Now().UTC().Add(-2 * time.Second),
		}

		request := PreflightRequest{
			Quote:      quote,
			Side:       trading.Buy,
			Quantity:   1,
			OrderType:  trading.Market,
			ActionType: logic.ActionMarket,
		}

		Convey("It should reject stale entries", func() {
			So(PreflightGates(request), ShouldNotBeNil)
		})

		Convey("It should accept exits with fresh last at tape time", func() {
			fresh := quote
			fresh.UpdatedAt = time.Now().UTC()
			exit := PreflightRequest{
				Quote:      fresh,
				Side:       trading.Sell,
				Quantity:   1,
				OrderType:  trading.Market,
				ActionType: logic.ActionSettlePosition,
			}

			So(PreflightGates(exit), ShouldBeNil)
		})
	})
}

func TestEffectiveNetworkLatencyFromFile(testingTB *testing.T) {
	Convey("Given a JSON latency profile", testingTB, func() {
		tempDir := testingTB.TempDir()
		path := filepath.Join(tempDir, "latency.json")
		err := os.WriteFile(path, []byte(`{"samples":[10000000,50000000,20000000]}`), 0o600)

		So(err, ShouldBeNil)

		latency := EffectiveNetworkLatencyFromFile(path)

		Convey("It should return the p95 sample", func() {
			So(latency, ShouldEqual, 20_000_000)
		})
	})
}

func TestSlippageFillHalfSpread(testingTB *testing.T) {
	Convey("Given a quote without book depth", testingTB, func() {
		quote := Quote{Symbol: "BTC/EUR", Bid: 99, Ask: 101, Last: 100}

		Convey("It should cross half the spread for buys", func() {
			fill, err := SlippageFill(quote, trading.Buy, 1)

			So(err, ShouldBeNil)
			So(fill.Price, ShouldAlmostEqual, 101, 1e-9)
		})
	})
}

func BenchmarkSlippageFill(benchmarkTB *testing.B) {
	quote := Quote{
		Symbol: "BTC/EUR",
		Bid:    99,
		Ask:    100,
		Last:   99.5,
		Book: Book{
			Asks: []BookLevel{{Price: 100, Qty: 2}, {Price: 101, Qty: 3}},
		},
		UpdatedAt: time.Now().UTC(),
	}

	for benchmarkTB.Loop() {
		_, _ = SlippageFill(quote, trading.Buy, 1)
	}
}

func insertIngest(tree *dmt.Tree, role, scope string, payload []byte) {
	insertIngestAt(tree, role, scope, payload, time.Time{})
}

func insertIngestAt(tree *dmt.Tree, role, scope string, payload []byte, at time.Time) {
	artifact := datura.Acquire("test", datura.Artifact_Type_json).
		WithRole(role).
		WithScope(scope).
		WithPayload(payload)

	if !at.IsZero() {
		artifact.SetTimestamp(at.UnixNano())
	}

	InsertTreeArtifact(tree, artifact)
}
