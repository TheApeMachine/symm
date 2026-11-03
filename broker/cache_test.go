package broker

import (
	"context"
	"iter"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/data"
)

/*
memBlob is an in-process S3 stand-in. Tests assert on real keys and bodies —
not spy call counts.
*/
type memBlob struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemBlob() *memBlob {
	return &memBlob{data: make(map[string][]byte)}
}

func (blob *memBlob) WriteAtPrefix(key string, value []byte) (int, error) {
	blob.mu.Lock()
	defer blob.mu.Unlock()

	copied := make([]byte, len(value))
	copy(copied, value)
	blob.data[key] = copied

	return len(value), nil
}

func (blob *memBlob) DeletePrefix(prefix string) error {
	blob.mu.Lock()
	defer blob.mu.Unlock()

	for key := range blob.data {
		if key == prefix || strings.HasPrefix(key, prefix) {
			delete(blob.data, key)
		}
	}

	return nil
}

func (blob *memBlob) Stream(ctx context.Context, prefix string) iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		blob.mu.Lock()
		keys := make([]string, 0, len(blob.data))

		for key := range blob.data {
			if strings.HasPrefix(key, prefix) {
				keys = append(keys, key)
			}
		}
		blob.mu.Unlock()

		for _, key := range keys {
			if ctx.Err() != nil {
				return
			}

			blob.mu.Lock()
			raw := blob.data[key]
			blob.mu.Unlock()

			if len(raw) == 0 {
				continue
			}

			if !yield(raw) {
				return
			}
		}
	}
}

func (blob *memBlob) keys() []string {
	blob.mu.Lock()
	defer blob.mu.Unlock()

	out := make([]string, 0, len(blob.data))
	for key := range blob.data {
		out = append(out, key)
	}

	return out
}

func (blob *memBlob) get(key string) []byte {
	blob.mu.Lock()
	defer blob.mu.Unlock()

	return blob.data[key]
}

func TestCachePersistsClosedPrefix(t *testing.T) {
	Convey("Add writes virtual/open, Remove moves to virtual/closed", t, func() {
		ctx := context.Background()
		blob := newMemBlob()
		cache := newCache(ctx, "virtual", nil, nil, nil, blob)

		now := time.Now().UTC()
		position := &Position{
			ID:       "lot-ake-1",
			Symbol:   "AKE/USD",
			Book:     "virtual",
			OpenedAt: now,
		}

		So(cache.Add(position.Symbol, position, true), ShouldBeFalse)

		openKey := "virtual/open/AKE/USD/lot-ake-1"
		So(blob.keys(), ShouldResemble, []string{openKey})

		raw := blob.get(openKey)
		So(len(raw), ShouldBeGreaterThan, 0)

		restored := cache.Restore(raw)
		So(restored, ShouldNotBeNil)
		So(restored.Symbol, ShouldEqual, "AKE/USD")
		So(restored.ID, ShouldEqual, "lot-ake-1")
		So(restored.ClosedAt, ShouldBeNil)

		closedAt := now.Add(time.Hour)
		position.ClosedAt = &closedAt
		position.ExitTrigger = "stoploss"

		So(cache.Remove(position.Symbol), ShouldBeTrue)

		closedKey := "virtual/closed/AKE/USD/lot-ake-1"
		So(blob.get(openKey), ShouldBeNil)
		So(len(blob.get(closedKey)), ShouldBeGreaterThan, 0)

		closed := cache.Restore(blob.get(closedKey))
		So(closed, ShouldNotBeNil)
		So(closed.ClosedAt, ShouldNotBeNil)
		So(closed.ExitTrigger, ShouldEqual, "stoploss")

		So(len(cache.Open()), ShouldEqual, 0)
		So(len(cache.Closed()), ShouldEqual, 1)
		So(cache.Closed()[0].ID, ShouldEqual, "lot-ake-1")
	})

	Convey("Reload from blob resurrects closed lots via lotRecord wrapper", t, func() {
		ctx := context.Background()
		blob := newMemBlob()
		warm := newCache(ctx, "virtual", nil, nil, nil, blob)

		closedAt := time.Now().UTC()
		position := &Position{
			ID:       "lot-reload-1",
			Symbol:   "BTC/USD",
			Book:     "virtual",
			ClosedAt: &closedAt,
		}

		So(warm.Add(position.Symbol, position, false), ShouldBeFalse)

		cold := newCache(ctx, "virtual", nil, nil, nil, blob)
		So(len(cold.Open()), ShouldEqual, 0)
		So(len(cold.Closed()), ShouldEqual, 1)
		So(cold.Closed()[0].Symbol, ShouldEqual, "BTC/USD")
		So(cold.Closed()[0].ID, ShouldEqual, "lot-reload-1")
	})

	Convey("Bare position JSON is rejected — hub expects lotRecord", t, func() {
		ctx := context.Background()
		cache := newCache(ctx, "virtual", nil, nil, nil, newMemBlob())

		So(cache.Restore([]byte(`{"id":"x","symbol":"ETH/USD"}`)), ShouldBeNil)
		So(cache.Restore([]byte(
			`{"epoch":1,"tick":2,"timestamp":3,"position":{"id":"x","symbol":"ETH/USD"}}`,
		)), ShouldNotBeNil)
	})

	Convey("lotRecord carries epoch/tick from the live desk clock", t, func() {
		ctx := context.Background()
		blob := newMemBlob()
		cache := newCache(ctx, "paper", nil, nil, nil, blob)

		data.Epoch.Store(42)
		data.Tick.Store(7)

		position := &Position{ID: "lot-clock", Symbol: "ETH/USD", Book: "paper"}
		So(cache.Add(position.Symbol, position, true), ShouldBeFalse)

		var record lotRecord
		So(sonicUnmarshal(blob.get("paper/open/ETH/USD/lot-clock"), &record), ShouldBeNil)
		So(record.Epoch, ShouldEqual, 42)
		So(record.Tick, ShouldEqual, 7)
		So(record.Position, ShouldNotBeNil)
		So(record.Position.Symbol, ShouldEqual, "ETH/USD")
	})
}

func sonicUnmarshal(raw []byte, dst any) error {
	return sonic.Unmarshal(raw, dst)
}
