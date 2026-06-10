package record

import (
	"context"
	"errors"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
)

/*
Capture subscribes to the raw bus and appends every frame to the capture file.
*/
type Capture struct {
	ctx    context.Context
	cancel context.CancelFunc
	bus    *internal.Bus
	writer *Writer
}

func NewCapture(
	ctx context.Context,
	pool *qpool.Q[any],
	writer *Writer,
) *Capture {
	if writer == nil {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)

	return &Capture{
		ctx:    ctx,
		cancel: cancel,
		bus: internal.NewBus(
			ctx,
			pool,
			nil,
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelRaw, "capture"),
			},
		),
		writer: writer,
	}
}

func (capture *Capture) Tick() error {
	for {
		if errnie.Error(capture.ctx.Err()) != nil {
			return capture.ctx.Err()
		}

		message, err := capture.bus.Receive(internal.ChannelRaw)

		if errnie.Error(err) != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}

			continue
		}

		if message == nil {
			continue
		}

		errnie.Error(capture.writer.Write(message.Type, message.Value))
	}
}

func (capture *Capture) Close() error {
	if capture == nil {
		return nil
	}

	capture.cancel()

	if capture.writer == nil {
		return nil
	}

	return capture.writer.Close()
}
