package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
)

func TestBroadcastJSONIsConcurrentSafe(t *testing.T) {
	server := httptest.NewServer(httpHandler(t))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)

	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	defer conn.Close()

	client := newWSClient(conn)
	const writers = 32
	const frames = 64

	var waitGroup sync.WaitGroup
	waitGroup.Add(writers)

	for writer := 0; writer < writers; writer++ {
		go func() {
			defer waitGroup.Done()

			for frame := 0; frame < frames; frame++ {
				payload := map[string]any{
					"event": "test",
					"frame": frame,
				}

				if err := client.writeJSON(payload); err != nil {
					return
				}
			}
		}()
	}

	done := make(chan struct{})

	go func() {
		waitGroup.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent writes did not finish")
	}
}

func httpHandler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(writer http.ResponseWriter, request *http.Request) {
		conn, err := wsUpgrader.Upgrade(writer, request, nil)

		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}

		defer conn.Close()

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}
}
