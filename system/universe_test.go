package system_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type inMemoryListener struct {
	conns  chan net.Conn
	closed chan struct{}
}

func newInMemoryListener() *inMemoryListener {
	return &inMemoryListener{
		conns:  make(chan net.Conn, 16),
		closed: make(chan struct{}),
	}
}

func (l *inMemoryListener) Accept() (net.Conn, error) {
	select {
	case c, ok := <-l.conns:
		if !ok {
			return nil, net.ErrClosed
		}
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *inMemoryListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return nil
}

func (l *inMemoryListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
}

func (l *inMemoryListener) Dial() (net.Conn, error) {
	clientConn, serverConn := net.Pipe()
	l.conns <- serverConn
	return clientConn, nil
}

func TestDiscoverUniverse(t *testing.T) {
	Convey("Given an in-memory mock Kraken WebSocket server", t, func() {
		listener := newInMemoryListener()
		defer listener.Close()

		mockServer := &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer c.Close()

				for {
					_, msg, err := c.ReadMessage()
					if err != nil {
						return
					}

					var req map[string]any
					if err := json.Unmarshal(msg, &req); err == nil {
						if req["method"] == "subscribe" {
							snapshot := system.InstrumentSnapshotMessage{
								Channel: "instrument",
								Type:    "snapshot",
								Data: system.InstrumentSnapshotData{
									Pairs: []system.InstrumentPair{
										{Symbol: "BTC/USD", Base: "BTC", Quote: "USD", Status: "online"},
										{Symbol: "ETH/USD", Base: "ETH", Quote: "USD", Status: "online"},
										{Symbol: "SOL/USD", Base: "SOL", Quote: "USD", Status: "cancel_only"},
										{Symbol: "EUR/USD", Base: "EUR", Quote: "USD", Status: "online"},
										{Symbol: "USDT/USD", Base: "USDT", Quote: "USD", Status: "online"},
										{Symbol: "BTC/EUR", Base: "BTC", Quote: "EUR", Status: "online"},
									},
								},
							}
							resp, _ := json.Marshal(snapshot)
							_ = c.WriteMessage(websocket.TextMessage, resp)
						}
					}
				}
			}),
		}

		go func() {
			_ = mockServer.Serve(listener)
		}()
		defer mockServer.Close()

		customDialer := &websocket.Dialer{
			NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return listener.Dial()
			},
		}

		Convey("When discovering universe for quote USD excluding USDT", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			symbols, err := system.DiscoverUniverseWithDialer(
				ctx,
				customDialer,
				"ws://in-memory/v2",
				"USD",
				[]string{"USDT", "EUR"},
			)

			So(err, ShouldBeNil)
			So(symbols, ShouldResemble, []string{"BTC/USD", "ETH/USD"})
		})
	})
}
