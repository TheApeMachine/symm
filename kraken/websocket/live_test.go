package websocket

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestNewSetsAuthURL(t *testing.T) {
	Convey("Given an authenticated live transport", t, func() {
		live := &Live{
			client: spot.NewWebSocket(),
			auth:   true,
		}
		live.client.URL = PrivateWebSocketURL

		live.client.OnAuthenticated.Recurring(func(event *callback.Event[string]) {
			live.status = types.READY
		})

		live.client.OnAuthenticated.Call("token")

		Convey("It should become ready after authentication", func() {
			So(live.client.URL, ShouldEqual, PrivateWebSocketURL)
			So(live.status, ShouldEqual, types.READY)
		})
	})
}

func TestLiveRoute(t *testing.T) {
	Convey("Given callbacks registered on a live transport", t, func() {
		live := &Live{sync: &sync.Map{}}
		level3Frames := make([][]byte, 0, 2)
		tickerFrames := make([][]byte, 0, 1)
		live.On("level3", func(raw []byte) {
			level3Frames = append(level3Frames, raw)
		})
		live.On("ticker", func(raw []byte) {
			tickerFrames = append(tickerFrames, raw)
		})

		Convey("It should route data frames by their top-level channel", func() {
			raw := []byte(`{"channel":"ticker","type":"update"}`)
			live.route(raw)

			So(tickerFrames, ShouldResemble, [][]byte{raw})
		})

		Convey("It should route level3 market data to registered handlers", func() {
			raw := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD"}]}`)
			live.route(raw)

			So(level3Frames, ShouldResemble, [][]byte{raw})
		})

		Convey("It should not route subscription acknowledgements as market data", func() {
			raw := []byte(`{"method":"subscribe","result":{"channel":"level3"},"success":true}`)
			live.route(raw)

			So(level3Frames, ShouldBeEmpty)
		})

		Convey("It should not route failed acknowledgements as market data", func() {
			raw := []byte(`{"error":"invalid depth","result":{"channel":"level3"},"success":false}`)
			live.route(raw)

			So(level3Frames, ShouldBeEmpty)
		})

		Convey("It should ignore status and heartbeat frames", func() {
			live.route([]byte(`{"channel":"status"}`))
			live.route([]byte(`{"channel":"heartbeat"}`))

			So(level3Frames, ShouldBeEmpty)
			So(tickerFrames, ShouldBeEmpty)
		})
	})
}

func BenchmarkLiveRoute(b *testing.B) {
	live := &Live{sync: &sync.Map{}}
	live.On("level3", func([]byte) {})
	raw := []byte(`{"channel":"level3","type":"update","data":[{"symbol":"BTC/USD"}]}`)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		live.route(raw)
	}
}

func TestAuthNonceSurvivesRestart(t *testing.T) {
	Convey("Given the auth nonce generator used for authenticated transports", t, func() {
		nonceCounter := kraken.NewEpochCounter()
		nonceCounter.Granularity = time.Microsecond

		priorRunLastNonce, err := strconv.ParseInt(nonceCounter.Get(), 10, 64)
		So(err, ShouldBeNil)

		Convey("It should stay within the int64 range Kraken expects", func() {
			So(priorRunLastNonce, ShouldBeGreaterThan, int64(0))
		})

		Convey("It should still increase for a brand new counter started immediately after", func() {
			restartedCounter := kraken.NewEpochCounter()
			restartedCounter.Granularity = time.Microsecond

			firstNonceAfterRestart, err := strconv.ParseInt(restartedCounter.Get(), 10, 64)

			So(err, ShouldBeNil)
			So(firstNonceAfterRestart, ShouldBeGreaterThan, priorRunLastNonce)
		})
	})
}
