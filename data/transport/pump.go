package transport

import (
	"context"
	"io"
)

/*
Pump wraps a pipeline with a cancellable ReadWriteCloser boundary.
*/
type Pump struct {
	ctx      context.Context
	cancel   context.CancelFunc
	pipeline io.ReadWriteCloser
}

/*
NewPump creates a new Pump instance that wraps the provided pipeline.
*/
func NewPump(ctx context.Context, pipeline io.ReadWriteCloser) *Pump {
	ctx, cancel := context.WithCancel(ctx)

	return &Pump{
		ctx:      ctx,
		cancel:   cancel,
		pipeline: pipeline,
	}
}

/*
Read implements the io.Reader interface.
It delegates the read operation to the underlying pipeline.
*/
func (pump *Pump) Read(p []byte) (n int, err error) {
	return pump.pipeline.Read(p)
}

/*
Write implements the io.Writer interface.
It delegates the write operation to the underlying pipeline.
*/
func (pump *Pump) Write(p []byte) (n int, err error) {
	return pump.pipeline.Write(p)
}

/*
Close implements the io.Closer interface.
*/
func (pump *Pump) Close() error {
	if pump.cancel != nil {
		pump.cancel()
	}

	if pump.pipeline != nil {
		return pump.pipeline.Close()
	}

	return nil
}