package kraken_test

import (
	"context"
	"sync"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/venue/kraken"
)

type fakeConn struct {
	mu    sync.Mutex
	wrote [][]byte
}

func (conn *fakeConn) Read(...string) ([]byte, error) { return nil, nil }

func (conn *fakeConn) Write(buf []byte) error {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	conn.wrote = append(conn.wrote, append([]byte(nil), buf...))
	return nil
}

func (conn *fakeConn) last() map[string]any {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	if len(conn.wrote) == 0 {
		return nil
	}

	out := map[string]any{}
	_ = sonic.Unmarshal(conn.wrote[len(conn.wrote)-1], &out)
	return out
}

func withLive(f func(*kraken.Live, *fakeConn)) func() {
	return func() {
		conn := &fakeConn{}
		live := kraken.NewLive(context.Background(), conn, nil)
		f(live, conn)
	}
}

func TestLiveWrite(t *testing.T) {
	Convey("Setup", t, withLive(func(live *kraken.Live, conn *fakeConn) {
		// Force a deterministic token source without hitting REST.
		live = kraken.NewLive(context.Background(), conn, kraken.NewAuth())

		Convey("When the method is not private", func() {
			buf, err := sonic.Marshal(map[string]any{
				"method": "ping",
			})
			So(err, ShouldBeNil)
			So(live.Write(buf), ShouldBeNil)

			frame := conn.last()
			So(frame["method"], ShouldEqual, "ping")
			params, _ := frame["params"].(map[string]any)
			So(params, ShouldBeNil)
		})

		Convey("When a private method already carries a token", func() {
			buf, err := sonic.Marshal(map[string]any{
				"method": "add_order",
				"params": map[string]any{"token": "keep-me", "symbol": "AAA/USD"},
			})
			So(err, ShouldBeNil)
			So(live.Write(buf), ShouldBeNil)

			frame := conn.last()
			params := frame["params"].(map[string]any)
			So(params["token"], ShouldEqual, "keep-me")
			So(params["symbol"], ShouldEqual, "AAA/USD")
		})

		Convey("When private params are missing", func() {
			buf, err := sonic.Marshal(map[string]any{
				"method": "subscribe",
			})
			So(err, ShouldBeNil)

			// Token fetch will fail without credentials — that is the real
			// contract: private writes without a token must attempt auth.
			err = live.Write(buf)
			So(err, ShouldNotBeNil)
			So(conn.last(), ShouldBeNil)
		})
	}))
}

func TestLivePost(t *testing.T) {
	Convey("Setup", t, withLive(func(live *kraken.Live, _ *fakeConn) {
		Convey("When credentials are unavailable", func() {
			_, err := live.Post("/0/private/Balance", nil)
			So(err, ShouldNotBeNil)
		})
	}))
}
