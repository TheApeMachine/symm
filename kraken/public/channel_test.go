package public

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestSocketMessageSplitDataRows(t *testing.T) {
	convey.Convey("Given a trade envelope", t, func() {
		envelope := &SocketMessage{
			Channel: TradesChannel,
			Type:    "update",
			Data:    []byte(`[{"symbol":"BTC/EUR","side":"buy","price":1,"qty":1,"ord_type":"market","trade_id":1,"timestamp":"2026-05-31T00:00:00Z"}]`),
		}

		rows, err := envelope.SplitDataRows()

		convey.Convey("It should split rows and preserve envelope type", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(rows), convey.ShouldEqual, 1)
			convey.So(rows[0].Channel, convey.ShouldEqual, TradesChannel)
			convey.So(rows[0].Type, convey.ShouldEqual, "update")
			convey.So(string(rows[0].Data), convey.ShouldContainSubstring, `"symbol":"BTC/EUR"`)
		})
	})
}

func BenchmarkSocketMessageSplitDataRows(b *testing.B) {
	envelope := &SocketMessage{
		Channel: BookChannel,
		Type:    "snapshot",
		Data:    []byte(`[{"symbol":"BTC/EUR","bids":[{"price":1,"qty":1}],"asks":[{"price":2,"qty":1}]}]`),
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = envelope.SplitDataRows()
	}
}
