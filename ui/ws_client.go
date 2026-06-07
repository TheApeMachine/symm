package ui

import (
	"sync"
	"time"

	"github.com/fasthttp/websocket"
)

// wsWriteDeadline bounds one frame write. The hub fans out synchronously from
// its Tick goroutine; without a deadline one stalled browser connection (sleeping
// laptop, dead Wi-Fi) blocked every other client's frames behind it.
const wsWriteDeadline = 2 * time.Second

/*
wsClient wraps one browser websocket with a write lock. fasthttp/websocket forbids
concurrent WriteJSON on the same connection; the hub fans out from multiple goroutines
(Tick, diagnostics watcher), so every outbound frame must pass through here.
*/
type wsClient struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func newWSClient(conn *websocket.Conn) *wsClient {
	return &wsClient{conn: conn}
}

func (client *wsClient) writeJSON(payload any) error {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()

	_ = client.conn.SetWriteDeadline(time.Now().Add(wsWriteDeadline))

	return client.conn.WriteJSON(payload)
}

func (client *wsClient) close() error {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()

	return client.conn.Close()
}
