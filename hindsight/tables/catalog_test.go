package tables_test

import (
	"testing"

	"github.com/apache/iceberg-go/table"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/hindsight/tables/tablestest"
)

func TestCatalogEnsure(t *testing.T) {
	Convey("The configured retry budget applies to new and existing tables", t, func() {
		previous := viper.Get("storage.iceberg.commit_retries")
		t.Cleanup(func() { viper.Set("storage.iceberg.commit_retries", previous) })
		viper.Set("storage.iceberg.commit_retries", 1)
		catalog := tablestest.New(t)
		loaded, err := catalog.Load(t.Context(), tables.Captures)
		So(err, ShouldBeNil)
		So(loaded.Properties()[table.CommitNumRetriesKey], ShouldEqual, "1")

		Convey("Changing the budget upgrades existing tables without rewriting on every boot", func() {
			viper.Set("storage.iceberg.commit_retries", 2)
			So(catalog.Ensure(t.Context()), ShouldBeNil)
			updated, err := catalog.Load(t.Context(), tables.Captures)
			So(err, ShouldBeNil)
			So(updated.Properties()[table.CommitNumRetriesKey], ShouldEqual, "2")
			So(catalog.Ensure(t.Context()), ShouldBeNil)
			unchanged, err := catalog.Load(t.Context(), tables.Captures)
			So(err, ShouldBeNil)
			So(unchanged.MetadataLocation(), ShouldEqual, updated.MetadataLocation())
		})

		Convey("Invalid budgets fail before changing the catalog", func() {
			for _, value := range []any{-1, "invalid"} {
				viper.Set("storage.iceberg.commit_retries", value)
				So(catalog.Ensure(t.Context()), ShouldNotBeNil)
			}
		})
	})
}
