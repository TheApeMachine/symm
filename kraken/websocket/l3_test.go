package websocket

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"

	. "github.com/smartystreets/goconvey/convey"
)

func TestL3Subscribe(t *testing.T) {
	Convey("Given an unauthenticated level3 websocket", t, func() {
		l3 := &L3{
			client: spot.NewWebSocket(),
			depth:  10,
		}

		Convey("When symbols are registered before authentication", func() {
			l3.Subscribe([]string{"BTC/USD"})

			Convey("It should retain symbols without subscribing yet", func() {
				So(l3.symbols, ShouldResemble, []string{"BTC/USD"})
				So(l3.subscribed, ShouldBeFalse)
			})
		})
	})
}

func TestL3Receive(t *testing.T) {
	Convey("Given a level3 websocket observer", t, func() {
		restore := errnie.SuppressLogging()
		defer restore()

		l3 := &L3{
			observers: map[string][]chan []byte{},
			buffer:    2,
		}
		level3 := l3.Observe("level3")

		Convey("When a non-market-data level3 frame arrives", func() {
			l3.receive([]byte(`{"channel":"level3","type":"subscription_ack"}`))

			Convey("Then it should not publish a level3 payload", func() {
				select {
				case observed := <-level3:
					t.Fatalf("control frame published as level3 data: %s", observed)
				default:
				}
			})
		})

		Convey("When a level3 update arrives", func() {
			l3.receive([]byte(`{
				"channel": "level3",
				"type": "update",
				"data": [{
					"symbol": "BTC/USD",
					"checksum": 1,
					"bids": [],
					"asks": []
				}]
			}`))

			Convey("Then it should publish the level3 data payload", func() {
				select {
				case observed := <-level3:
					So(string(observed), ShouldContainSubstring, "BTC/USD")
				case <-time.After(time.Second):
					t.Fatal("level3 data was not routed")
				}
			})
		})
	})
}

func BenchmarkL3Receive(benchmarkTB *testing.B) {
	l3 := &L3{
		observers: map[string][]chan []byte{},
		buffer:    64,
	}
	level3 := l3.Observe("level3")
	frame := []byte(`{
		"channel": "level3",
		"type": "update",
		"data": [{
			"symbol": "BTC/USD",
			"checksum": 1,
			"bids": [],
			"asks": []
		}]
	}`)

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		l3.receive(frame)
		<-level3
	}
}
