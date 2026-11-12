package cmd

import (
	"context"
	"fmt"
	"reflect"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	nmruntime "github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
)

type tickEnvelopeStore interface {
	WriteAtPrefixWithEnvelope(key string, envelope *nmruntime.Envelope) (int, error)
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
		return envelope
	}

	if _, err := persist.store.WriteAtPrefixWithEnvelope(fmt.Sprintf(
		"workspace/tick/aggregate/%019d/%019d", data.Epoch.Load(), envelope.N,
	), envelope); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO, "failed to write envelope", err,
		))
	}

	return envelope
}
