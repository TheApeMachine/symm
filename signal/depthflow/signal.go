package depthflow

import (
	"context"

	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
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
	status *runtime.Status

	level3 *Level3
}

/*
NewSignal composes the Level3 (depth-flow) entity.
*/
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		status: runtime.NewStatus(),
		level3: NewLevel3(),
	}
}

func (signal *Signal) Name() string { return "depthflow" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(envelope *types.Envelope) *types.Envelope {
	envelope.DepthFlow = signal.level3.Step(envelope.Level3Data)

	return envelope
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.level3.Close()
}
