package internal

import (
	"context"
	"fmt"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/audit"
)

type Bus struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	broadcasts  map[Channel]*qpool.BroadcastGroup
	subscribers map[Channel]*qpool.BroadcastConsumer
	recorder    *audit.Recorder
	audit       bool
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
		recorder:    nil,
		audit:       viper.GetBool("system.audit.enabled"),
	}

	if bus.audit {
		bus.recorder, bus.err = audit.NewRecorder(viper.GetString("system.audit.file"))

		if errnie.Error(bus.err) != nil {
			return nil
		}
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

func (bus *Bus) Poll(channel Channel) (*qpool.QValue[any], error) {
	if bus.subscribers[channel] == nil {
		return nil, errnie.Error(fmt.Errorf("channel %s not found", channel))
	}

	return bus.subscribers[channel].Poll(), nil
}

func (bus *Bus) Send(channel Channel, messageType string, value any) error {
	if bus.broadcasts[channel] == nil {
		return errnie.Error(fmt.Errorf("channel %s not found", channel))
	}

	bus.broadcasts[channel].Send(&qpool.QValue[any]{
		Type:  messageType,
		Value: value,
	})

	if bus.recorder != nil {
		bus.recorder.Write(map[string]any{
			"channel": channel,
			"type":    messageType,
			"value":   value,
		})
	}

	return nil
}

/*
Audit appends one diagnostic row when system audit recording is enabled.
It does not broadcast on the bus.
*/
func (bus *Bus) Audit(eventType string, value any) error {
	return audit.Record(bus.recorder, eventType, value)
}

func (bus *Bus) Close() error {
	bus.cancel()
	return nil
}
