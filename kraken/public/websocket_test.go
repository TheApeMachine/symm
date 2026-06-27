package public

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	wswebsocket "github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func TestOnMessageRequiresConnection(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	socket := NewWebSocket(ctx, pool, dmt.NewTree(""))
	defer socket.Close()

	payload := []byte(`{"method":"subscribe","params":{"channel":"ticker"}}`)
	artifact := datura.Acquire("hub", datura.APPJSON).
		WithDestination("kraken:public").
		WithPayload(payload)

	err := socket.onMessage(artifact)

	if err == nil {
		t.Fatal("onMessage should fail when websocket is not connected")
	}
}

func TestOnMessageSendsAfterConnection(t *testing.T) {
	t.Parallel()

	received := make(chan []byte, 1)
	upgrader := wswebsocket.Upgrader{
		CheckOrigin: func(request *http.Request) bool {
			return true
		},
	}

	server := httptest.NewServer(http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			conn, err := upgrader.Upgrade(response, request, nil)

			if err != nil {
				t.Errorf("upgrade failed: %v", err)
				return
			}

			defer conn.Close()

			// subscribeAll sends the instrument subscribe on connect; relay
			// reads until it sees the payload pushed via onMessage.
			for {
				_, payload, err := conn.ReadMessage()

				if err != nil {
					t.Errorf("read failed: %v", err)
					return
				}

				if strings.Contains(string(payload), `"channel":"ticker"`) {
					received <- payload
					return
				}
			}
		},
	))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	socket := NewWebSocket(ctx, pool, dmt.NewTree(""))
	socket.connectMaxDelay = 2
	defer socket.Close()

	endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))

	if err := socket.Connect(endpoint, 1); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	payload := []byte(`{"method":"subscribe","params":{"channel":"ticker"}}`)
	artifact := datura.Acquire("hub", datura.APPJSON).
		WithDestination("kraken:public").
		WithPayload(payload)

	if err := socket.onMessage(artifact); err != nil {
		t.Fatalf("onMessage failed: %v", err)
	}

	select {
	case wire := <-received:
		if string(wire) != string(payload) {
			t.Fatalf("received %q, want %q", wire, payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not receive websocket payload")
	}
}

func TestConnectHandlesNilHTTPResponse(t *testing.T) {
	originalDial := dialWebSocket
	dialWebSocket = func(string, http.Header) (*wswebsocket.Conn, *http.Response, error) {
		return nil, nil, errors.New("dial failed before response")
	}
	t.Cleanup(func() {
		dialWebSocket = originalDial
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	socket := NewWebSocket(ctx, pool, dmt.NewTree(""))
	defer socket.Close()

	err := socket.Connect(WebSocketURL, 1)

	if err == nil {
		t.Fatal("connect should fail when context expires")
	}
	if socket.isConnected.Load() {
		t.Fatal("socket should not be marked connected")
	}
}

func TestConnectRetriesAfterInitialFailure(t *testing.T) {
	viper.Set("market.ws_reconnect_initial", time.Millisecond)
	viper.Set("market.ws_reconnect_max", time.Millisecond)
	viper.Set("market.ws_reconnect_multiplier", 1)
	t.Cleanup(viper.Reset)

	received := make(chan []byte, 1)
	upgrader := wswebsocket.Upgrader{
		CheckOrigin: func(request *http.Request) bool {
			return true
		},
	}

	server := httptest.NewServer(http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			conn, err := upgrader.Upgrade(response, request, nil)
			if err != nil {
				t.Errorf("upgrade failed: %v", err)
				return
			}
			defer conn.Close()

			_, payload, err := conn.ReadMessage()
			if err != nil {
				t.Errorf("read failed: %v", err)
				return
			}

			received <- payload
		},
	))
	defer server.Close()

	originalDial := dialWebSocket
	attempts := 0
	dialWebSocket = func(endpoint string, header http.Header) (*wswebsocket.Conn, *http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, nil, errors.New("temporary dial failure")
		}
		return originalDial(endpoint, header)
	}
	t.Cleanup(func() {
		dialWebSocket = originalDial
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	socket := NewWebSocket(ctx, pool, dmt.NewTree(""))
	defer socket.Close()

	endpoint := EndpointType("ws" + strings.TrimPrefix(server.URL, "http"))

	if err := socket.Connect(endpoint, 1); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	if attempts < 2 {
		t.Fatalf("attempts = %d, want retry", attempts)
	}

	select {
	case payload := <-received:
		if !strings.Contains(string(payload), `"channel": "instrument"`) &&
			!strings.Contains(string(payload), `"channel":"instrument"`) {
			t.Fatalf("subscription payload = %s, want instrument subscribe", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not receive instrument subscription")
	}
}
