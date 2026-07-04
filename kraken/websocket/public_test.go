package websocket

import (
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPublicSymbols(t *testing.T) {
	Convey("Given a public websocket watching instrument symbols", t, func() {
		previousBatch := viper.GetInt("market.subscribe_batch")
		viper.Set("market.subscribe_batch", 2)
		defer viper.Set("market.subscribe_batch", previousBatch)

		public := &Public{
			client: spot.NewWebSocket(),
			quote:  "USD",
			depth:  10,
			buffer: 1,
		}
		symbols := public.Symbols()

		Convey("When an instrument snapshot arrives", func() {
			public.receive([]byte(`{
				"channel": "instrument",
				"type": "snapshot",
				"data": {
					"pairs": [
						{"symbol": "BTC/USD", "status": "online", "quote": "USD"},
						{"symbol": "ETH/EUR", "status": "online", "quote": "EUR"},
						{"symbol": "SOL/USD", "status": "cancel_only", "quote": "USD"}
					]
				}
			}`))

			Convey("It should publish online symbols for the configured quote", func() {
				select {
				case observed := <-symbols:
					So(observed, ShouldResemble, []string{"BTC/USD"})
				case <-time.After(time.Second):
					t.Fatal("instrument symbols were not published")
				}
			})
		})
	})
}

func TestPublicObserve(t *testing.T) {
	Convey("Given public websocket observers by channel", t, func() {
		public := &Public{buffer: 1}
		ticker := public.Observe("ticker")
		trade := public.Observe("trade")

		Convey("When a ticker frame arrives", func() {
			public.receive([]byte(`{
				"channel": "ticker",
				"type": "update",
				"data": [{"symbol": "BTC/USD", "last": 100.0}]
			}`))

			Convey("It should route only the ticker data payload", func() {
				select {
				case observed := <-ticker:
					var rows []map[string]any
					So(sonic.Unmarshal(observed, &rows), ShouldBeNil)
					So(len(rows), ShouldEqual, 1)
					So(rows[0]["symbol"], ShouldEqual, "BTC/USD")
				case <-time.After(time.Second):
					t.Fatal("ticker data was not routed")
				}

				select {
				case observed := <-trade:
					t.Fatalf("trade received %s", observed)
				default:
				}
			})
		})
	})
}
