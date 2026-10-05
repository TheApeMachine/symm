package tables_test

import (
	"testing"
	"time"

	"github.com/apache/iceberg-go"
	icecat "github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/table"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestCatalog_EnsureEvolvesSchema(t *testing.T) {
	Convey("Given a measurements table created before the tick column existed", t, func() {
		underlying := tablestest.Underlying(t)
		canonical := tables.MeasurementSchema()

		legacy := make([]iceberg.NestedField, 0, len(canonical.Fields())-1)

		for _, field := range canonical.Fields() {
			if field.Name != "tick" {
				legacy = append(legacy, field)
			}
		}

		So(underlying.CreateNamespace(t.Context(), table.Identifier{tables.Namespace}, nil), ShouldBeNil)

		partitioning := tables.MeasurementPartitioning()
		_, err := underlying.CreateTable(
			t.Context(),
			table.Identifier{tables.Namespace, tables.Measurements},
			iceberg.NewSchema(0, legacy...),
			icecat.WithPartitionSpec(&partitioning),
		)
		So(err, ShouldBeNil)

		catalog := tables.Wrap(underlying)

		Convey("Ensure adds the column and the writer round-trips tick", func() {
			So(catalog.Ensure(t.Context()), ShouldBeNil)

			loaded, err := catalog.Load(t.Context(), tables.Measurements)
			So(err, ShouldBeNil)

			_, found := loaded.Schema().FindFieldByName("tick")
			So(found, ShouldBeTrue)

			writer := tables.NewWriter(catalog, 7)
			measurement := data.NewMeasurement("spot:trade", nil)
			measurement.Epoch = 7
			measurement.Label = "BTC/USD"
			measurement.Tick = 42
			measurement.SeqIdx = 1
			measurement.At = time.Now().UTC()
			writer.Add(tables.Measurements, data.Publication{Measurement: measurement})
			So(writer.CommitReady(t.Context(), true), ShouldBeNil)

			var ticks []int64

			for restored := range catalog.Scan(t.Context(), tables.Measurements, 7, nil, 0) {
				ticks = append(ticks, restored.Tick)
			}

			So(ticks, ShouldResemble, []int64{42})
		})

		Convey("A second Ensure leaves the evolved table untouched", func() {
			So(catalog.Ensure(t.Context()), ShouldBeNil)

			first, err := catalog.Load(t.Context(), tables.Measurements)
			So(err, ShouldBeNil)

			So(catalog.Ensure(t.Context()), ShouldBeNil)

			second, err := catalog.Load(t.Context(), tables.Measurements)
			So(err, ShouldBeNil)
			So(second.MetadataLocation(), ShouldEqual, first.MetadataLocation())
		})
	})
}
