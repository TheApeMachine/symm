package network

import (
	"context"
	"errors"
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
	ctx := t.Context()

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
	ctx := t.Context()

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

func TestWebsocketOnReconnectHookFailureRedials(t *testing.T) {
	if system.Cfg == nil {
		system.Cfg = &system.Config{WebSocket: system.NewWebSocket()}
	}
	if system.Cfg.WebSocket == nil {
		system.Cfg.WebSocket = system.NewWebSocket()
	}

	upgrader := gorilla.Upgrader{}

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
	ctx := t.Context()

	client := NewWebsocketClient(ctx)

	if err := client.Open(wsURL); err != nil {
		t.Fatalf("open: %v", err)
	}

	resubscribe := errors.New("resubscribe rejected")
	var hooks atomic.Int32
	client.OnReconnect(func() error {
		if hooks.Add(1) == 1 {
			return resubscribe
		}

		return nil
	})

	_ = client.softDown(context.Canceled)
	client.backoff = 5 * time.Millisecond

	err := client.ensureReady()
	if err == nil || !errors.Is(err, resubscribe) {
		t.Fatalf("ensureReady must return the hook failure, got %v", err)
	}
	if client.Context().Err() != nil {
		t.Fatalf("a failed reconnect hook must not close the client")
	}
	if client.Status() != runtime.WAITING {
		t.Fatalf("a failed reconnect hook must leave the client waiting to redial, got %v", client.Status())
	}
	if client.hookBackoff < time.Second {
		t.Fatalf("a failed reconnect hook must back off before redialing, got %v", client.hookBackoff)
	}

	if err := client.ensureReady(); err != nil {
		t.Fatalf("the next attempt must redial and resubscribe, got %v", err)
	}
	if hooks.Load() != 2 || client.Status() != runtime.READY {
		t.Fatalf("expected a second, successful hook on READY, got %d hooks, %v", hooks.Load(), client.Status())
	}
	if client.hookBackoff != 0 {
		t.Fatalf("a successful hook must clear its backoff, got %v", client.hookBackoff)
	}
}

func TestWebsocketOnDisconnectHook(t *testing.T) {
	if system.Cfg == nil {
		system.Cfg = &system.Config{WebSocket: system.NewWebSocket()}
	}
	if system.Cfg.WebSocket == nil {
		system.Cfg.WebSocket = system.NewWebSocket()
	}

	upgrader := gorilla.Upgrader{}

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
	ctx := t.Context()

	client := NewWebsocketClient(ctx)
	var drops atomic.Int32
	client.OnDisconnect(func() { drops.Add(1) })

	if err := client.Open(wsURL); err != nil {
		t.Fatalf("open: %v", err)
	}

	_ = client.softDown(context.Canceled)
	if drops.Load() != 1 {
		t.Fatalf("a live drop must run the disconnect hook once, got %d", drops.Load())
	}

	_ = client.softDown(context.Canceled)
	if drops.Load() != 1 {
		t.Fatalf("a drop with no live connection must not re-run the hook, got %d", drops.Load())
	}
}
