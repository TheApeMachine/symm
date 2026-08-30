package store

import (
	"database/sql"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewSQLite(t *testing.T) {
	Convey("Given a store package", t, func() {
		Convey("Constructing with an empty path", func() {
			engine, err := NewSQLite("")

			So(engine, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})

		Convey("Constructing with a valid file path", func() {
			path := t.TempDir() + "/events.sqlite"
			engine, err := NewSQLite(path)

			So(err, ShouldBeNil)
			So(engine, ShouldNotBeNil)

			Reset(func() {
				_ = engine.Close()
			})

			Convey("Writing and reading back one frame round-trips the bytes and endpoint", func() {
				at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
				err := engine.WriteFrame("wss://example/ws", "ticker", []byte("hello"), at)

				So(err, ShouldBeNil)

				rows, queryErr := engine.database.Query(
					"SELECT kind, endpoint, data FROM events WHERE kind = 'ticker'",
				)

				So(queryErr, ShouldBeNil)

				defer rows.Close()

				var kind string
				var endpoint string
				var data []byte
				found := rows.Next()

				So(found, ShouldBeTrue)

				So(rows.Scan(&kind, &endpoint, &data), ShouldBeNil)
				So(kind, ShouldEqual, "ticker")
				So(endpoint, ShouldEqual, "wss://example/ws")
				So(string(data), ShouldEqual, "hello")
			})
		})
	})
}

func TestSQLiteWriteFrame(t *testing.T) {
	Convey("Given an open sqlite engine", t, func() {
		path := t.TempDir() + "/events.sqlite"
		engine, err := NewSQLite(path)
		So(err, ShouldBeNil)

		Reset(func() {
			_ = engine.Close()
		})

		Convey("Writing multiple kinds keeps them distinct by kind", func() {
			So(engine.WriteFrame("wss://a", "ticker", []byte("1"), time.Now()), ShouldBeNil)
			So(engine.WriteFrame("wss://b", "trade", []byte("2"), time.Now()), ShouldBeNil)
			So(engine.WriteFrame("wss://a", "ticker", []byte("3"), time.Now()), ShouldBeNil)

			rows, queryErr := engine.database.Query(
				"SELECT kind FROM events ORDER BY id",
			)

			So(queryErr, ShouldBeNil)

			defer rows.Close()

			kinds := make([]string, 0, 3)

			for rows.Next() {
				var kind string
				So(rows.Scan(&kind), ShouldBeNil)
				kinds = append(kinds, kind)
			}

			So(kinds, ShouldResemble, []string{"ticker", "trade", "ticker"})
		})
	})
}

func TestSQLiteEndpointMigration(t *testing.T) {
	Convey("Given a database created before the endpoint column split", t, func() {
		path := t.TempDir() + "/events.sqlite"

		rawDatabase, err := sql.Open("sqlite3", path)
		So(err, ShouldBeNil)

		_, err = rawDatabase.Exec(`
CREATE TABLE events (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT NOT NULL,
    at   TEXT NOT NULL,
    data BLOB NOT NULL
);`)
		So(err, ShouldBeNil)

		So(rawDatabase.Close(), ShouldBeNil)

		Convey("Opening it through NewSQLite adds the endpoint column without losing rows", func() {
			engine, err := NewSQLite(path)
			So(err, ShouldBeNil)

			Reset(func() {
				_ = engine.Close()
			})

			So(engine.WriteFrame("", "legacy", []byte("kept"), time.Now()), ShouldBeNil)

			var endpoint string

			So(engine.database.QueryRow(
				"SELECT endpoint FROM events WHERE kind = 'legacy'",
			).Scan(&endpoint), ShouldBeNil)

			// New records default to an empty endpoint, not a welded URL.
			So(endpoint, ShouldEqual, "")
		})
	})
}
