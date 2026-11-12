package cmd

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/data"
	nmruntime "github.com/theapemachine/nomagique/runtime"
)

type recordingTickStore struct {
	mu    sync.Mutex
	keys  []string
	ticks []*nmruntime.Envelope
	err   error
}

func (store *recordingTickStore) WriteAtPrefixWithEnvelope(
	key string,
	envelope *nmruntime.Envelope,
) (int, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	if store.err != nil {
		return 0, store.err
	}

	store.keys = append(store.keys, key)
	store.ticks = append(store.ticks, envelope)

	return 0, nil
}

func TestTickPersistStep(t *testing.T) {
	Convey("Step ignores a nil envelope", t, func() {
		persist := NewTickPersist(context.Background(), nil)
		So(persist.Step(nil), ShouldBeNil)
	})

	Convey("Step passes through a live envelope when store is nil", t, func() {
		envelope := nmruntime.NewEnvelope(nil)
		persist := NewTickPersist(context.Background(), nil)

		out := persist.Step(envelope)
		So(out, ShouldEqual, envelope)
	})

	Convey("Step enqueues workspace aggregate keys with epoch and tick", t, func() {
		backing := &recordingTickStore{}
		epoch := data.Epoch.Load()
		envelope := nmruntime.NewEnvelope(nil)
		envelope.N = 42

		persist := &TickPersist{
			System: nmruntime.NewSystem(context.Background(), "tick-persist"),
			store:  backing,
		}

		wantKey := fmt.Sprintf(
			"workspace/tick/aggregate/%019d/%019d", epoch, envelope.N,
		)

		out := persist.Step(envelope)
		So(out, ShouldEqual, envelope)
		So(len(backing.keys), ShouldEqual, 1)
		So(backing.keys[0], ShouldEqual, wantKey)
		So(backing.ticks[0].N, ShouldEqual, 42)
	})

}

func TestTickPersistKeyFormat(t *testing.T) {
	Convey("aggregate prefix is stable under tick growth", t, func() {
		backing := &recordingTickStore{}
		persist := &TickPersist{
			System: nmruntime.NewSystem(context.Background(), "tick-persist"),
			store:  backing,
		}

		for n := range uint64(3) {
			envelope := nmruntime.NewEnvelope(nil)
			envelope.N = n + 1
			persist.Step(envelope)
		}

		So(len(backing.keys), ShouldEqual, 3)

		for i, key := range backing.keys {
			So(strings.HasPrefix(key, "workspace/tick/aggregate/"), ShouldBeTrue)
			So(strings.HasSuffix(key, fmt.Sprintf("%019d", uint64(i+1))), ShouldBeTrue)
		}
	})
}
