package transport_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/transport"
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

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			for {
				msgType, p, err := conn.ReadMessage()
				if err != nil {
					return
				}

				var req map[string]any
				if err := json.Unmarshal(p, &req); err != nil {
					continue
				}

				if req["method"] == "subscribe" {
					// Send status update first
					statusFrame := map[string]any{
						"channel": "status",
						"type":    "update",
					}
					statusBytes, _ := json.Marshal(statusFrame)
					_ = conn.WriteMessage(msgType, statusBytes)

					// Send instrument snapshot
					snapshot := transport.InstrumentSnapshotMessage{
						Channel: "instrument",
						Type:    "snapshot",
						Data: transport.InstrumentSnapshotData{
							Pairs: []transport.InstrumentPair{
								{Symbol: "BTC/USD", Base: "BTC", Quote: "USD", Status: "online"},
								{Symbol: "ETH/USD", Base: "ETH", Quote: "USD", Status: "online"},
								{Symbol: "USDT/USD", Base: "USDT", Quote: "USD", Status: "online"},
								{Symbol: "SOL/EUR", Base: "SOL", Quote: "EUR", Status: "online"},
								{Symbol: "XRP/USD", Base: "XRP", Quote: "USD", Status: "maintenance"},
								{Symbol: "ADA/USD", Base: "ADA", Quote: "USD", Status: "online"},
							},
						},
					}
					snapBytes, _ := json.Marshal(snapshot)
					_ = conn.WriteMessage(msgType, snapBytes)
					return
				}
			}
		})

		server := &http.Server{Handler: handler}
		go func() {
			_ = server.Serve(listener)
		}()
		defer server.Close()

		dialer := &websocket.Dialer{
			NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return listener.Dial()
			},
		}

		Convey("When discovering universe for quote USD excluding USDT", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			symbols, err := transport.DiscoverUniverseWithDialer(
				ctx, dialer, "ws://in-memory/v2", "USD", []string{"USDT"},
			)

			Convey("Then only online USD pairs not in excluded are returned", func() {
				So(err, ShouldBeNil)
				So(symbols, ShouldResemble, []string{"ADA/USD", "BTC/USD", "ETH/USD"})
			})
		})
	})
}
