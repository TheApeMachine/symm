package types

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
)

/*
Subscription is a thin typed wrapper over a channel so call sites keep T.
*/
type Subscription[T any] struct {
	Channel chan T
}

/*
NewSubscription opens one buffered typed channel using system.actor.buffer.
*/
func NewSubscription[T any]() *Subscription[T] {
	return NewSubscriptionSize[T](0)
}

/*
NewSubscriptionSize opens a typed channel with an explicit buffer. A non-positive
size falls back to system.actor.buffer (then 64) so call sites can request a
strict depth without re-reading config.
*/
func NewSubscriptionSize[T any](buffer int) *Subscription[T] {
	if buffer < 1 {
		buffer = viper.GetViper().GetInt("system.actor.buffer")
	}

	if buffer < 1 {
		buffer = 64
	}

	return &Subscription[T]{
		Channel: make(chan T, buffer),
	}
}

/*
Send delivers one message, blocking when the subscriber is behind so producers
apply real backpressure instead of sleeping and dropping under load.
*/
func (subscription *Subscription[T]) Send(message T) {
	subscription.Channel <- message
}

/*
TrySend enqueues without blocking. It returns false when the subscriber inbox is
full so websocket readers can hand off without stalling the socket.
*/
func (subscription *Subscription[T]) TrySend(message T) bool {
	select {
	case subscription.Channel <- message:
		return true
	default:
		return false
	}
}

/*
Envelope is one ordered inbox item: topic routing plus optional venue sequence
and event time so an Actor consumes a single stream instead of busy-polling
topic maps.
*/
type Envelope struct {
	Topic          string
	SourceSequence uint64
	EventTime      time.Time
	Payload        any
}

/*
Topic names a pipe from an upstream Actor under Name.
*/
type Topic struct {
	Name  string
	Actor *Actor
}

/*
Handler binds a topic name to the function that transforms one Envelope payload.
*/
type Handler struct {
	Topic string
	Fn    func(any) any
}

/*
Actor is embeddable as `*Actor` on entrypoints (Desk, Crypto, Signal, …).
One goroutine blocks on a unified inbox; per-topic subscriptions pump into that
inbox so idle actors sleep and cross-topic order follows arrival order.
*/
type Actor struct {
	ctx           context.Context
	cancel        context.CancelFunc
	subscriptions map[string]*Subscription[any]
	subscribers   map[string][]*Subscription[any]
	handlers      map[string]Handler
	inbox         chan Envelope
	mu            sync.Mutex
	started       atomic.Bool
	frozen        atomic.Bool
	wg            sync.WaitGroup
	seq           atomic.Uint64
	dropped       atomic.Uint64
}

/*
NewActor constructs an Actor with handlers fixed for its lifetime.
*/
func NewActor(
	ctx context.Context,
	handlers map[string]Handler,
) *Actor {
	ctx, cancel := context.WithCancel(ctx)

	if handlers == nil {
		handlers = map[string]Handler{}
	}

	buffer := viper.GetViper().GetInt("system.actor.buffer")

	if buffer < 1 {
		buffer = 64
	}

	return &Actor{
		ctx:           ctx,
		cancel:        cancel,
		subscriptions: make(map[string]*Subscription[any]),
		subscribers:   make(map[string][]*Subscription[any]),
		handlers:      handlers,
		inbox:         make(chan Envelope, buffer),
	}
}

/*
AddRoot registers a producer-owned ingress subscription under name.
*/
func (actor *Actor) AddRoot(name string, subscription *Subscription[any]) {
	actor.mu.Lock()
	defer actor.mu.Unlock()

	if actor.frozen.Load() {
		panic("types.Actor: AddRoot after Start")
	}

	actor.subscriptions[name] = subscription
}

/*
Subscribe registers interest in this actor's topic and returns the shared
channel used as the subscriber's inbound subscription.
*/
func (actor *Actor) Subscribe(topic string) *Subscription[any] {
	return actor.SubscribeSize(topic, 0)
}

/*
SubscribeSize registers interest with an explicit inbound buffer depth.
Subscribers may attach after Start; only the actor's own inbound roots freeze.
*/
func (actor *Actor) SubscribeSize(topic string, buffer int) *Subscription[any] {
	actor.mu.Lock()
	defer actor.mu.Unlock()

	subscription := NewSubscriptionSize[any](buffer)
	actor.subscribers[topic] = append(actor.subscribers[topic], subscription)

	return subscription
}

/*
Initialize attaches this actor to upstream topics under the same Name.
*/
func (actor *Actor) Initialize(topics ...Topic) {
	actor.InitializeSize(0, topics...)
}

/*
InitializeSize attaches upstream topics with an explicit inbound buffer depth so
a slow consumer can apply real backpressure instead of coalescing mutable state.
*/
func (actor *Actor) InitializeSize(buffer int, topics ...Topic) {
	if actor.frozen.Load() {
		panic("types.Actor: InitializeSize after Start")
	}

	actor.mu.Lock()

	for _, topic := range topics {
		actor.subscriptions[topic.Name] = topic.Actor.SubscribeSize(
			topic.Name, buffer,
		)
	}

	actor.mu.Unlock()
	actor.Start()
}

/*
Start freezes topology, pumps each subscription into the unified inbox, and
runs one blocking consumer. Repeated Start is a no-op.
*/
func (actor *Actor) Start() {
	if !actor.started.CompareAndSwap(false, true) {
		return
	}

	actor.frozen.Store(true)
	actor.mu.Lock()
	topics := make([]string, 0, len(actor.subscriptions))

	for topic := range actor.subscriptions {
		topics = append(topics, topic)
	}

	actor.mu.Unlock()

	for _, topic := range topics {
		actor.pump(topic)
	}

	actor.wg.Go(func() {
		actor.loop()
	})
}

/*
Run is retained for call sites; it starts the actor once.
*/
func (actor *Actor) Run() {
	actor.Start()
}

/*
Close cancels the actor and waits until the inbox loop and pumps exit.
*/
func (actor *Actor) Close() error {
	actor.cancel()
	actor.wg.Wait()

	return nil
}

func (actor *Actor) pump(topic string) {
	actor.mu.Lock()
	subscription := actor.subscriptions[topic]
	actor.mu.Unlock()

	if subscription == nil {
		return
	}


	actor.wg.Go(func() {
		for {
			select {
			case <-actor.ctx.Done():
				return
			case message, ok := <-subscription.Channel:
				if !ok {
					return
				}

				envelope := Envelope{
					Topic:          topic,
					SourceSequence: actor.seq.Add(1),
					EventTime:      time.Now().UTC(),
					Payload:        message,
				}

				select {
				case <-actor.ctx.Done():
					return
				case actor.inbox <- envelope:
				}
			}
		}
	})
}

func (actor *Actor) loop() {
	for {
		select {
		case <-actor.ctx.Done():
			return
		case envelope, ok := <-actor.inbox:
			if !ok {
				return
			}

			actor.dispatch(envelope)
		}
	}
}

func (actor *Actor) dispatch(envelope Envelope) {
	handler, ok := actor.handlers[envelope.Topic]

	if !ok {
		// Root fan-out (Live/Paper): never block ingress on a slow subscriber.
		// Cascade actors always register handlers and keep blocking publish so
		// cut identity stays serial where InitializeSize(1) is intentional.
		actor.tryPublish(envelope.Topic, envelope.Payload)
		return
	}

	result := handler.Fn(envelope.Payload)

	if result == nil {
		return
	}

	actor.publish(envelope.Topic, result)
}

/*
Dropped reports how many tryPublish subscriber enqueues were rejected.
*/
func (actor *Actor) Dropped() uint64 {
	return actor.dropped.Load()
}

func (actor *Actor) publish(topic string, result any) {
	actor.mu.Lock()
	subscribers := append([]*Subscription[any](nil), actor.subscribers[topic]...)
	actor.mu.Unlock()

	for _, subscriber := range subscribers {
		select {
		case <-actor.ctx.Done():
			return
		case subscriber.Channel <- result:
		}
	}
}

func (actor *Actor) tryPublish(topic string, result any) {
	actor.mu.Lock()
	subscribers := append([]*Subscription[any](nil), actor.subscribers[topic]...)
	actor.mu.Unlock()

	for _, subscriber := range subscribers {
		select {
		case <-actor.ctx.Done():
			return
		case subscriber.Channel <- result:
		default:
			actor.dropped.Add(1)
		}
	}
}
