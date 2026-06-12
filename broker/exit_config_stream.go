package broker

import (
	"fmt"
	"sync/atomic"

	"github.com/smarty/go-disruptor"
	"github.com/theapemachine/symm/config"
)

const (
	exitConfigStreamSize = 64
	exitConfigStreamMask = exitConfigStreamSize - 1
)

/*
ExitConfigStream publishes immutable exit config snapshots through a disruptor.
Desk hot paths consume the latest atomic snapshot without touching Viper,
channels, or mutex-protected globals.
*/
type ExitConfigStream struct {
	ring      [exitConfigStreamSize]config.ExitConfig
	current   atomic.Pointer[config.ExitConfig]
	disruptor disruptor.Disruptor
}

func NewExitConfigStream(
	initial config.ExitConfig,
) (*ExitConfigStream, error) {
	stream := &ExitConfigStream{}
	stream.store(initial)

	instance, err := disruptor.New(
		disruptor.Options.BufferCapacity(exitConfigStreamSize),
		disruptor.Options.WriterCount(1),
		disruptor.Options.NewHandlerGroup(exitConfigHandler{stream: stream}),
	)

	if err != nil {
		return nil, fmt.Errorf("broker: exit config disruptor: %w", err)
	}

	stream.disruptor = instance

	go instance.Listen()

	return stream, nil
}

func (stream *ExitConfigStream) Load() config.ExitConfig {
	if stream == nil {
		return config.ExitConfig{}
	}

	current := stream.current.Load()

	if current == nil {
		return config.ExitConfig{}
	}

	return *current
}

func (stream *ExitConfigStream) Publish(next config.ExitConfig) error {
	if stream == nil || stream.disruptor == nil {
		return fmt.Errorf("broker: exit config stream is not initialized")
	}

	upperSequence := stream.disruptor.Reserve(1)

	if upperSequence < 0 {
		return fmt.Errorf("broker: exit config reserve failed: %d", upperSequence)
	}

	stream.ring[upperSequence&exitConfigStreamMask] = next
	stream.disruptor.Commit(upperSequence, upperSequence)

	return nil
}

func (stream *ExitConfigStream) Close() error {
	if stream == nil || stream.disruptor == nil {
		return nil
	}

	return stream.disruptor.Close()
}

func (stream *ExitConfigStream) store(next config.ExitConfig) {
	snapshot := next

	stream.current.Store(&snapshot)
}

type exitConfigHandler struct {
	stream *ExitConfigStream
}

func (handler exitConfigHandler) Handle(
	lowerSequence int64,
	upperSequence int64,
) {
	for sequence := lowerSequence; sequence <= upperSequence; sequence++ {
		handler.stream.store(
			handler.stream.ring[sequence&exitConfigStreamMask],
		)
	}
}
