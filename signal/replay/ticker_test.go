package replay

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/signal/codec"
)

func TestIngestTickerBatch(testingTB *testing.T) {
	Convey("Given a cross-section of ticker updates", testingTB, func() {
		tree := krakenmarket.MarketTree()

		IngestTickerBatch(tree, krakenmarket.TickerFeedArtifact(krakenmarket.TickerUpdates{
			{Symbol: "BTC/EUR", ChangePct: 0.02, Last: 100},
			{Symbol: "ETH/EUR", ChangePct: -0.01, Last: 50},
			{Symbol: "SOL/EUR", ChangePct: 0.03, Last: 10},
		}))

		var convictionFound bool

		for inbound := range tree.Seek([]byte("features/BTC/EUR")) {
			payload, payloadOK := inbound.PayloadQuiet()

			if payloadOK && codec.ValidFloatPayload(payload, codec.ConvictionMinFloats) {
				convictionFound = true
			}

			inbound.Release()
		}

		Convey("It should publish conviction features for tracked scopes", func() {
			So(convictionFound, ShouldBeTrue)
		})
	})
}
