package depthflow

import (
	"context"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Signal is the depth-flow measuring instrument. It composes its market entity
in its constructor and exposes the canonical signal structure: Constructor,
Name, Error, Step, Close.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	level3 *Level3
}

/*
NewSignal composes the Level3 (full-book) entity. The book is read through the
shared-object pool at each step; it is requested here at registration.
*/
func NewSignal(ctx context.Context, workspace *runtime.Workspace) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		level3: NewLevel3(workspace),
	}
}

func (signal *Signal) Name() string { return "depthflow" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(symbol string, at time.Time) *data.Measurement[float64] {
	return signal.level3.Step(symbol, at)
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.level3.Close()
}
