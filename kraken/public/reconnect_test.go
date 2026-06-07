package public

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
)

func TestOnReconnectNotifiesHandlers(t *testing.T) {
	var calls atomic.Int32

	OnReconnect(func() {
		calls.Add(1)
	})

	notifyReconnect()

	if calls.Load() != 1 {
		t.Fatalf("notifyReconnect calls = %d, want 1", calls.Load())
	}
}

func TestReconnectBackoffCapsAtThirtySeconds(t *testing.T) {
	if reconnectBackoff(0) != 0 {
		t.Fatal("attempt 0 should not delay")
	}

	if reconnectBackoff(20) <= reconnectBackoff(1) {
		t.Fatal("backoff should grow with attempt count")
	}

	if reconnectBackoff(100) != 30*time.Second {
		t.Fatalf("backoff cap = %v, want 30s", reconnectBackoff(100))
	}
}

func TestWebSocketMarkDisconnected(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ws := &WebSocket{
		ctx:         ctx,
		conns:       []*websocket.Conn{nil},
		isConnected: true,
	}

	ws.markDisconnected()

	if ws.isConnected {
		t.Fatal("expected disconnected after markDisconnected")
	}
}
