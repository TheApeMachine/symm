package broker

import (
	"context"
	"fmt"

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
	price     *Price
	positions *Cache
	virtuals  *Cache
	store     *store.Client
	transport Transport
	onSettled func(*Position)
}

func NewHistory(
	ctx context.Context,
	price *Price,
	positions *Cache,
	virtuals *Cache,
	transport Transport,
	onSettled func(*Position),
) *History {
	history := &History{
		System:    runtime.NewSystem(ctx, "history"),
		price:     price,
		positions: positions,
		virtuals:  virtuals,
		transport: transport,
		store:     store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
	}

	for key, cache := range map[string]*Cache{
		"position": positions,
		"virtual":  virtuals,
	} {
		for _, state := range []string{"open", "closed"} {
			for value := range history.store.Stream(
				history.Context(), fmt.Sprintf("%s/%s", key, state),
			) {
				position := data.Get[Position](value, "position")
				position.price = price
				position.transport = transport
				position.stopLoss = NewStopLoss()
				position.onSettled = onSettled

				cache.Add(position.Symbol, &position, state == "open")
			}
		}
	}

	return history
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

func (history *History) Drop(prefix string) error {
	if history == nil || history.store == nil || prefix == "" {
		return nil
	}

	return history.store.DeleteOtherKeys(prefix, "")
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
