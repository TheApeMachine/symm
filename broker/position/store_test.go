package position

import (
	venue "github.com/theapemachine/symm/tests/venue"
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
		Qty:        venue.Decimal("3"),
		EntryPrice: venue.Decimal("2.00"),
		EntryFee:   venue.Decimal("0.01"),
		EntryAt:    &entryAt,
	}
}

func newTestPositionStore(t testing.TB) *Store {
	t.Helper()

	store, err := NewStore(
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

func TestPositionStoreSave(t *testing.T) {
	Convey("Given a saturated writer whose database connection is temporarily occupied", t, func() {
		store := newTestPositionStore(t)
		store.database.SetMaxOpenConns(1)
		connection, err := store.database.Conn(t.Context())
		So(err, ShouldBeNil)
		defer connection.Close()
		// More lots than the complete queue plus the writer's in-flight batch.
		lots := testPositionStoreQueueDepth + testPositionStoreBatchSize + 1
		entryAt := time.Unix(1700000000, 0)
		completed := make(chan error, 1)
		go func() {
			for index := range lots {
				if err := store.Save(openLot("SATURATED/USD", entryAt.Add(time.Duration(index)))); err != nil {
					completed <- err
					return
				}
			}
			completed <- nil
		}()
		// This test deadline distinguishes durable backpressure from returning
		// success while silently discarding the unpersisted tail of the queue.
		select {
		case err := <-completed:
			t.Fatalf("full writer returned without a database connection: %v", err)
		case <-time.After(time.Second):
		}
		So(connection.Close(), ShouldBeNil)
		So(<-completed, ShouldBeNil)
		So(store.Sync(), ShouldBeNil)
		var retained int
		So(store.database.QueryRow("SELECT count(*) FROM open_positions").Scan(&retained), ShouldBeNil)
		So(retained, ShouldEqual, lots)

		Convey("A later delete is ordered after every accepted save", func() {
			So(store.Delete("SATURATED/USD"), ShouldBeNil)
			So(store.Sync(), ShouldBeNil)
			So(store.database.QueryRow("SELECT count(*) FROM open_positions").Scan(&retained), ShouldBeNil)
			So(retained, ShouldEqual, 0)
		})
	})
}

func BenchmarkPositionStoreSave(b *testing.B) {
	store, err := NewStore(
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
