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
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apache/iceberg-go/catalog"
	icesql "github.com/apache/iceberg-go/catalog/sql"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/system"

	// modernc's driver is pure Go, so tests need no cgo toolchain.
	_ "modernc.org/sqlite"
)

/*
New returns a catalog backed by a temporary directory, with every Hindsight
table already created. The directory is removed when the test finishes.
*/
func New(t testing.TB) *tables.Catalog {
	t.Helper()

	savedCfg := system.Cfg
	t.Cleanup(func() {
		system.Cfg = savedCfg
	})

	if system.Cfg != nil && system.Cfg.Storage != nil && system.Cfg.Storage.S3 != nil {
		s3Copy := *system.Cfg.Storage.S3
		s3Copy.Endpoint = ""
		storageCopy := *system.Cfg.Storage
		storageCopy.S3 = &s3Copy
		cfgCopy := *system.Cfg
		cfgCopy.Storage = &storageCopy
		system.Cfg = &cfgCopy
	}

	catalog := tables.Wrap(Underlying(t))

	if err := catalog.Ensure(context.Background()); err != nil {
		t.Fatalf("tablestest: ensure tables: %v", err)
	}

	return catalog
}

// Underlying returns an empty real catalog for catalog-boundary test adapters.
func Underlying(t testing.TB) catalog.Catalog {
	t.Helper()

	warehouse := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(warehouse, "catalog.db"))

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

/*
DropDataFiles deletes every Parquet data file of one table while leaving its
metadata and manifests intact. A later scan still plans those files and then
fails to read them: a real storage failure in the middle of a read, not a
missing table.
*/
func DropDataFiles(t testing.TB, catalog *tables.Catalog, name string) {
	t.Helper()

	loaded, err := catalog.Load(context.Background(), name)

	if err != nil {
		t.Fatalf("tablestest: load %s: %v", name, err)
	}

	location := strings.TrimPrefix(loaded.Location(), "file://")
	dropped := 0

	err = filepath.WalkDir(location, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() || filepath.Ext(path) != ".parquet" {
			return nil
		}

		dropped++

		return os.Remove(path)
	})

	if err != nil {
		t.Fatalf("tablestest: drop data files of %s: %v", name, err)
	}

	if dropped == 0 {
		t.Fatalf("tablestest: %s has no data files to drop under %s", name, location)
	}
}
