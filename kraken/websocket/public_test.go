package websocket

import (
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/tests/fixtures/instrument"

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
			for payload := range instrument.NewFixture(instrument.SNAPSHOT, 1).Generate() {
				public.receive(payload)
			}

			Convey("It should publish online symbols for the configured quote", func() {
				select {
				case observed := <-symbols:
					So(observed, ShouldResemble, []string{"BTC/USD", "MATIC/USD"})
				case <-time.After(time.Second):
					t.Fatal("instrument symbols were not published")
				}
			})
		})
	})
}

func TestPublicObserveInstrument(t *testing.T) {
	Convey("Given a public websocket instrument observer", t, func() {
		public := &Public{
			client: spot.NewWebSocket(),
			quote:  "USD",
			depth:  10,
			buffer: 1,
		}
		instruments := public.Observe("instrument")

		Convey("When an instrument snapshot arrives", func() {
			for payload := range instrument.NewFixture(instrument.SNAPSHOT, 1).Generate() {
				public.receive(payload)
			}

			Convey("It should route the instrument data payload", func() {
				select {
				case observed := <-instruments:
					var frame map[string]any
					So(sonic.Unmarshal(observed, &frame), ShouldBeNil)
					So(frame["pairs"], ShouldNotBeEmpty)
				case <-time.After(time.Second):
					t.Fatal("instrument data was not routed")
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

func TestPublicObserveBook(t *testing.T) {
	Convey("Given a public websocket book observer", t, func() {
		public := &Public{buffer: 1}
		book := public.Observe("book")

		Convey("When a book frame arrives", func() {
			public.receive([]byte(`{
				"channel": "book",
				"type": "snapshot",
				"data": [{
					"symbol": "BTC/USD",
					"bids": [{"price": 100.0, "qty": 1.0}],
					"asks": [{"price": 101.0, "qty": 1.0}]
				}]
			}`))

			Convey("It should route the full book frame so row type is preserved", func() {
				select {
				case observed := <-book:
					var frame map[string]any
					So(sonic.Unmarshal(observed, &frame), ShouldBeNil)
					So(frame["channel"], ShouldEqual, "book")
					So(frame["type"], ShouldEqual, "snapshot")
				case <-time.After(time.Second):
					t.Fatal("book data was not routed")
				}
			})
		})
	})
}
