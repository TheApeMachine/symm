package types

import (
	"context"
	"runtime"

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
Topic names a pipe from an upstream Actor under Name.
*/
type Topic struct {
	Name  string
	Actor *Actor
}

type Handler struct {
	Topic string
	Fn    func(any) any
}

/*
Actor is embeddable as `*Actor` on entrypoints (Desk, Crypto, Signal, …).
Run pops each subscription, runs the handler, and Sends the result to
subscribers of that same topic.
*/
type Actor struct {
	ctx           context.Context
	cancel        context.CancelFunc
	subscriptions map[string]*Subscription[any]
	subscribers   map[string][]*Subscription[any]
	handlers      map[string]Handler
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

	return &Actor{
		ctx:           ctx,
		cancel:        cancel,
		subscriptions: make(map[string]*Subscription[any]),
		subscribers:   make(map[string][]*Subscription[any]),
		handlers:      handlers,
	}
}

/*
AddRoot registers a producer-owned ingress subscription under name.
*/
func (actor *Actor) AddRoot(name string, subscription *Subscription[any]) {
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
*/
func (actor *Actor) SubscribeSize(topic string, buffer int) *Subscription[any] {
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
	for _, topic := range topics {
		actor.subscriptions[topic.Name] = topic.Actor.SubscribeSize(
			topic.Name, buffer,
		)
	}

	actor.Run()
}

/*
Run starts the long-running loop that pops subscriptions into handlers.
*/
func (actor *Actor) Run() {
	go func() {
		for {
			select {
			case <-actor.ctx.Done():
				return
			default:
				if !actor.handle() {
					runtime.Gosched()
				}
			}
		}
	}()
}

/*
handle drains ready subscription messages round-robin until every topic is
empty for one full pass. It returns true when any work ran so the idle path
only yields after a fully empty poll.
*/
func (actor *Actor) handle() bool {
	worked := false

	for {
		progress := false

		for topic, subscription := range actor.subscriptions {
			select {
			case <-actor.ctx.Done():
				return worked
			default:
			}

			select {
			case <-actor.ctx.Done():
				return worked
			case message := <-subscription.Channel:
				progress = true
				worked = true
				handler, ok := actor.handlers[topic]

				if !ok {
					actor.publish(topic, message)
					continue
				}

				result := handler.Fn(message)

				if result == nil {
					continue
				}

				actor.publish(topic, result)
			default:
			}
		}

		if !progress {
			return worked
		}
	}
}

func (actor *Actor) publish(topic string, result any) {
	for _, subscriber := range actor.subscribers[topic] {
		subscriber.Send(result)
	}
}
