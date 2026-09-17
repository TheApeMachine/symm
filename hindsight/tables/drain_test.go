package tables_test

import (
	"context"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
)

func TestDrain(t *testing.T) {
	Convey("A nil catalog produces a safe no-op drain", t, func() {
		drain := tables.NewDrain(t.Context(), nil, 1)
		So(drain, ShouldNotBeNil)
		So(drain.Error(), ShouldBeNil)
	})

	Convey("Items enqueued via Next are drained without panic", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		drain := tables.NewDrain(ctx, nil, 1)

		item := map[string]any{"price": 50000.5, "symbol": "BTC/USD"}
		feed := func(yield func(unsafe.Pointer) bool) {
			if !yield(unsafe.Pointer(&item)) {
				return
			}
		}

		for range drain.Next(feed) {
		}

		cancel()
		So(drain.Error(), ShouldBeNil)
	})
}
