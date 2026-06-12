package internal

import (
	"context"
	"fmt"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

type Bus struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	broadcasts  map[Channel]*qpool.BroadcastGroup
	subscribers map[Channel]*qpool.BroadcastConsumer
}

func NewBus(
	ctx context.Context,
	pool *qpool.Q[any],
	broadcasts []Channel,
	subscriptions []Subscription,
) *Bus {
	ctx, cancel := context.WithCancel(ctx)

	bus := &Bus{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[Channel]*qpool.BroadcastGroup),
		subscribers: make(map[Channel]*qpool.BroadcastConsumer),
	}

	for _, broadcast := range broadcasts {
		bus.broadcasts[broadcast] = pool.CreateBroadcastGroup(
			broadcast.String(), viper.GetDuration("system.queue.ttl"),
		)
	}

	for _, subscription := range subscriptions {
		if bus.broadcasts[subscription.Channel] == nil {
			bus.broadcasts[subscription.Channel] = pool.CreateBroadcastGroup(
				subscription.Channel.String(), viper.GetDuration("system.queue.ttl"),
			)
		}

		subscriberName := subscription.Name

		if subscriberName == "" {
			subscriberName = subscription.Channel.String()
		}

		bus.subscribers[subscription.Channel] = bus.broadcasts[subscription.Channel].Subscribe(
			subscriberName, viper.GetInt("system.queue.buffer"),
		)
	}

	return bus
}

func (bus *Bus) Receive(channel Channel) (*qpool.QValue[any], error) {
	if bus.subscribers[channel] == nil {
		return nil, errnie.Error(fmt.Errorf("channel %s not found", channel))
	}

	return bus.subscribers[channel].Wait(bus.ctx)
}

func (bus *Bus) Send(channel Channel, messageType string, value any) error {
	if bus.broadcasts[channel] == nil {
		return errnie.Error(fmt.Errorf("channel %s not found", channel))
	}

	bus.broadcasts[channel].Send(&qpool.QValue[any]{
		Type:  messageType,
		Value: value,
	})

	return nil
}

func (bus *Bus) Close() error {
	bus.cancel()
	return nil
}
