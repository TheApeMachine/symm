package mockapi

import (
	"io"
	"sync"
)

/*
outbound is one queued channel payload awaiting an explicit Drain.
*/
type outbound struct {
	channel string
	payload []byte
}

/*
queue holds frames that Publish records until Drain delivers them through Emit.
*/
type queue struct {
	mu     sync.Mutex
	frames []outbound
	closed bool
}

/*
Active reports whether the connection still accepts writes and drains.
*/
func (conn *MockConn) Active() bool {
	if conn == nil {
		return false
	}

	conn.queue.mu.Lock()
	defer conn.queue.mu.Unlock()
	return !conn.queue.closed
}

/*
Queue records one frame for a later Drain.
*/
func (conn *MockConn) Queue(channel string, payload []byte) error {
	if conn == nil {
		return io.ErrClosedPipe
	}

	conn.queue.mu.Lock()
	defer conn.queue.mu.Unlock()

	if conn.queue.closed {
		return io.ErrClosedPipe
	}

	conn.queue.frames = append(conn.queue.frames, outbound{
		channel: channel,
		payload: append([]byte(nil), payload...),
	})
	return nil
}

func (conn *MockConn) closeQueue() {
	conn.queue.mu.Lock()
	conn.queue.closed = true
	conn.queue.frames = nil
	conn.queue.mu.Unlock()
}
