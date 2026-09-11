package workbench_test

import (
	"bytes"
	"context"
	"database/sql"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/ipc"
	_ "github.com/marcboeker/go-duckdb/v2"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/workbench"
)

/*
local opens an in-memory DuckDB carrying one table shaped like the Hindsight
layout: flat columns beside the nested ones (a list and a map) that the viewer
cannot hold. The Iceberg attachment is the only part of the engine this
replaces, so projection, encoding, and listing run exactly as they do against
the catalog.
*/
func local(t *testing.T) *workbench.Warehouse {
	t.Helper()

	db, err := sql.Open("duckdb", "")
	So(err, ShouldBeNil)

	ctx := context.Background()

	for _, statement := range []string{
		"INSTALL arrow FROM community",
		"LOAD arrow",
		"ATTACH ':memory:' AS symmtables",
		"CREATE SCHEMA symmtables.hindsight",
		`CREATE TABLE symmtables.hindsight.decisions (
			run VARCHAR,
			"at" TIMESTAMP WITH TIME ZONE,
			cost DECIMAL(38,18),
			context BIGINT[],
			versions MAP(VARCHAR, VARCHAR)
		)`,
		`INSERT INTO symmtables.hindsight.decisions VALUES
			('run-a', '2026-09-09 00:00:00+00', 1.5, [1, 2, 3], MAP {'capture': '3'}),
			('run-a', '2026-09-09 00:00:01+00', 2.5, [4, 5], MAP {'capture': '3'})`,
	} {
		_, err := db.ExecContext(ctx, statement)
		So(err, ShouldBeNil)
	}

	warehouse := workbench.Wrap(db)
	t.Cleanup(func() {
		if err := warehouse.Close(); err != nil {
			t.Fatalf("close warehouse: %v", err)
		}
	})

	return warehouse
}

// rowsOf decodes an Arrow IPC stream into its row count and column names.
func rowsOf(t *testing.T, stream []byte) (int64, []string) {
	t.Helper()

	reader, err := ipc.NewReader(bytes.NewReader(stream))
	So(err, ShouldBeNil)

	defer reader.Release()

	var rows int64

	for reader.Next() {
		rows += reader.RecordBatch().NumRows()
	}

	So(reader.Err(), ShouldBeNil)

	names := make([]string, 0, reader.Schema().NumFields())

	for _, field := range reader.Schema().Fields() {
		names = append(names, field.Name)
	}

	return rows, names
}

func TestWarehouseQuery(t *testing.T) {
	Convey("Given a warehouse holding nested columns the viewer cannot hold", t, func() {
		warehouse := local(t)
		ctx := context.Background()

		Convey("When a query selects them alongside flat columns", func() {
			stream, err := warehouse.Execute(
				ctx, `SELECT * FROM symmtables.hindsight.decisions ORDER BY "at"`,
			)

			So(err, ShouldBeNil)

			rows, names := rowsOf(t, stream)

			Convey("Then every column survives under its own name", func() {
				So(names, ShouldResemble, []string{"run", "at", "cost", "context", "versions"})
			})

			/*
				A list column reaches the viewer as one row per element unless it
				is rendered first, which multiplies the row count of the answer.
			*/
			Convey("Then the answer has one row per record, not one per list element", func() {
				So(rows, ShouldEqual, 2)
			})

			Convey("Then the nested columns arrive as their JSON rendering", func() {
				So(string(stream), ShouldContainSubstring, "[1,2,3]")
				So(string(stream), ShouldContainSubstring, `{"capture":"3"}`)
			})
		})

		Convey("When a query aggregates", func() {
			stream, err := warehouse.Execute(
				ctx, "SELECT run, count(*) AS decisions FROM symmtables.hindsight.decisions GROUP BY run",
			)

			So(err, ShouldBeNil)

			rows, names := rowsOf(t, stream)

			Convey("Then the aggregate is what the viewer receives", func() {
				So(names, ShouldResemble, []string{"run", "decisions"})
				So(rows, ShouldEqual, 1)
			})
		})

		Convey("When the SQL is not valid", func() {
			_, err := warehouse.Execute(ctx, "SELECT nonexistent FROM symmtables.hindsight.decisions")

			Convey("Then the failure surfaces instead of an empty result", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

/*
The viewer materializes a view under a name, reads it across later statements,
and drops it. Those statements produce no rows, and they only work if every one
of them lands in the same session.
*/
func TestWarehouseExecuteSession(t *testing.T) {
	Convey("Given a warehouse the viewer is driving", t, func() {
		warehouse := local(t)
		ctx := context.Background()

		Convey("When it materializes a view", func() {
			stream, err := warehouse.Execute(ctx, `CREATE OR REPLACE TABLE v1 AS (
				SELECT run, count(*) AS graded FROM symmtables.hindsight.decisions GROUP BY run
			)`)

			So(err, ShouldBeNil)

			Convey("Then the statement returns no rows rather than failing", func() {
				So(stream, ShouldBeEmpty)
			})

			Convey("Then a later statement still sees it", func() {
				read, err := warehouse.Execute(ctx, "SELECT * FROM v1")

				So(err, ShouldBeNil)

				rows, names := rowsOf(t, read)
				So(names, ShouldResemble, []string{"run", "graded"})
				So(rows, ShouldEqual, 1)
			})

			Convey("Then dropping it is also rowless, and it is gone", func() {
				dropped, err := warehouse.Execute(ctx, "DROP TABLE IF EXISTS v1")

				So(err, ShouldBeNil)
				So(dropped, ShouldBeEmpty)

				_, err = warehouse.Execute(ctx, "SELECT * FROM v1")
				So(err, ShouldNotBeNil)
			})
		})

		/*
			getHostedTables and tableSchema are how the viewer discovers what it
			can show; both parse as selects and must carry rows back.
		*/
		Convey("When the viewer asks what the warehouse holds", func() {
			listing, err := warehouse.Execute(ctx, "SHOW ALL TABLES")

			So(err, ShouldBeNil)

			rows, _ := rowsOf(t, listing)

			Convey("Then the listing comes back as rows", func() {
				So(rows, ShouldBeGreaterThan, 0)
			})
		})

		Convey("When a client sends a SELECT without a selection list", func() {
			stream, err := warehouse.Execute(ctx, "SELECT FROM symmtables.hindsight.decisions")

			So(err, ShouldBeNil)

			rows, columns := rowsOf(t, stream)

			Convey("Then the query succeeds with a placeholder selection rather than failing parser", func() {
				So(rows, ShouldEqual, 2)
				So(len(columns), ShouldEqual, 1)
			})
		})
	})
}
