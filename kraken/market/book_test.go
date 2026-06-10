package market

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/types"
)

func TestBookUpdatesUnmarshal(t *testing.T) {
	Convey("Given a Kraken book frame with array data", t, func() {
		message := types.NewSocketMessage()
		message.Type = "snapshot"
		message.Data = json.RawMessage(`[{
			"symbol":"BTC/USD",
			"bids":[{"price":100,"qty":1}],
			"asks":[{"price":101,"qty":2}],
			"checksum":123,
			"timestamp":"2024-01-01T00:00:00Z"
		}]`)

		updates := BookUpdates{}

		Convey("It should decode each book update and stamp envelope type", func() {
			So(updates.Unmarshal(message), ShouldBeNil)
			So(len(updates), ShouldEqual, 1)
			So(updates[0].Symbol, ShouldEqual, "BTC/USD")
			So(updates[0].Type, ShouldEqual, "snapshot")
			So(updates[0].Bids[0].Price, ShouldEqual, 100)
			So(updates[0].Timestamp, ShouldEqual, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		})
	})
}
