package broker

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

const (
	testPositionStoreQueueDepth = 64
	testPositionStoreBatchSize  = 16
)

/*
legacyStoplossRow marshals a Stoploss the way a pre-entry_at build did: state
JSON with no entry_at field at all, inserted into a symbol-only schema table.
*/
func legacyStoplossRow(t *testing.T, symbol string) []byte {
	t.Helper()

	stoploss := &types.Stoploss{
		Symbol:        symbol,
		Status:        types.ARMED,
		TickSize:      mustDecimal("0.01"),
		TrailDistance: mustDecimal("0.1"),
		Floor:         mustDecimal("1.5"),
		Mark:          mustDecimal("2.0"),
		Peak:          mustDecimal("2.2"),
		ProfitLine:    mustDecimal("1.8"),
		ArmAt:         mustDecimal("1.7"),
		LockFloor:     mustDecimal("1.6"),
	}

	state, err := stoploss.MarshalState()
	if err != nil {
		t.Fatalf("failed to marshal legacy stoploss: %v", err)
	}

	return state
}

func TestPositionStoreEnsureSchema(t *testing.T) {
	Convey("Given a database created under the pre-entry_at symbol-only schema", t, func() {
		storePath := t.TempDir() + "/legacy.sqlite"

		seed, err := sql.Open("sqlite3", storePath)
		if err != nil {
			t.Fatalf("failed to open seed database: %v", err)
		}

		if _, err := seed.Exec(`
CREATE TABLE position_stoplosses (
    symbol TEXT PRIMARY KEY,
    state BLOB NOT NULL
) STRICT;`); err != nil {
			t.Fatalf("failed to create legacy schema: %v", err)
		}

		if _, err := seed.Exec(
			"INSERT INTO position_stoplosses (symbol, state) VALUES (?, ?)",
			"AAA/USD", legacyStoplossRow(t, "AAA/USD"),
		); err != nil {
			t.Fatalf("failed to seed legacy row: %v", err)
		}

		if err := seed.Close(); err != nil {
			t.Fatalf("failed to close seed database: %v", err)
		}

		Convey("Opening it through NewPositionStore migrates the table without data loss of position identity", func() {
			store, err := NewPositionStore(
				storePath, testPositionStoreQueueDepth, testPositionStoreBatchSize,
			)
			So(err, ShouldBeNil)
			defer store.Close()

			var columnCount int
			err = store.database.QueryRow(
				"SELECT count(*) FROM pragma_table_info('position_stoplosses') WHERE name = 'entry_at'",
			).Scan(&columnCount)
			So(err, ShouldBeNil)
			So(columnCount, ShouldEqual, 1)

			var legacyTableCount int
			err = store.database.QueryRow(
				"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='position_stoplosses_pre_entry_at'",
			).Scan(&legacyTableCount)
			So(err, ShouldBeNil)
			So(legacyTableCount, ShouldEqual, 0)

			Convey("A legacy row with no entry_at is dropped, not silently misattributed to a new lot", func() {
				var rowCount int
				err = store.database.QueryRow(
					"SELECT count(*) FROM position_stoplosses WHERE symbol = 'AAA/USD'",
				).Scan(&rowCount)
				So(err, ShouldBeNil)
				So(rowCount, ShouldEqual, 0)
			})

			Convey("The store is fully usable afterward for new saves and loads", func() {
				entryAt := time.Unix(1700000000, 0).UTC()
				stoploss := &types.Stoploss{
					Symbol:        "BBB/USD",
					Status:        types.ARMED,
					TickSize:      mustDecimal("0.01"),
					TrailDistance: mustDecimal("0.1"),
					Floor:         mustDecimal("1.5"),
					Mark:          mustDecimal("2.0"),
					Peak:          mustDecimal("2.2"),
					ProfitLine:    mustDecimal("1.8"),
					ArmAt:         mustDecimal("1.7"),
					LockFloor:     mustDecimal("1.6"),
					EntryAt:       &entryAt,
				}

				So(store.Save(stoploss), ShouldBeNil)

				loaded, err := store.Load(t.Context(), "BBB/USD", entryAt)
				So(err, ShouldBeNil)
				So(loaded, ShouldNotBeNil)
				So(loaded.Symbol, ShouldEqual, "BBB/USD")
			})
		})
	})

	Convey("Given a fresh database with no existing tables", t, func() {
		storePath := t.TempDir() + "/fresh.sqlite"

		Convey("NewPositionStore creates the current schema directly", func() {
			store, err := NewPositionStore(
				storePath, testPositionStoreQueueDepth, testPositionStoreBatchSize,
			)
			So(err, ShouldBeNil)
			defer store.Close()

			var columnCount int
			err = store.database.QueryRow(
				"SELECT count(*) FROM pragma_table_info('position_stoplosses') WHERE name = 'entry_at'",
			).Scan(&columnCount)
			So(err, ShouldBeNil)
			So(columnCount, ShouldEqual, 1)
		})
	})

	Convey("Given a database already migrated to the current schema", t, func() {
		storePath := t.TempDir() + "/current.sqlite"

		store, err := NewPositionStore(
			storePath, testPositionStoreQueueDepth, testPositionStoreBatchSize,
		)
		if err != nil {
			t.Fatalf("failed to open store: %v", err)
		}

		entryAt := time.Unix(1700000000, 0).UTC()
		stoploss := &types.Stoploss{
			Symbol:        "CCC/USD",
			Status:        types.ARMED,
			TickSize:      mustDecimal("0.01"),
			TrailDistance: mustDecimal("0.1"),
			Floor:         mustDecimal("1.5"),
			Mark:          mustDecimal("2.0"),
			Peak:          mustDecimal("2.2"),
			ProfitLine:    mustDecimal("1.8"),
			ArmAt:         mustDecimal("1.7"),
			LockFloor:     mustDecimal("1.6"),
			EntryAt:       &entryAt,
		}

		if err := store.Save(stoploss); err != nil {
			t.Fatalf("failed to save stoploss: %v", err)
		}

		if err := store.Close(); err != nil {
			t.Fatalf("failed to close store: %v", err)
		}

		Convey("Re-opening it is a no-op that preserves the existing row", func() {
			reopened, err := NewPositionStore(
				storePath, testPositionStoreQueueDepth, testPositionStoreBatchSize,
			)
			So(err, ShouldBeNil)
			defer reopened.Close()

			loaded, err := reopened.Load(t.Context(), "CCC/USD", entryAt)
			So(err, ShouldBeNil)
			So(loaded, ShouldNotBeNil)
			So(loaded.Symbol, ShouldEqual, "CCC/USD")
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
	stoploss := &types.Stoploss{
		Symbol:        "BENCH/USD",
		Status:        types.ARMED,
		TickSize:      mustDecimal("0.01"),
		TrailDistance: mustDecimal("0.1"),
		Floor:         mustDecimal("1.5"),
		Mark:          mustDecimal("2.0"),
		Peak:          mustDecimal("2.2"),
		ProfitLine:    mustDecimal("1.8"),
		ArmAt:         mustDecimal("1.7"),
		LockFloor:     mustDecimal("1.6"),
		EntryAt:       &entryAt,
	}
	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		if err := store.Save(stoploss); err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()

	if err := store.Close(); err != nil {
		b.Fatal(err)
	}
}
