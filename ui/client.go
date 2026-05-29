package ui

import (
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/symm/runstats"
)

/*
wsClient owns one connected browser with a bounded lossy outbox.
*/
type wsClient struct {
	conn   *websocket.Conn
	out    chan any
	done   chan struct{}
	closed atomic.Bool
}

func newClient(conn *websocket.Conn) *wsClient {
	return &wsClient{
		conn: conn,
		out:  make(chan any, perClientBuffer),
		done: make(chan struct{}),
	}
}

func (client *wsClient) close() error {
	if client.closed.Swap(true) {
		return nil
	}

	close(client.done)
	return client.conn.Close()
}

func (client *wsClient) enqueue(payload any) bool {
	if client.closed.Load() {
		return false
	}

	select {
	case client.out <- payload:
		runstats.UIFramesSent(1)
		return true
	case <-client.done:
		return false
	default:
		runstats.UIFramesDropped(1)
		return true
	}
}

func (client *wsClient) enqueuePriority(payload any) bool {
	if client.closed.Load() {
		return false
	}

	select {
	case client.out <- payload:
		runstats.UIFramesSent(1)
		return true
	case <-client.done:
		return false
	default:
	}

	select {
	case <-client.out:
		runstats.UIFramesDropped(1)
	default:
	}

	select {
	case client.out <- payload:
		runstats.UIFramesSent(1)
		return true
	case <-client.done:
		return false
	default:
		runstats.UIFramesDropped(1)
		return true
	}
}

func (client *wsClient) runWriter() {
	for {
		select {
		case <-client.done:
			return
		case payload := <-client.out:
			_ = client.conn.SetWriteDeadline(time.Now().Add(writeDeadline))

			if err := client.conn.WriteJSON(payload); err != nil {
				_ = client.close()
				return
			}
		}
	}
}
