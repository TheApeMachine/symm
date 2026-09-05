package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

const (
	testPositionStoreQueueDepth = 64
	testPositionStoreBatchSize  = 8
)

/* openLot builds one filled lot's durable entry facts. */
func openLot(symbol string, entryAt time.Time) *types.Holding {
	return &types.Holding{
		Symbol:     symbol,
		Status:     types.OPEN,
		Qty:        mustDecimal("3"),
		EntryPrice: mustDecimal("2.00"),
		EntryFee:   mustDecimal("0.01"),
		EntryAt:    &entryAt,
	}
}

func newTestPositionStore(t *testing.T) *PositionStore {
	t.Helper()

	store, err := NewPositionStore(
		t.TempDir()+"/positions.sqlite",
		testPositionStoreQueueDepth,
		testPositionStoreBatchSize,
	)

	if err != nil {
		t.Fatalf("open position store: %v", err)
	}

	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestPositionStoreOpenLots(t *testing.T) {
	entryAt := time.Unix(1_700_000_000, 0).UTC()

	Convey("Given a store holding one open lot's entry facts", t, func() {
		store := newTestPositionStore(t)
		So(store.Save(openLot("AAA/USD", entryAt)), ShouldBeNil)

		Convey("It reads back the basis the lot was actually opened on", func() {
			holding, err := store.Load(t.Context(), "AAA/USD", entryAt)
			So(err, ShouldBeNil)
			So(holding, ShouldNotBeNil)
			So(holding.Symbol, ShouldEqual, "AAA/USD")
			So(holding.EntryPrice.String(), ShouldEqual, "2.00")
			So(holding.Qty.String(), ShouldEqual, "3")
		})

		Convey("A different entry time is a different lot, never this one's basis", func() {
			holding, err := store.Load(t.Context(), "AAA/USD", entryAt.Add(time.Second))
			So(err, ShouldBeNil)
			So(holding, ShouldBeNil)
		})

		Convey("A lot that was never stored reports absent rather than empty facts", func() {
			holding, err := store.Load(t.Context(), "BBB/USD", entryAt)
			So(err, ShouldBeNil)
			So(holding, ShouldBeNil)
		})

		Convey("Closing the position clears every row the symbol left behind", func() {
			So(store.Save(openLot("AAA/USD", entryAt.Add(time.Hour))), ShouldBeNil)
			So(store.Delete("AAA/USD"), ShouldBeNil)
			So(store.Sync(), ShouldBeNil)

			holding, err := store.Load(t.Context(), "AAA/USD", entryAt)
			So(err, ShouldBeNil)
			So(holding, ShouldBeNil)
		})
	})

	Convey("Given a holding that never filled", t, func() {
		store := newTestPositionStore(t)

		Convey("It is refused rather than written as a position that opened", func() {
			So(store.Save(&types.Holding{Symbol: "AAA/USD"}), ShouldNotBeNil)
			So(store.Save(nil), ShouldNotBeNil)
		})
	})
}

func BenchmarkPositionStoreSave(b *testing.B) {
	store, err := NewPositionStore(
		b.TempDir()+"/positions.sqlite",
		1024,
		128,
	)

	if err != nil {
		b.Fatal(err)
	}

	entryAt := time.Unix(1_700_000_000, 0).UTC()
	holding := openLot("BENCH/USD", entryAt)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := store.Save(holding); err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()

	if err := store.Close(); err != nil {
		b.Fatal(err)
	}
}
