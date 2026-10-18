package broker

import (
	"context"
	"io"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/data/transport"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
)

type History struct {
	*runtime.System
	*transport.Stream
	store *store.Client
}

func NewHistory(ctx context.Context, store *store.Client) *History {
	history := &History{
		System: runtime.NewSystem(ctx, "history"),
		store:  store,
	}

	history.Stream = transport.NewStream(ctx, func(v *data.Value) *data.Value {
		if v == nil {
			return nil
		}

		v.Prefix = "position/open/"
		flipflop := transport.NewFlipFlop(v, history.store)
		_, err := io.Copy(flipflop, v)

		if err != nil {
			v.Err = errnie.Error(errnie.Err(
				errnie.IO,
				"[history] failed to copy value to store",
				err,
			))

			return nil
		}

		return v
	})

	return history
}
