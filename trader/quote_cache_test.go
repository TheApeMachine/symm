package trader

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
)

func seedExitQuote(crypto *Crypto, symbol string, last float64) {
	crypto.quotes.ingestTicker(market.TickerUpdate{
		Symbol: symbol,
		Last:   last,
		Bid:    last - 1,
		Ask:    last + 1,
	})
}

func TestQuoteCacheSnapshot(t *testing.T) {
	Convey("Given a missing quote cache entry", t, func() {
		cache := newQuoteCache()

		Convey("snapshot should not manufacture bid/ask or timestamps", func() {
			quote := cache.snapshot("BTC/EUR", 100)

			So(quote.Last, ShouldEqual, 0)
			So(quote.Bid, ShouldEqual, 0)
			So(quote.Ask, ShouldEqual, 0)
			So(quote.At.IsZero(), ShouldBeTrue)
		})
	})
}

func TestQuoteCacheIngestUsesLocalReceiveTime(t *testing.T) {
	Convey("Given a ticker row with an exchange timestamp in the past", t, func() {
		cache := newQuoteCache()
		before := time.Now().Add(-time.Minute)

		cache.ingestTicker(market.TickerUpdate{
			Symbol:    "BTC/EUR",
			Last:      100,
			Bid:       99,
			Ask:       101,
			Timestamp: before.UTC().Format(time.RFC3339Nano),
		})

		Convey("The cached quote should be fresh for PreflightGates", func() {
			quote := cache.snapshot("BTC/EUR", 0)
			buy := broker.Buy{
				Symbol:    "BTC/EUR",
				Quote:     quote,
				Execution: config.ExecutionScopeFrom(config.System),
			}

			So(buy.PreflightGates(), ShouldBeNil)
			So(time.Since(quote.At), ShouldBeLessThan, config.System.SnapshotFreshnessTTL)
		})
	})
}

func TestQuoteCacheIngestBookRefreshesFreshness(t *testing.T) {
	Convey("Given a quote stamped from an earlier ticker ingest", t, func() {
		cache := newQuoteCache()
		cache.quotes["BTC/EUR"] = broker.Quote{
			Last: 100,
			Bid:  99,
			Ask:  101,
			At:   time.Now().Add(-time.Second),
		}

		cache.ingestBook(market.BookUpdate{
			Symbol: "BTC/EUR",
			Kind:   market.BookSnapshot,
			Bids:   []market.BookLevel{{Price: 99.5, Qty: 2}},
			Asks:   []market.BookLevel{{Price: 100.5, Qty: 1}},
		})

		Convey("The book ingest should refresh At and top-of-book on snapshots", func() {
			quote := cache.snapshot("BTC/EUR", 0)
			buy := broker.Buy{
				Symbol:    "BTC/EUR",
				Quote:     quote,
				Execution: config.ExecutionScopeFrom(config.System),
			}

			So(quote.Bid, ShouldEqual, 99.5)
			So(quote.Ask, ShouldEqual, 100.5)
			So(buy.PreflightGates(), ShouldBeNil)
		})
	})
}

func TestPreflightGatesRejectsIncompleteQuote(t *testing.T) {
	Convey("Given a buy with last only", t, func() {
		buy := broker.Buy{
			Symbol: "BTC/EUR",
			Quote:  broker.Quote{Last: 100},
		}

		Convey("PreflightGates should reject the incomplete top of book", func() {
			err := buy.PreflightGates()

			So(err, ShouldNotBeNil)
		})
	})
}
