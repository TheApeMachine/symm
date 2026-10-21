package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"iter"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
)

/*
openPositionPrefix is the store home of open lots: position/open/<symbol>,
one object per symbol, each one serialized data.Value whose layer is the
position's JSON.
*/
const openPositionPrefix = "position/open/"

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

/*
Restore yields every lot stored under position/open/. A history without a
store yields nothing; a corrupt object yields an error, never a silent skip.
*/
func (history *History) Restore() iter.Seq2[*Position, error] {
	return func(yield func(*Position, error) bool) {
		if history == nil || history.store == nil {
			return
		}

		for key := range history.store.ListKeys(openPositionPrefix) {
			position, err := history.load(key)

			if err != nil {
				if !yield(nil, err) {
					return
				}

				continue
			}

			if position == nil {
				if !yield(nil, errnie.Err(errnie.IO, "[history] object at "+key+" holds no layer", nil)) {
					return
				}

				continue
			}

			if !yield(position, nil) {
				return
			}
		}
	}
}

/*
Record stores one open lot at position/open/<symbol>, replacing its
previous state.
*/
func (history *History) Record(position *Position) error {
	if history == nil || history.store == nil || position == nil || position.Symbol == "" {
		return nil
	}

	raw, err := sonic.Marshal(position)

	if err != nil {
		return errnie.Error(err)
	}

	value := data.NewValue("position", "open", position.Symbol)
	value.Append(raw, "position")

	if _, err := io.Copy(history.store, value); err != nil {
		return errnie.Error(err)
	}

	return nil
}

/*
Forget removes one symbol's stored lot, closing its round trip's persistence.
*/
func (history *History) Forget(symbol string) error {
	if history == nil || history.store == nil || symbol == "" {
		return nil
	}

	for key := range history.store.ListKeys(openPositionPrefix + symbol) {
		if err := history.store.DeletePrefix(key); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

/*
Drop removes every stored lot so a wallet reset does not restore positions
onto an empty account.
*/
func (history *History) Drop() error {
	if history == nil || history.store == nil {
		return nil
	}

	for key := range history.store.ListKeys(openPositionPrefix) {
		if err := history.store.DeletePrefix(key); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

/*
load decodes the one position object stored at key. A nil position with a
nil error means the object held no layer.
*/
func (history *History) load(key string) (*Position, error) {
	body := &bytes.Buffer{}

	if err := history.store.Stream(history.Context(), key, body); err != nil {
		return nil, errnie.Error(err)
	}

	value := data.From(body.Bytes())

	for _, index := range value.Index {
		for _, raw := range data.Layer[json.RawMessage](value, leaf(index)) {
			position := &Position{}

			if err := sonic.Unmarshal(raw, position); err != nil {
				return nil, errnie.Err(
					errnie.UnprocessableContent,
					"[history] position at "+key+" is not position JSON",
					err,
				)
			}

			return position, nil
		}
	}

	return nil, nil
}

/*
leaf is the final path segment of a layer index.
*/
func leaf(index string) string {
	if idx := strings.LastIndex(index, "/"); idx >= 0 {
		return index[idx+1:]
	}

	return index
}
