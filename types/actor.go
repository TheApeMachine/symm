package types

import (
	"context"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

/*
Subscription is a thin typed wrapper over a channel so call sites keep T.
*/
type Subscription struct {
	Channel chan any
}

/*
NewSubscription opens one buffered typed channel.
*/
func NewSubscription() *Subscription {
	buffer := viper.GetViper().GetInt("system.actor.buffer")

	if buffer < 1 {
		buffer = 64
	}

	return &Subscription{
		Channel: make(chan any, buffer),
	}
}

/*
Send delivers one message to the subscription channel.
*/
func (subscription *Subscription) Send(message any) {
	retry := 1

	for retry < 21 {
		select {
		case subscription.Channel <- message:
			return
		default:
			errnie.Warn("subscription buffer full")
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
	name          string
	subscriptions map[string][]*Subscription
	subscribers   map[string][]*Subscription
	handlers      map[string]Handler
}

/*
NewActor constructs an Actor with handlers fixed for its lifetime.
*/
func NewActor(
	ctx context.Context,
	name string,
	handlers map[string]Handler,
) *Actor {
	ctx, cancel := context.WithCancel(ctx)

	if handlers == nil {
		handlers = map[string]Handler{}
	}

	return &Actor{
		ctx:           ctx,
		cancel:        cancel,
		subscriptions: make(map[string][]*Subscription),
		subscribers:   make(map[string][]*Subscription),
		handlers:      handlers,
		name:          name,
	}
}

/*
AddRoot registers a producer-owned ingress subscription under name.
*/
func (actor *Actor) AddRoot(name string, subscription *Subscription) {
	actor.subscriptions[name] = []*Subscription{subscription}
}

/*
Subscribe registers interest in this actor's topic and returns the shared
channel used as the subscriber's inbound subscription.
*/
func (actor *Actor) Subscribe(topic string) *Subscription {
	subscription := NewSubscription()
	actor.subscribers[topic] = append(actor.subscribers[topic], subscription)

	return subscription
}

/*
Initialize attaches this actor to upstream topics under the same Name.
*/
func (actor *Actor) Initialize(topics ...Topic) {
	for _, topic := range topics {
		if actor.subscriptions[topic.Name] == nil {
			actor.subscriptions[topic.Name] = []*Subscription{topic.Actor.Subscribe(topic.Name)}
			continue
		}

		actor.subscriptions[topic.Name] = append(actor.subscriptions[topic.Name], topic.Actor.Subscribe(topic.Name))
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
	for topic, subscriptions := range actor.subscriptions {
		select {
		case <-actor.ctx.Done():
			return
		default:
			for _, subscription := range subscriptions {
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
}

func (actor *Actor) publish(topic string, result any) {
	for _, subscriber := range actor.subscribers[topic] {
		subscriber.Send(result)
	}
}

func (actor *Actor) Close() error {
	actor.cancel()
	return nil
}
