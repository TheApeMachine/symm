package types

import (
	"context"
	"reflect"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

var (
	subscriptionSaturationMu   sync.RWMutex
	subscriptionSaturationHook func(map[string]any)
)

/*
Subscription is a thin typed wrapper over a channel so call sites keep T.
*/
type Subscription struct {
	Channel     chan any
	edgeKind    string
	sourceActor string
	sourceTopic string
	ownerActor  string
	ownerTopic  string
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
SetSubscriptionSaturationHook registers one process-local observer for full
subscription buffers so runtime diagnostics can be recorded without importing a
concrete sink into the types package.
*/
func SetSubscriptionSaturationHook(hook func(map[string]any)) {
	subscriptionSaturationMu.Lock()
	defer subscriptionSaturationMu.Unlock()

	subscriptionSaturationHook = hook
}

/*
Send delivers one message to the subscription channel.
*/
func (subscription *Subscription) Send(message any) {
	retry := 1
	reported := false

	for retry < 21 {
		select {
		case subscription.Channel <- message:
			return
		default:
			if !reported {
				reported = true
				subscription.reportSaturation(message, retry)
			}

			retry++
		}
	}
}

func (subscription *Subscription) reportSaturation(message any, retry int) {
	messageType := "<nil>"

	if message != nil {
		messageType = reflect.TypeOf(message).String()
	}

	errnie.Warn(
		"subscription buffer full source=" + subscription.sourceActor +
			" topic=" + subscription.sourceTopic +
			" owner=" + subscription.ownerActor,
	)

	subscriptionSaturationMu.RLock()
	hook := subscriptionSaturationHook
	subscriptionSaturationMu.RUnlock()

	if hook == nil {
		return
	}

	hook(map[string]any{
		"edgeKind":    subscription.edgeKind,
		"sourceActor": subscription.sourceActor,
		"sourceTopic": subscription.sourceTopic,
		"ownerActor":  subscription.ownerActor,
		"ownerTopic":  subscription.ownerTopic,
		"bufferLen":   len(subscription.Channel),
		"bufferCap":   cap(subscription.Channel),
		"retry":       retry,
		"messageType": messageType,
	})
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
	subscription.edgeKind = "root"
	subscription.ownerActor = actor.name
	subscription.ownerTopic = name
	subscription.sourceTopic = name
	actor.subscriptions[name] = []*Subscription{subscription}
}

/*
Subscribe registers interest in this actor's topic and returns the shared
channel used as the subscriber's inbound subscription.
*/
func (actor *Actor) Subscribe(topic string) *Subscription {
	subscription := NewSubscription()
	subscription.edgeKind = "actor"
	subscription.sourceActor = actor.name
	subscription.sourceTopic = topic
	actor.subscribers[topic] = append(actor.subscribers[topic], subscription)

	return subscription
}

/*
Initialize attaches this actor to upstream topics under the same Name.
*/
func (actor *Actor) Initialize(topics ...Topic) {
	for _, topic := range topics {
		subscription := topic.Actor.Subscribe(topic.Name)
		subscription.ownerActor = actor.name
		subscription.ownerTopic = topic.Name

		if actor.subscriptions[topic.Name] == nil {
			actor.subscriptions[topic.Name] = []*Subscription{subscription}
			continue
		}

		actor.subscriptions[topic.Name] = append(actor.subscriptions[topic.Name], subscription)
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

/*
handle blocks on every subscribed inbox until one message arrives or the actor is
cancelled, then routes that message through the topic handler and publishes any
resulting payload to downstream subscribers of the same topic.
*/
func (actor *Actor) handle() {
	cases := []reflect.SelectCase{{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(actor.ctx.Done()),
	}}
	topics := make([]string, 1)

	for topic, subscriptions := range actor.subscriptions {
		for _, subscription := range subscriptions {
			if subscription == nil {
				continue
			}

			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(subscription.Channel),
			})
			topics = append(topics, topic)
		}
	}

	chosen, value, ok := reflect.Select(cases)

	if chosen == 0 || !ok {
		return
	}

	topic := topics[chosen]
	message := value.Interface()
	handler, found := actor.handlers[topic]

	if !found {
		actor.publish(topic, message)
		return
	}

	result := handler.Fn(message)

	if result == nil {
		return
	}

	actor.publish(topic, result)
}

/*
publish fan-outs one topic result to every downstream subscriber registered for
that same topic.
*/
func (actor *Actor) publish(topic string, result any) {
	for _, subscriber := range actor.subscribers[topic] {
		subscriber.Send(result)
	}
}

/*
Close cancels the actor context so the run loop and any blocked handle call stop
cleanly.
*/
func (actor *Actor) Close() error {
	actor.cancel()
	return nil
}
