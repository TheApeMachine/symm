package replay

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/signal/codec"
)

func TestIngestBookBatch(testingTB *testing.T) {
	Convey("Given repeated book snapshots", testingTB, func() {
		tree := krakenmarket.MarketTree()
		scope := "ETH/EUR"
		observed := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

		for index := range 8 {
			IngestBookBatch(tree, krakenmarket.BookFeedArtifact(krakenmarket.BookUpdates{{
				Symbol:    scope,
				Type:      "snapshot",
				Timestamp: observed.Add(time.Duration(index) * time.Second),
				Bids:      []krakenmarket.BookLevel{{Price: 100 - float64(index)*0.01, Qty: 20 - float64(index)}},
				Asks:      []krakenmarket.BookLevel{{Price: 100.2, Qty: 10}},
			}}))
		}

		var decayFound bool

		for inbound := range tree.Seek([]byte("features/" + scope)) {
			payload, payloadOK := inbound.PayloadQuiet()

			if payloadOK && codec.ValidDecayPayload(payload) {
				decayFound = true
			}

			inbound.Release()
		}

		Convey("It should publish decay features under features/{scope}", func() {
			So(decayFound, ShouldBeTrue)
		})
	})
}

func BenchmarkIngestBookBatch(b *testing.B) {
	tree := krakenmarket.MarketTree()
	batch := krakenmarket.BookFeedArtifact(krakenmarket.BookUpdates{{
		Symbol: "BTC/EUR",
		Type:   "snapshot",
		Bids:   []krakenmarket.BookLevel{{Price: 100, Qty: 2}},
		Asks:   []krakenmarket.BookLevel{{Price: 100.2, Qty: 1}},
	}})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		IngestBookBatch(tree, batch)
	}
}
