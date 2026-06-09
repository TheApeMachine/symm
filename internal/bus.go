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
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.BroadcastConsumer
}

func NewBus(
	ctx context.Context,
	pool *qpool.Q[any],
	broadcasts []string,
	subscribers []string,
) *Bus {
	ctx, cancel := context.WithCancel(ctx)

	bus := &Bus{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.BroadcastConsumer),
	}

	for _, broadcast := range broadcasts {
		bus.broadcasts[broadcast] = pool.CreateBroadcastGroup(
			broadcast, viper.GetDuration("system.queue.ttl"),
		)
	}

	for _, subscriber := range subscribers {
		if bus.broadcasts[subscriber] == nil {
			bus.broadcasts[subscriber] = pool.CreateBroadcastGroup(
				subscriber, viper.GetDuration("system.queue.ttl"),
			)
		}

		bus.subscribers[subscriber] = bus.broadcasts[subscriber].Subscribe(
			subscriber, viper.GetInt("system.queue.buffer"),
		)
	}

	return bus
}

func (bus *Bus) Receive(channel string) (*qpool.QValue[any], error) {
	if bus.subscribers[channel] == nil {
		return nil, errnie.Error(fmt.Errorf("channel %s not found", channel))
	}

	return bus.subscribers[channel].Wait(bus.ctx)
}

func (bus *Bus) Poll(channel string) (*qpool.QValue[any], error) {
	if bus.subscribers[channel] == nil {
		return nil, errnie.Error(fmt.Errorf("channel %s not found", channel))
	}

	return bus.subscribers[channel].Poll(), nil
}

func (bus *Bus) Send(channel, t string, value any) error {
	if bus.broadcasts[channel] == nil {
		return errnie.Error(fmt.Errorf("channel %s not found", channel))
	}

	bus.broadcasts[channel].Send(&qpool.QValue[any]{
		Type:  t,
		Value: value,
	})

	return nil
}

func (bus *Bus) Close() error {
	bus.cancel()
	return nil
}
