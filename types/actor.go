package types

import (
	"context"
	"fmt"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/utils"
)

/*
Subscription is a thin typed wrapper over a channel so call sites keep T.
*/
type Subscription[T any] struct {
	Channel chan T
}

/*
NewSubscription opens one buffered typed channel.
*/
func NewSubscription[T any]() *Subscription[T] {
	buffer := viper.GetViper().GetInt("system.actor.buffer")

	if buffer < 1 {
		buffer = 64
	}

	return &Subscription[T]{
		Channel: make(chan T, buffer),
	}
}

/*
Send delivers one message to the subscription channel.
*/
func (subscription *Subscription[T]) Send(message T) {
	retry := 1

	for retry < 21 {
		select {
		case subscription.Channel <- message:
			return
		default:
			retry = utils.Backoff(retry)
			errnie.Warn(fmt.Sprintf("subscription buffer full, retrying: %d", retry))
		}
	}
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
	subscription := NewSubscription[any]()
	actor.subscribers[topic] = append(actor.subscribers[topic], subscription)

	return subscription
}

/*
Initialize attaches this actor to upstream topics under the same Name.
*/
func (actor *Actor) Initialize(topics ...Topic) {
	for _, topic := range topics {
		actor.subscriptions[topic.Name] = topic.Actor.Subscribe(topic.Name)
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
				actor.handle()
			}
		}
	}()
}

func (actor *Actor) handle() {
	for topic, subscription := range actor.subscriptions {
		select {
		case <-actor.ctx.Done():
			return
		default:
			select {
			case <-actor.ctx.Done():
				return
			case message := <-subscription.Channel:
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
	}
}

func (actor *Actor) publish(topic string, result any) {
	for _, subscriber := range actor.subscribers[topic] {
		subscriber.Send(result)
	}
}
