package toxicity

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/signal/codec"
)

func TestReplayBookPayload(testingTB *testing.T) {
	Convey("Given book and trade observations", testingTB, func() {
		ResetDefault()

		symbol := "ETH/EUR"
		pair := PairFromTick(symbol, 0.1)
		startAt := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

		IngestBook(symbol, pair, &krakenmarket.BookUpdate{
			Symbol: symbol,
			Type:   "snapshot",
			Bids:   []krakenmarket.BookLevel{{Price: 100, Qty: 80}},
			Asks:   []krakenmarket.BookLevel{{Price: 101, Qty: 80}},
		}, startAt)
		IngestTrade(symbol, pair, 100.5, 5, startAt.Add(time.Second))

		payload, ok := ReplayBookPayload(symbol)

		Convey("It should expose a book-quality replay payload", func() {
			So(ok, ShouldBeTrue)
			So(len(payload), ShouldEqual, 13)
			So(codec.ValidFloatPayload(codec.EncodePayload(payload...), codec.BookQualityMinFloats), ShouldBeTrue)
		})
	})
}
