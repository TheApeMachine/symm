package tables

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/ipc"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestReplayBatchRead(t *testing.T) {
	Convey("Indexed rows use the retained binary batch's own offsets", t, func() {
		database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "index.sqlite"))
		So(err, ShouldBeNil)
		defer func() { So(database.Close(), ShouldBeNil) }()
		_, err = database.Exec("CREATE TABLE batches (id INTEGER PRIMARY KEY, remaining INTEGER, payload BLOB)")
		So(err, ShouldBeNil)
		observations := []*data.Measurement[float64]{
			data.NewMeasurement("first", map[string]data.Metric[float64]{"left": {Raw: 11}}),
			data.NewMeasurement("second", map[string]data.Metric[float64]{"right": {Raw: 22}}),
		}
		reader, err := measurementRecords(MeasurementSchema(), observations, 1)
		So(err, ShouldBeNil)
		defer reader.Release()
		So(reader.Next(), ShouldBeTrue)
		var payload bytes.Buffer
		writer := ipc.NewWriter(&payload, ipc.WithSchema(reader.Schema()))
		So(writer.Write(reader.RecordBatch()), ShouldBeNil)
		So(writer.Close(), ShouldBeNil)
		_, err = database.Exec("INSERT INTO batches VALUES (1, 2, ?)", payload.Bytes())
		So(err, ShouldBeNil)
		batch := &replayBatch{}
		defer batch.Close()

		for _, ordinal := range []int64{1, 0, 1} {
			measurement, err := batch.Read(t.Context(), database, 1, ordinal)
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, observations[ordinal].Source)
			for key, metric := range observations[ordinal].Metrics {
				So(measurement.Metrics[key].Raw, ShouldEqual, metric.Raw)
			}
		}

		Convey("Out-of-range addresses fail explicitly", func() {
			_, err := batch.Read(t.Context(), database, 1, 2)
			So(err, ShouldNotBeNil)
		})

		Convey("Corrupt batch bytes fail without substituting previous data", func() {
			_, err := database.Exec("INSERT INTO batches VALUES (2, 1, ?)", []byte("broken"))
			So(err, ShouldBeNil)
			measurement, err := batch.Read(t.Context(), database, 2, 0)
			So(err, ShouldNotBeNil)
			So(measurement, ShouldBeNil)
		})
	})
}

func TestReplayBatchClose(t *testing.T) {
	Convey("Closing an unused or already closed cache is safe", t, func() {
		batch := &replayBatch{}
		batch.Close()
		batch.Close()
		So(batch.record, ShouldBeNil)
	})
}
