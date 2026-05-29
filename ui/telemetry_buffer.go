package ui

import (
	"context"
	"sync/atomic"

	"github.com/theapemachine/symm/runstats"
)

/*
TelemetryBuffer is a fixed-size lossy queue between engine broadcasts and
websocket fanout.
*/
type TelemetryBuffer struct {
	queue   chan any
	dropped atomic.Int64
}

func NewTelemetryBuffer(capacity int) *TelemetryBuffer {
	if capacity <= 0 {
		capacity = 1
	}

	return &TelemetryBuffer{
		queue: make(chan any, capacity),
	}
}

func (buffer *TelemetryBuffer) Push(payload any) {
	if buffer == nil || payload == nil {
		return
	}

	select {
	case buffer.queue <- payload:
		return
	default:
	}

	select {
	case <-buffer.queue:
		buffer.dropped.Add(1)
		runstats.UIFramesDropped(1)
	default:
	}

	select {
	case buffer.queue <- payload:
	default:
		buffer.dropped.Add(1)
		runstats.UIFramesDropped(1)
	}
}

func (buffer *TelemetryBuffer) Run(
	ctx context.Context,
	consume func(any),
) {
	if buffer == nil || consume == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-buffer.queue:
			consume(payload)
		}
	}
}

func (buffer *TelemetryBuffer) Dropped() int64 {
	if buffer == nil {
		return 0
	}

	return buffer.dropped.Load()
}

func (buffer *TelemetryBuffer) Depth() int {
	if buffer == nil {
		return 0
	}

	return len(buffer.queue)
}

func (buffer *TelemetryBuffer) Capacity() int {
	if buffer == nil {
		return 0
	}

	return cap(buffer.queue)
}
