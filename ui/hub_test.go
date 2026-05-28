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

	for index := range 32 {
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

	for index := range 32 {
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

func TestHubReplaysWalletSnapshotOnConnect(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	hub, err := NewHub(ctx, pool)

	if err != nil {
		t.Fatalf("new hub: %v", err)
	}

	t.Cleanup(func() { _ = hub.Close() })

	hub.broadcasts["wallet"].Send(&qpool.QValue[any]{Value: map[string]any{
		"Currency":    "EUR",
		"Balance":     200.0,
		"ReservedEUR": 0.0,
		"FeePct":      0.26,
		"Inventory":   map[string]float64{},
	}})

	time.Sleep(5 * time.Millisecond)

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

	_, payload, err = conn.ReadMessage()

	if err != nil {
		t.Fatalf("read wallet snapshot: %v", err)
	}

	var wallet map[string]any

	if err := json.Unmarshal(payload, &wallet); err != nil {
		t.Fatalf("decode wallet json: %v", err)
	}

	if wallet["Balance"] != 200.0 {
		t.Fatalf("expected wallet balance 200, got %#v", wallet["Balance"])
	}
}

func TestHubReplaysWalletAndMarksOnReconnect(t *testing.T) {
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
	firstConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)

	if err != nil {
		t.Fatalf("dial first websocket: %v", err)
	}

	if frame := readSocketJSON(t, firstConn); frame["event"] != "hello" {
		t.Fatalf("expected first hello, got %#v", frame)
	}

	hub.broadcasts["wallet"].Send(&qpool.QValue[any]{Value: map[string]any{
		"Currency":    "EUR",
		"Balance":     193.69,
		"ReservedEUR": 6.31,
		"FeePct":      0.26,
		"Inventory":   map[string]float64{"H": 26.29},
		"AvgEntry":    map[string]float64{"H": 0.24},
		"Marks":       map[string]float64{"H/EUR": 0.245},
	}})
	hub.broadcasts["ui"].Send(&qpool.QValue[any]{Value: map[string]any{
		"event":  "mark",
		"ts":     "2026-05-28T01:10:10Z",
		"symbol": "H/EUR",
		"price":  0.245,
	}})

	seenWallet := false
	seenMark := false
	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) && (!seenWallet || !seenMark) {
		frame := readSocketJSON(t, firstConn)
		seenWallet = seenWallet || frame["Balance"] == 193.69
		seenMark = seenMark || frame["event"] == "mark" && frame["symbol"] == "H/EUR"
	}

	if !seenWallet || !seenMark {
		t.Fatalf("expected first connection to receive wallet=%v mark=%v", seenWallet, seenMark)
	}

	if err := firstConn.Close(); err != nil {
		t.Fatalf("close first websocket: %v", err)
	}

	secondConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)

	if err != nil {
		t.Fatalf("dial second websocket: %v", err)
	}

	t.Cleanup(func() { _ = secondConn.Close() })

	if frame := readSocketJSON(t, secondConn); frame["event"] != "hello" {
		t.Fatalf("expected reconnect hello, got %#v", frame)
	}

	wallet := readUntilSocketJSON(t, secondConn, func(frame map[string]any) bool {
		return frame["Balance"] == 193.69
	})

	if wallet["ReservedEUR"] != 6.31 {
		t.Fatalf("expected reserved EUR replay, got %#v", wallet["ReservedEUR"])
	}

	mark := readUntilSocketJSON(t, secondConn, func(frame map[string]any) bool {
		return frame["event"] == "mark" && frame["symbol"] == "H/EUR"
	})

	if mark["price"] != 0.245 {
		t.Fatalf("expected H/EUR mark replay, got %#v", mark["price"])
	}
}

func TestHubConnectDuringBroadcast(t *testing.T) {
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

	stop := make(chan struct{})
	var flood sync.WaitGroup
	flood.Add(1)

	go func() {
		defer flood.Done()

		for {
			select {
			case <-stop:
				return
			default:
				hub.broadcasts["wallet"].Send(&qpool.QValue[any]{Value: map[string]any{
					"Currency":    "EUR",
					"Balance":     199.0,
					"ReservedEUR": 0.0,
					"FeePct":      0.26,
					"Inventory":   map[string]float64{"BTC": 0.01},
				}})
				hub.broadcasts["confidence"].Send(&qpool.QValue[any]{Value: map[string]any{
					"source":     "hawkes",
					"confidence": 0.5,
				}})
				hub.broadcasts["feedback"].Send(&qpool.QValue[any]{Value: map[string]any{
					"Source":          "hawkes",
					"Symbol":          "BTC/EUR",
					"PredictedReturn": 0.01,
					"ActualReturn":    0.008,
					"Error":           0.002,
				}})
			}
		}
	}()

	t.Cleanup(func() {
		close(stop)
		flood.Wait()
	})

	var dials sync.WaitGroup

	for range 16 {
		dials.Add(1)

		go func() {
			defer dials.Done()

			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)

			if err != nil {
				t.Errorf("dial hub websocket: %v", err)
				return
			}

			defer conn.Close()

			deadline := time.Now().Add(200 * time.Millisecond)

			for time.Now().Before(deadline) {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
	}

	dials.Wait()
}

func httpHandler(hub *Hub) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.handleWS)

	return mux
}

func readSocketJSON(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	_, payload, err := conn.ReadMessage()

	if err != nil {
		t.Fatalf("read websocket frame: %v", err)
	}

	var frame map[string]any

	if err := json.Unmarshal(payload, &frame); err != nil {
		t.Fatalf("decode websocket frame: %v", err)
	}

	return frame
}

func readUntilSocketJSON(
	t *testing.T,
	conn *websocket.Conn,
	matches func(map[string]any) bool,
) map[string]any {
	t.Helper()

	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		frame := readSocketJSON(t, conn)

		if matches(frame) {
			return frame
		}
	}

	t.Fatal("timed out waiting for matching websocket frame")

	return nil
}
