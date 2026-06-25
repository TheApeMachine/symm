package public

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	wswebsocket "github.com/fasthttp/websocket"
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

			_, payload, err := conn.ReadMessage()

			if err != nil {
				t.Errorf("read failed: %v", err)
				return
			}

			received <- payload
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
