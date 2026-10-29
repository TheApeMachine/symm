package broker

import (
	"context"
	"iter"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
)

/*
History persists open lots so a restart can exit what the previous process
entered, and drops them when the paper wallet resets.
*/
type History struct {
	*runtime.System
	store *store.Client
}

func NewHistory(ctx context.Context) *History {
	history := &History{
		System: runtime.NewSystem(ctx, "history"),
		store:  store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
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
ReplaceOpen stores this snapshot and deletes every other open object for the
same lot. A restart used to leave the previous epoch's object in place, so
one symbol showed two open records.
*/
func (history *History) ReplaceOpen(value *data.Value) error {
	if history == nil || history.store == nil || value == nil {
		return nil
	}

	// Run cleanup in background so trading tick is never blocked by S3 latency
	go func(prefix, keep string) {
		if err := history.store.DeleteOtherKeys(prefix, keep); err != nil {
			errnie.Error(err)
		}
	}(openDir(value.Prefix), value.Prefix)

	return history.Store(value)
}

func (history *History) Drop(prefix string) error {
	if history == nil || history.store == nil || prefix == "" {
		return nil
	}

	return history.store.DeleteOtherKeys(prefix, "")
}

func openDir(prefix string) string {
	parts := strings.Split(prefix, "/")
	if len(parts) < 3 {
		return prefix
	}

	return strings.Join(parts[:len(parts)-2], "/") + "/"
}

/*
Move retires the open records of a lot and stores the closed one: the value is written at its
own prefix and every record under the source prefix is deleted. The value carries the outcome
of the lot, so the closed record is written from the desk's memory and not copied from the
open side, which still holds the state the lot had when it entered.
*/
func (history *History) Move(value *data.Value, from string) error {
	if _, err := history.store.WriteAtPrefix(
		value.Prefix, value.Bytes(),
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[history.Move] failed to store value",
			err,
		))
	}

	if err := history.store.DeletePrefix(from); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[history.Move] failed to retire open value",
			err,
		))
	}

	return nil
}
