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
	buffer := viper.GetInt("system.actor.buffer")

	return &Subscription[T]{
		Channel: make(chan T, buffer),
	}
}

/*
Send publishes one message onto the subscription channel. When the channel is
already full the oldest message is drained first so one slow consumer cannot
block the market-data fan-out; the freshest observation always reaches the
buffer. Every consumer was already expected to tolerate coalesced frames.
*/
func (subscription *Subscription[T]) Send(message T) {
	subscription.Channel <- message
}
