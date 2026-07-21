package mockapi

import (
	"io"
	"sync"
)

type mockHandler struct {
	id uint64
	fn func([]byte)
}

type outbound struct {
	channel string
	payload []byte
}

/*
Scheduler owns deterministic handler registration and queued delivery without
goroutines, sleeps, or wall-clock races.
*/
type Scheduler struct {
	mu       sync.Mutex
	channels map[string][]mockHandler
	nextID   uint64
	queue    []outbound
	closed   bool
}

/*
newScheduler creates one active deterministic delivery queue.
*/
func newScheduler() *Scheduler {
	return &Scheduler{}
}

/*
On registers one exact channel handler and returns its subscription identity.
*/
func (scheduler *Scheduler) On(channel string, action func([]byte)) uint64 {
	if scheduler == nil || action == nil {
		return 0
	}

	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()

	if scheduler.closed {
		return 0
	}

	if scheduler.channels == nil {
		scheduler.channels = map[string][]mockHandler{}
	}

	scheduler.nextID++
	scheduler.channels[channel] = append(scheduler.channels[channel], mockHandler{
		id: scheduler.nextID,
		fn: action,
	})
	return scheduler.nextID
}

/*
Unsubscribe removes one exact registered handler.
*/
func (scheduler *Scheduler) Unsubscribe(channel string, id uint64) {
	if scheduler == nil || id == 0 {
		return
	}

	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	handlers := scheduler.channels[channel]
	next := make([]mockHandler, 0, len(handlers))

	for _, handler := range handlers {
		if handler.id != id {
			next = append(next, handler)
		}
	}

	scheduler.channels[channel] = next
}

/*
Emit immediately delivers one payload to current channel handlers.
*/
func (scheduler *Scheduler) Emit(channel string, payload []byte) {
	scheduler.mu.Lock()

	if scheduler.closed {
		scheduler.mu.Unlock()
		return
	}

	handlers := append([]mockHandler(nil), scheduler.channels[channel]...)
	scheduler.mu.Unlock()

	for _, handler := range handlers {
		handler.fn(payload)
	}
}

/*
Queue schedules one outbound frame for an explicit later drain.
*/
func (scheduler *Scheduler) Queue(channel string, payload []byte) error {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()

	if scheduler.closed {
		return io.ErrClosedPipe
	}

	scheduler.queue = append(scheduler.queue, outbound{
		channel: channel,
		payload: append([]byte(nil), payload...),
	})
	return nil
}

/*
Drain delivers every scheduled frame in insertion order.
*/
func (scheduler *Scheduler) Drain() error {
	scheduler.mu.Lock()

	if scheduler.closed {
		scheduler.mu.Unlock()
		return io.ErrClosedPipe
	}

	queued := append([]outbound(nil), scheduler.queue...)
	scheduler.queue = nil
	scheduler.mu.Unlock()

	for _, frame := range queued {
		scheduler.Emit(frame.channel, frame.payload)
	}

	return nil
}

/*
Active reports whether requests and scheduling remain valid.
*/
func (scheduler *Scheduler) Active() bool {
	if scheduler == nil {
		return false
	}

	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	return !scheduler.closed
}

func (scheduler *Scheduler) close() {
	scheduler.mu.Lock()
	scheduler.closed = true
	scheduler.channels = nil
	scheduler.queue = nil
	scheduler.mu.Unlock()
}
