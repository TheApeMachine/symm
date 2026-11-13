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

func (store *recordingTickStore) QueueDepth() int {
	store.mu.Lock()
	defer store.mu.Unlock()

	return len(store.keys)
}

func TestNewTickPersist(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewTickPersist is called without a store client", func() {
			persist := NewTickPersist(context.Background(), nil)

			Convey("A tick-persist system is returned", func() {
				So(persist, ShouldNotBeNil)
				So(persist.System, ShouldNotBeNil)
			})
		})
	})
}

func TestStep(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When Step receives a nil envelope", func() {
			persist := NewTickPersist(context.Background(), nil)
			out := persist.Step(nil)

			Convey("It returns nil", func() {
				So(out, ShouldBeNil)
			})
		})

		Convey("When the store is nil", func() {
			envelope := nmruntime.NewEnvelope(nil)
			persist := NewTickPersist(context.Background(), nil)

			out := persist.Step(envelope)

			Convey("The envelope passes through unchanged", func() {
				So(out, ShouldEqual, envelope)
			})
		})

		Convey("When a backing store is wired", func() {
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

			Convey("One aggregate key is enqueued with the same envelope", func() {
				So(out, ShouldEqual, envelope)
				So(len(backing.keys), ShouldEqual, 1)
				So(backing.keys[0], ShouldEqual, wantKey)
				So(backing.ticks[0].N, ShouldEqual, 42)
			})
		})

		Convey("When ticks advance under one store", func() {
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

			Convey("Each key keeps the aggregate prefix and zero-padded tick", func() {
				So(len(backing.keys), ShouldEqual, 3)

				for i, key := range backing.keys {
					So(strings.HasPrefix(key, "workspace/tick/aggregate/"), ShouldBeTrue)
					So(strings.HasSuffix(key, fmt.Sprintf("%019d", uint64(i+1))), ShouldBeTrue)
				}
			})
		})
	})
}

func BenchmarkNewTickPersist(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		_ = NewTickPersist(ctx, nil)
	}
}

func BenchmarkStep(b *testing.B) {
	ctx := context.Background()
	backing := &recordingTickStore{}
	persist := &TickPersist{
		System: nmruntime.NewSystem(ctx, "tick-persist"),
		store:  backing,
	}
	envelope := nmruntime.NewEnvelope(nil)
	envelope.N = 42

	b.ReportAllocs()

	for b.Loop() {
		_ = persist.Step(envelope)
	}
}
