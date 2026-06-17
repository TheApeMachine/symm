package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestInsertMarketArtifactBook(testingTB *testing.T) {
	Convey("Given a batched book artifact", testingTB, func() {
		tree := MarketTree()
		scope := "BTC/EUR"

		InsertMarketArtifact(tree, BookFeedArtifact(BookUpdates{{
			Symbol: scope,
			Bids:   []BookLevel{{Price: 100, Qty: 2}},
			Asks:   []BookLevel{{Price: 101, Qty: 1}},
		}}))

		probe := datura.Acquire("trader", datura.Artifact_Type_json)
		probe.WithRole("measurement")
		probe.WithScope(scope)

		var found bool

		for inbound := range tree.Seek([]byte("book/" + scope)) {
			found = true
			inbound.Release()
		}

		Convey("It should index the row under book/{scope}", func() {
			So(found, ShouldBeTrue)
		})
	})
}

func TestInsertMarketArtifactTrade(testingTB *testing.T) {
	Convey("Given a batched trade artifact", testingTB, func() {
		tree := MarketTree()
		scope := "ETH/EUR"

		InsertMarketArtifact(tree, TradeFeedArtifact(TradeUpdates{{
			Symbol:    scope,
			Price:     2500,
			Qty:       1,
			Side:      "buy",
			Timestamp: time.Now(),
		}}))

		var found bool

		for inbound := range tree.Seek([]byte("trade/" + scope)) {
			found = true
			inbound.Release()
		}

		Convey("It should index the row under trade/{scope}", func() {
			So(found, ShouldBeTrue)
		})
	})
}

func BenchmarkInsertMarketArtifact(b *testing.B) {
	tree := MarketTree()
	batch := BookFeedArtifact(BookUpdates{{
		Symbol: "BTC/EUR",
		Bids:   []BookLevel{{Price: 100, Qty: 2}},
		Asks:   []BookLevel{{Price: 101, Qty: 1}},
	}})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		InsertMarketArtifact(tree, batch)
	}
}
