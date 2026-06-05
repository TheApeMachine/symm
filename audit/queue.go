package audit

import (
	"fmt"
	"sync"
)

type writerQueue struct {
	mu     sync.Mutex
	ready  *sync.Cond
	frames []map[string]any
	head   int
	closed bool
}

func newWriterQueue() *writerQueue {
	queue := &writerQueue{}
	queue.ready = sync.NewCond(&queue.mu)

	return queue
}

func (queue *writerQueue) Push(frame map[string]any) error {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	if queue.closed {
		return fmt.Errorf("audit: writer closed")
	}

	queue.frames = append(queue.frames, frame)
	queue.ready.Signal()

	return nil
}

func (queue *writerQueue) Pop() (map[string]any, bool) {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	for queue.head == len(queue.frames) && !queue.closed {
		queue.ready.Wait()
	}

	if queue.head == len(queue.frames) {
		return nil, false
	}

	frame := queue.frames[queue.head]
	queue.frames[queue.head] = nil
	queue.head++
	queue.compactLocked()

	return frame, true
}

func (queue *writerQueue) Close() {
	queue.mu.Lock()
	queue.closed = true
	queue.ready.Broadcast()
	queue.mu.Unlock()
}

func (queue *writerQueue) compactLocked() {
	if queue.head == 0 || queue.head < len(queue.frames)/2 {
		return
	}

	copy(queue.frames, queue.frames[queue.head:])
	queue.frames = queue.frames[:len(queue.frames)-queue.head]
	queue.head = 0
}
