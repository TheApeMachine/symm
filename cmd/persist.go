package cmd

import (
	"context"
	"fmt"
	"reflect"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	nmruntime "github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

type tickEnvelopeStore interface {
	WriteAtPrefixWithEnvelope(key string, envelope *nmruntime.Envelope) (int, error)
	QueueDepth() int
}

/*
TickPersist writes the finished envelope after the model ring. Step only parks
the latest envelope (latest-wins); one worker marshals and hands off to the
store's upload queue. A deep backlog of full envelopes is never retained.
*/
type TickPersist struct {
	*nmruntime.System
	store tickEnvelopeStore
}

func NewTickPersist(ctx context.Context, client *store.Client) *TickPersist {
	persist := &TickPersist{}
	if client != nil {
		persist.store = client
	}
	persist.System = nmruntime.NewSystem(ctx, "tick-persist", persist)
	return persist
}

func tickStoreReady(store tickEnvelopeStore) bool {
	if store == nil {
		return false
	}

	val := reflect.ValueOf(store)

	return val.Kind() != reflect.Ptr || !val.IsNil()
}

func (persist *TickPersist) Step(envelope *nmruntime.Envelope) *nmruntime.Envelope {
	if envelope == nil {
		return nil
	}

	if !tickStoreReady(persist.store) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[tick-persist] store client required",
			nil,
		))
	} else if _, err := persist.store.WriteAtPrefixWithEnvelope(fmt.Sprintf(
		"workspace/tick/aggregate/%019d/%019d", data.Epoch.Load(), envelope.N,
	), envelope); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO, "failed to write envelope", err,
		))
	}

	focus := types.Focus()

	for _, ctx := range types.SignalSourceStrings {
		if focus == "" {
			continue
		}

		if payload := envelope.Publish(ctx, focus); len(payload) > 0 {
			ui.UI.Write(payload)
		}
	}

	for _, ctx := range types.LogicSourceStrings {
		if payload := envelope.Publish(ctx, ""); len(payload) > 0 {
			ui.UI.Write(payload)
		}
	}

	if types.WantsGrid() {
		for _, ctx := range []string{"regions", "paths", "decisions", "trade"} {
			if payload := envelope.Publish(ctx, ""); len(payload) > 0 {
				ui.UI.Write(payload)
			}
		}
	}

	ui.UI.WriteJSON(map[string]any{
		"tick": map[string]any{
			"epoch": data.Epoch.Load(),
			"n":     envelope.N,
		},
	})

	return envelope
}
