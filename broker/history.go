package broker

import (
	"context"
	"iter"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
)

/*
History persists open lots so a restart can exit what the previous process
entered, and drops them when the paper wallet resets.
*/
type History struct {
	*runtime.System
	store *store.Client
}

func NewHistory(ctx context.Context, store *store.Client) *History {
	history := &History{
		System: runtime.NewSystem(ctx, "history"),
		store:  store,
	}

	return history
}

func (history *History) Restore(prefix string) iter.Seq[*data.Value] {
	return history.store.Stream(history.Context(), prefix)
}

/*
Store a value in the history.
*/
func (history *History) Store(value *data.Value) error {
	_, err := history.store.WriteAtPrefix(
		value.Prefix, value.Bytes(),
	)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[history.Store] failed to store value",
			err,
		))
	}

	return nil
}

/*
Move the prefix.
*/
func (history *History) Move(value *data.Value, to string) error {
	from := value.Prefix
	value.Prefix = to
	return history.store.MovePrefix(from, to)
}
