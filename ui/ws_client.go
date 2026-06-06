package ui

import (
	"sync"

	"github.com/fasthttp/websocket"
)

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

	return client.conn.WriteJSON(payload)
}

func (client *wsClient) close() error {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()

	return client.conn.Close()
}
