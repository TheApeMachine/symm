package websocket

import (
	"fmt"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestNewAPISharesAuthToken(t *testing.T) {
	Convey("Given private and level3 live transports", t, func() {
		sharedREST := spot.NewREST()
		private := &Live{
			status: types.INITIALIZING,
			client: spot.NewWebSocket(),
			auth:   true,
		}
		private.client.REST = sharedREST

		level3 := &Live{
			status:           types.INITIALIZING,
			client:           spot.NewWebSocket(),
			auth:             true,
			skipAuthenticate: true,
		}
		level3.client.REST = sharedREST

		public := &stubConn{}
		NewAPI(public, private, level3)

		private.Client().OnAuthenticated.Call("shared-token")

		Convey("It should copy the websocket token to level3", func() {
			So(level3.Client().Token, ShouldEqual, "shared-token")
			So(level3.status, ShouldEqual, types.PENDING)
		})
	})
}

func TestNewSharesAuthREST(t *testing.T) {
	Convey("Given two authenticated live transports", t, func() {
		sharedAuthREST = nil
		defer func() { sharedAuthREST = nil }()

		private := &Live{
			client: spot.NewWebSocket(),
			auth:   true,
		}
		private.client.REST.BaseURL = "https://api.kraken.com"

		sharedAuthRESTLock.Lock()

		if sharedAuthREST == nil {
			sharedAuthREST = private.client.REST
		}

		sharedAuthRESTLock.Unlock()

		level3 := &Live{
			client: spot.NewWebSocket(),
			auth:   true,
		}
		level3.client.REST.BaseURL = "https://api.kraken.com"

		sharedAuthRESTLock.Lock()

		if sharedAuthREST == nil {
			sharedAuthREST = level3.client.REST
		}

		if level3.client.REST != sharedAuthREST {
			level3.client.REST = sharedAuthREST
			level3.skipAuthenticate = true
		}

		sharedAuthRESTLock.Unlock()

		Convey("It should keep one REST client for both transports", func() {
			So(private.Client().REST, ShouldEqual, sharedAuthREST)
			So(level3.Client().REST, ShouldEqual, sharedAuthREST)
			So(level3.skipAuthenticate, ShouldBeTrue)
		})
	})
}

func TestAuthNonceMonotonic(t *testing.T) {
	Convey("Given an auth nonce offset from Kraken server time", t, func() {
		authNonceOffset = 5
		authNonceUnix = 0
		authNonceSeq = 0

		rest := spot.NewREST()
		rest.Nonce = func() string {
			authNonceLock.Lock()
			defer authNonceLock.Unlock()

			currentUnix := time.Now().Unix() + authNonceOffset

			if currentUnix != authNonceUnix {
				authNonceUnix = currentUnix
				authNonceSeq = 0
			}

			if authNonceSeq >= 999 {
				time.Sleep(time.Until(time.Now().Add(time.Second)))

				authNonceUnix = time.Now().Unix() + authNonceOffset
				authNonceSeq = 0
			}

			nonce := fmt.Sprintf("%d%03d", authNonceUnix, authNonceSeq)
			authNonceSeq++

			return nonce
		}

		first := rest.Nonce()
		second := rest.Nonce()

		Convey("It should keep nonces strictly increasing", func() {
			So(second > first, ShouldBeTrue)
		})
	})
}

func TestLiveOnConnectedSkipsAuthenticate(t *testing.T) {
	Convey("Given a level3 transport that defers token minting", t, func() {
		live := &Live{
			status:           types.INITIALIZING,
			client:           spot.NewWebSocket(),
			auth:             true,
			skipAuthenticate: true,
		}

		live.client.OnConnected.Recurring(func(event *callback.Event[any]) {
			if !live.auth {
				return
			}

			if live.skipAuthenticate {
				live.status = types.PENDING
				return
			}

			live.status = types.ERROR
		})

		live.client.OnConnected.Call(nil)

		Convey("It should wait for the shared token without authenticating", func() {
			So(live.status, ShouldEqual, types.PENDING)
			So(live.Client().Token, ShouldBeEmpty)
		})
	})
}
