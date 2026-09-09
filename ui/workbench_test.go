package ui

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/ipc"
	"github.com/gofiber/fiber/v3"
	_ "github.com/marcboeker/go-duckdb/v2"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/workbench"
)

/*
workbenchHub mounts the workbench routes over a local DuckDB carrying one
table. The Iceberg attachment is the only thing this replaces: the handlers,
the projection, and the Arrow encoding are the production ones.
*/
func workbenchHub(t *testing.T) *Hub {
	t.Helper()

	db, err := sql.Open("duckdb", "")
	So(err, ShouldBeNil)

	ctx := context.Background()

	for _, statement := range []string{
		"INSTALL arrow FROM community",
		"LOAD arrow",
		"ATTACH ':memory:' AS symmtables",
		"CREATE SCHEMA symmtables.hindsight",
		"CREATE TABLE symmtables.hindsight.runs (id VARCHAR, positions INTEGER)",
		"INSERT INTO symmtables.hindsight.runs VALUES ('run-a', 2), ('run-b', 0)",
	} {
		_, err := db.ExecContext(ctx, statement)
		So(err, ShouldBeNil)
	}

	hub := &Hub{
		ctx:       ctx,
		app:       fiber.New(),
		warehouse: workbench.Wrap(db),
	}

	hub.registerWorkbench()

	t.Cleanup(func() {
		if err := hub.warehouse.Close(); err != nil {
			t.Fatalf("close warehouse: %v", err)
		}
	})

	return hub
}

func TestRegisterWorkbench(t *testing.T) {
	Convey("Given the workbench mounted over a warehouse", t, func() {
		hub := workbenchHub(t)

		/*
			The viewer materializes a view before reading it, and that statement
			produces no rows. It must come back as an empty stream, not an error.
		*/
		Convey("When the viewer posts a statement that produces no rows", func() {
			request := httptest.NewRequest(
				http.MethodPost, "/workbench/query",
				bytes.NewBufferString(
					`{"sql":"CREATE OR REPLACE TABLE v1 AS (SELECT * FROM symmtables.hindsight.runs)"}`,
				),
			)

			request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

			response, err := hub.app.Test(request)
			So(err, ShouldBeNil)

			defer response.Body.Close()

			body, err := io.ReadAll(response.Body)
			So(err, ShouldBeNil)

			Convey("Then the stream is empty and the view is readable afterwards", func() {
				So(response.StatusCode, ShouldEqual, http.StatusOK)
				So(body, ShouldBeEmpty)

				read := httptest.NewRequest(
					http.MethodPost, "/workbench/query",
					bytes.NewBufferString(`{"sql":"SELECT * FROM v1"}`),
				)

				read.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

				second, err := hub.app.Test(read)
				So(err, ShouldBeNil)

				defer second.Body.Close()

				So(second.StatusCode, ShouldEqual, http.StatusOK)
			})
		})

		Convey("When it posts a query", func() {
			request := httptest.NewRequest(
				http.MethodPost, "/workbench/query",
				bytes.NewBufferString(`{"sql":"SELECT * FROM symmtables.hindsight.runs"}`),
			)

			request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

			response, err := hub.app.Test(request)
			So(err, ShouldBeNil)

			defer response.Body.Close()

			body, err := io.ReadAll(response.Body)
			So(err, ShouldBeNil)

			Convey("Then the body is an Arrow IPC stream the viewer can load", func() {
				So(response.StatusCode, ShouldEqual, http.StatusOK)
				So(response.Header.Get(fiber.HeaderContentType), ShouldEqual, arrowStream)

				reader, err := ipc.NewReader(bytes.NewReader(body))
				So(err, ShouldBeNil)

				defer reader.Release()

				var rows int64

				for reader.Next() {
					rows += reader.RecordBatch().NumRows()
				}

				So(reader.Err(), ShouldBeNil)
				So(rows, ShouldEqual, 2)
			})
		})

		Convey("When it posts a query the engine cannot answer", func() {
			request := httptest.NewRequest(
				http.MethodPost, "/workbench/query",
				bytes.NewBufferString(`{"sql":"SELECT * FROM nowhere"}`),
			)

			request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

			response, err := hub.app.Test(request)
			So(err, ShouldBeNil)

			defer response.Body.Close()

			Convey("Then the engine's own message comes back rather than an empty table", func() {
				So(response.StatusCode, ShouldEqual, http.StatusBadRequest)

				body, err := io.ReadAll(response.Body)
				So(err, ShouldBeNil)
				So(string(body), ShouldContainSubstring, "nowhere")
			})
		})

		Convey("When it posts no query at all", func() {
			request := httptest.NewRequest(
				http.MethodPost, "/workbench/query", bytes.NewBufferString(`{"sql":""}`),
			)

			request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

			response, err := hub.app.Test(request)
			So(err, ShouldBeNil)

			defer response.Body.Close()

			Convey("Then the request is rejected", func() {
				So(response.StatusCode, ShouldEqual, http.StatusBadRequest)
			})
		})
	})
}
