package types

import "github.com/spf13/viper"

/*
Subscription is one typed buffered channel used for direct fan-out without the
old Actor layer.
*/
type Subscription[T any] struct {
	Channel chan T
}

/*
NewSubscription allocates one buffered typed subscription using the configured
system buffer or the production default when unset.
*/
func NewSubscription[T any]() *Subscription[T] {
	viper.SetDefault("system.actor.buffer", 1024)
	buffer := viper.GetInt("system.actor.buffer")

	return &Subscription[T]{
		Channel: make(chan T, buffer),
	}
}

/*
NewLatestSubscription allocates one slot for a shared state pointer.
Producers mutate the same state between notifications, so queueing older
notifications cannot preserve older state; it only repeats work on the newest
state. SendLatest replaces that one pending notification when consumers lag.
*/
func NewLatestSubscription[T any]() *Subscription[T] {
	viper.SetDefault("system.actor.buffer", 1024)
	buffer := viper.GetInt("system.actor.buffer")

	return &Subscription[T]{
		Channel: make(chan T, buffer),
	}
}

/*
Send publishes one message onto the subscription channel.
*/
func (subscription *Subscription[T]) Send(message T) {
	subscription.Channel <- message
}

/*
SendLatest publishes the newest message without letting a stale buffered value
block hot market-data paths. While the buffer is full, queued values are dropped
until the current message is accepted.
*/
func (subscription *Subscription[T]) SendLatest(message T) {
	subscription.Channel <- message
}
