package morphology

import (
	"context"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Signal is the book-morphology measuring instrument. It composes its market
entity in its constructor and exposes the canonical signal structure:
Constructor, Name, Error, Step, Close.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	book *Book
}

func NewSignal(ctx context.Context, workspace *runtime.Workspace) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		book:   NewBook(workspace),
	}
}

func (signal *Signal) Name() string { return "morphology" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(symbol string, at time.Time) *data.Measurement[float64] {
	return signal.book.Step(symbol, at)
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.book.Close()
}
