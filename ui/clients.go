package ui

import (
	"sync"

	fastwebsocket "github.com/fasthttp/websocket"
	"github.com/theapemachine/symm/types"
)

/*
clientQueue is one dashboard connection's bounded outbox. The single writer
goroutine drains frames; the Workspace Step only ever does a non-blocking send,
so a slow or vanished peer is skipped rather than wedging the bus. The queue is
never closed while it can still be enqueued to: removal from the registry and
the writer's exit happen before stop, and enqueue drops as soon as the entry is
gone.
*/
type clientQueue struct {
	frames  chan *types.UIFrame
	stopped chan struct{}
	once    sync.Once
}

const clientQueueCapacity = 2048

func newClientQueue() *clientQueue {
	return &clientQueue{
		frames:  make(chan *types.UIFrame, clientQueueCapacity),
		stopped: make(chan struct{}),
	}
}

func (queue *clientQueue) enqueue(frame *types.UIFrame) {
	select {
	case queue.frames <- frame:
	default:
	}
}

func (queue *clientQueue) stop() {
	queue.once.Do(func() { close(queue.stopped) })
}

/*
clients is the Hub's lock-free fan-out of live dashboard sockets. It is the
single data-routing surface for UI frames: the Workspace Step writes once into
this registry and every connected peer drains its own bounded queue.
*/
type clients struct {
	mu    sync.RWMutex
	table map[*clientQueue]struct{}
}

func newClients() *clients {
	return &clients{table: make(map[*clientQueue]struct{})}
}

func (registry *clients) add(queue *clientQueue) {
	registry.mu.Lock()
	registry.table[queue] = struct{}{}
	registry.mu.Unlock()
}

func (registry *clients) remove(queue *clientQueue) {
	registry.mu.Lock()
	delete(registry.table, queue)
	registry.mu.Unlock()
}

func (registry *clients) broadcast(frame *types.UIFrame) {
	registry.mu.RLock()
	queues := make([]*clientQueue, 0, len(registry.table))

	for queue := range registry.table {
		queues = append(queues, queue)
	}

	registry.mu.RUnlock()

	for _, queue := range queues {
		queue.enqueue(frame)
	}
}

/*
Step is the Hub's Workspace processing step for ChannelUI. It receives one UI
frame and fans it out to every connected socket. There is no return topic: the
socket is the terminal edge of this pipeline.
*/
func (hub *Hub) Step(value any) any {
	if hub.clients == nil {
		return nil
	}

	frame, ok := value.(*types.UIFrame)

	if !ok || frame == nil {
		return nil
	}

	hub.clients.broadcast(frame)

	return nil
}

/*
writeFrames runs the single writer goroutine for one connection until the
connection or its queue stops.
*/
func (hub *Hub) writeFrames(queue *clientQueue, conn *fastwebsocket.Conn) {
	for {
		select {
		case <-queue.stopped:
			return
		case frame := <-queue.frames:
			err := writeUI(
				conn,
				hub.maxMessageBytes,
				[]*types.UIFrame{frame},
			)

			if expectedDashboardWriteClosure(err) {
				queue.stop()
				return
			}
		}
	}
}
