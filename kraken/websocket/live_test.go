package websocket

import (
	"strconv"
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
		live.client.URL = "wss://ws-auth.kraken.com/v2"

		live.client.OnAuthenticated.Recurring(func(event *callback.Event[string]) {
			live.status = types.READY
		})

		live.client.OnAuthenticated.Call("token")

		Convey("It should become ready after authentication", func() {
			So(live.client.URL, ShouldEqual, "wss://ws-auth.kraken.com/v2")
			So(live.status, ShouldEqual, types.READY)
		})
	})
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
