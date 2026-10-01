package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gorilla "github.com/gorilla/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

func TestWebsocketSoftDownDoesNotErrorFlood(t *testing.T) {
	if system.Cfg == nil {
		system.Cfg = &system.Config{WebSocket: system.NewWebSocket()}
	}
	if system.Cfg.WebSocket == nil {
		system.Cfg.WebSocket = system.NewWebSocket()
	}

	var dials atomic.Int32
	upgrader := gorilla.Upgrader{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		dials.Add(1)
		// Close immediately to simulate 1006 / peer reset.
		_ = conn.Close()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := NewWebsocketClient(ctx)
	client.backoff = 10 * time.Millisecond

	if err := client.Open(wsURL); err != nil {
		t.Fatalf("open: %v", err)
	}

	// First read should soft-fail without leaving System in ERROR.
	_, err := client.Read()
	if err == nil {
		t.Fatal("expected read error after peer close")
	}
	if client.Status() == runtime.ERROR {
		t.Fatalf("disconnect must not System.ERROR, got %v", client.Status())
	}

	// ensureReady should redial (second dial) without ERROR spam path.
	client.backoff = 10 * time.Millisecond
	_ = client.ensureReady()

	if dials.Load() < 2 {
		t.Fatalf("expected reconnect dial, got %d", dials.Load())
	}
	if client.Status() == runtime.ERROR {
		t.Fatalf("reconnect must not leave ERROR status")
	}
}

func TestWebsocketOnReconnectHook(t *testing.T) {
	if system.Cfg == nil {
		system.Cfg = &system.Config{WebSocket: system.NewWebSocket()}
	}
	if system.Cfg.WebSocket == nil {
		system.Cfg.WebSocket = system.NewWebSocket()
	}

	upgrader := gorilla.Upgrader{}
	var hooks atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := NewWebsocketClient(ctx)
	client.OnReconnect(func() error {
		hooks.Add(1)
		return nil
	})

	if err := client.Open(wsURL); err != nil {
		t.Fatalf("open: %v", err)
	}
	if hooks.Load() != 1 {
		t.Fatalf("open should run reconnect hook once, got %d", hooks.Load())
	}

	_ = client.softDown(context.Canceled)
	client.backoff = 5 * time.Millisecond
	if err := client.ensureReady(); err != nil {
		t.Fatalf("ensureReady: %v", err)
	}
	if hooks.Load() != 2 {
		t.Fatalf("redial should run hook again, got %d", hooks.Load())
	}
}
