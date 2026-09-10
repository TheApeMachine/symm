/*
Package tablestest builds a real Iceberg catalog for tests.

The old in-memory blob bucket has no Iceberg equivalent, so tests get an actual
catalog instead of a fake: a SQLite catalog over a temporary directory. It
exercises the same schemas, Arrow encoders, appends and scans as production —
only the catalog implementation and the file system differ — so an encoding
mistake fails here rather than only against the cluster.
*/
package tablestest

import (
	"context"
	"database/sql"
	"testing"

	"github.com/apache/iceberg-go/catalog"
	icesql "github.com/apache/iceberg-go/catalog/sql"
	"github.com/theapemachine/symm/hindsight/tables"

	// modernc's driver is pure Go, so tests need no cgo toolchain.
	_ "modernc.org/sqlite"
)

/*
New returns a catalog backed by a temporary directory, with every Hindsight
table already created. The directory is removed when the test finishes.
*/
func New(t testing.TB) *tables.Catalog {
	t.Helper()

	catalog := tables.Wrap(Underlying(t))

	if err := catalog.Ensure(context.Background()); err != nil {
		t.Fatalf("tablestest: ensure tables: %v", err)
	}

	return catalog
}

/*
Empty returns a catalog with no tables created, for exercising the failure
path. Every append against it fails because there is nothing to append to,
which reproduces a persistence failure without faking the storage layer.
*/
func Empty(t testing.TB) *tables.Catalog {
	t.Helper()

	return tables.Wrap(Underlying(t))
}

// Underlying returns an empty real catalog for catalog-boundary test adapters.
func Underlying(t testing.TB) catalog.Catalog {
	t.Helper()

	warehouse := t.TempDir()
	db, err := sql.Open("sqlite", ":memory:")

	if err != nil {
		t.Fatalf("tablestest: open sqlite: %v", err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("tablestest: close sqlite: %v", err)
		}
	})

	underlying, err := icesql.NewCatalog("test", db, icesql.SQLite, map[string]string{
		"warehouse": "file://" + warehouse,
	})

	if err != nil {
		t.Fatalf("tablestest: create catalog: %v", err)
	}

	return underlying
}
