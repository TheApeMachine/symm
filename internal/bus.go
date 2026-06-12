package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/observability"
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
		err := fmt.Errorf("channel %s not found", channel)
		observability.Shared().RecordBusDrop(
			channel.String(),
			"",
			err.Error(),
			time.Now().UTC(),
		)

		return nil, errnie.Error(err)
	}

	message, err := bus.subscribers[channel].Wait(bus.ctx)

	if err != nil {
		observability.Shared().RecordBusDrop(
			channel.String(),
			"",
			err.Error(),
			time.Now().UTC(),
		)

		return message, err
	}

	if message != nil {
		observability.Shared().RecordBusReceive(
			channel.String(),
			message.Type,
			time.Now().UTC(),
		)
	}

	return message, nil
}

func (bus *Bus) Send(channel Channel, messageType string, value any) error {
	if bus.broadcasts[channel] == nil {
		err := fmt.Errorf("channel %s not found", channel)
		observability.Shared().RecordBusDrop(
			channel.String(),
			messageType,
			err.Error(),
			time.Now().UTC(),
		)

		return errnie.Error(err)
	}

	bus.broadcasts[channel].Send(&qpool.QValue[any]{
		Type:  messageType,
		Value: value,
	})

	observability.Shared().RecordBusSend(
		channel.String(),
		messageType,
		time.Now().UTC(),
	)

	return nil
}

func (bus *Bus) Close() error {
	bus.cancel()
	return nil
}
