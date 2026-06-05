package signal

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
)

func TestSocketMessageFromValue(t *testing.T) {
	Convey("Given a replay raw book envelope", t, func() {
		raw := json.RawMessage(`[
			{"symbol":"SYN/EUR","bids":[{"price":99,"qty":8}],"asks":[{"price":101,"qty":4}]}
		]`)

		envelope, ok := SocketMessageFromValue(map[string]any{
			"channel": public.BookChannel,
			"type":    market.BookSnapshot,
			"data":    raw,
		})

		Convey("It should preserve the channel type for book decoding", func() {
			So(ok, ShouldBeTrue)
			So(envelope.Type, ShouldEqual, market.BookSnapshot)

			books := GetBooks(envelope)
			So(books, ShouldHaveLength, 1)
			So(books[0].IsSnapshot(), ShouldBeTrue)
		})
	})
}
