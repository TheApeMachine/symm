package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/qpool"
)

func TestHandleWSHello(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	hub, err := NewHub(ctx, pool)

	if err != nil {
		t.Fatalf("new hub: %v", err)
	}

	t.Cleanup(func() { _ = hub.Close() })

	server := httptest.NewServer(httpHandler(hub))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)

	if err != nil {
		t.Fatalf("dial hub websocket: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close() })

	_, payload, err := conn.ReadMessage()

	if err != nil {
		t.Fatalf("read hello frame: %v", err)
	}

	var hello map[string]any

	if err := json.Unmarshal(payload, &hello); err != nil {
		t.Fatalf("decode hello json: %v", err)
	}

	if hello["event"] != "hello" {
		t.Fatalf("expected hello event, got %#v", hello["event"])
	}
}

func TestHubConcurrentBroadcasts(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	hub, err := NewHub(ctx, pool)

	if err != nil {
		t.Fatalf("new hub: %v", err)
	}

	t.Cleanup(func() { _ = hub.Close() })

	server := httptest.NewServer(httpHandler(hub))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)

	if err != nil {
		t.Fatalf("dial hub websocket: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close() })

	_, _, err = conn.ReadMessage()

	if err != nil {
		t.Fatalf("read hello frame: %v", err)
	}

	var writers sync.WaitGroup

	for index := 0; index < 32; index++ {
		writers.Add(1)

		go func(value int) {
			defer writers.Done()

			hub.broadcasts["confidence"].Send(&qpool.QValue[any]{Value: map[string]any{
				"source":     "hawkes",
				"confidence": float64(value) / 32,
				"count":      1,
			}})
		}(index)
	}

	writers.Wait()
	time.Sleep(100 * time.Millisecond)

	for index := 0; index < 32; index++ {
		_, payload, err := conn.ReadMessage()

		if err != nil {
			t.Fatalf("read broadcast frame %d: %v", index, err)
		}

		var row map[string]any

		if err := json.Unmarshal(payload, &row); err != nil {
			t.Fatalf("decode broadcast json %d: %v", index, err)
		}

		if row["source"] != "hawkes" {
			t.Fatalf("expected hawkes source, got %#v", row["source"])
		}
	}
}

func httpHandler(hub *Hub) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.handleWS)

	return mux
}
