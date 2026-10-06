package tables_test

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestCatalog_BlobStorageUnconfigured(t *testing.T) {
	Convey("Given a catalog without object storage", t, func() {
		catalog := tablestest.New(t)

		Convey("GetBlob and PutBlob fail with ErrBlobStorageUnconfigured", func() {
			_, err := catalog.GetBlob(t.Context(), "grid/latest")
			So(errors.Is(err, tables.ErrBlobStorageUnconfigured), ShouldBeTrue)
			So(errors.Is(err, tables.ErrBlobMissing), ShouldBeFalse)

			err = catalog.PutBlob(t.Context(), "grid/latest", []byte("x"))
			So(errors.Is(err, tables.ErrBlobStorageUnconfigured), ShouldBeTrue)
		})
	})
}
