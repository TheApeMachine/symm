package replay

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/signal/codec"
)

func TestIngestTradeBatch(testingTB *testing.T) {
	Convey("Given a trade burst", testingTB, func() {
		tree := krakenmarket.MarketTree()
		scope := "BTC/EUR"
		base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

		for index := range 16 {
			IngestTradeBatch(tree, krakenmarket.TradeFeedArtifact(krakenmarket.TradeUpdates{{
				Symbol:    scope,
				Price:     100 + float64(index),
				Qty:       1,
				Side:      "buy",
				Timestamp: base.Add(time.Duration(index) * 100 * time.Millisecond),
			}}))
		}

		var excitationFound bool

		for inbound := range tree.Seek([]byte("trade/" + scope)) {
			payload, payloadOK := inbound.PayloadQuiet()

			if payloadOK && codec.ValidExcitationPayload(payload) {
				excitationFound = true
			}

			inbound.Release()
		}

		Convey("It should index excitation payloads under trade/{scope}", func() {
			So(excitationFound, ShouldBeTrue)
		})
	})
}

func BenchmarkIngestTradeBatch(b *testing.B) {
	tree := krakenmarket.MarketTree()
	batch := krakenmarket.TradeFeedArtifact(krakenmarket.TradeUpdates{{
		Symbol:    "ETH/EUR",
		Price:     2500,
		Qty:       1,
		Side:      "sell",
		Timestamp: time.Now(),
	}})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		IngestTradeBatch(tree, batch)
	}
}
