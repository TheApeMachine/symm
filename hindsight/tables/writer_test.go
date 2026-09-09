package tables_test

import (
	"bytes"
	"math"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/hindsight/tables/tablestest"
)

func TestWriterCommit(t *testing.T) {
	if os.Getenv("SYMM_LARGE_PAYLOAD_TEST") != "1" {
		t.Skip("set SYMM_LARGE_PAYLOAD_TEST=1 to exercise the real 2 GiB Arrow offset boundary")
	}

	Convey("One snapshot preserves binary payloads exceeding a signed 32-bit offset in total", t, func() {
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)
		// Repeating a one-MiB payload crosses the actual Arrow Binary boundary
		// without retaining separate copies in the input fixture.
		payload := bytes.Repeat([]byte("x"), 1<<20)
		count := math.MaxInt32/len(payload) + 2

		for index := range count {
			writer.AddWitness(tables.WitnessRow{
				Run: "large", ArtifactKind: "precursor",
				Envelope: tables.EnvelopeRefRow{Run: "large", Sequence: int64(index + 1)},
				Payload:  payload,
			})
		}
		So(writer.Commit(t.Context()), ShouldBeNil)
		loaded, err := catalog.Load(t.Context(), tables.Witnesses)
		So(err, ShouldBeNil)
		So(len(loaded.Metadata().Snapshots()), ShouldEqual, 1)
		rows, err := catalog.Witnesses(t.Context(), "large", "precursor")
		So(err, ShouldBeNil)
		So(len(rows), ShouldEqual, count)

		seen := make(map[int64]bool, count)
		for _, row := range rows {
			So(seen[row.Envelope.Sequence], ShouldBeFalse)
			seen[row.Envelope.Sequence] = true
			So(bytes.Equal(row.Payload, payload), ShouldBeTrue)
		}
		after, err := catalog.Load(t.Context(), tables.Witnesses)
		So(err, ShouldBeNil)
		So(after.MetadataLocation(), ShouldEqual, loaded.MetadataLocation())

		Convey("A single unrepresentable payload fails before another snapshot is committed", func() {
			writer.AddWitness(tables.WitnessRow{
				Run: "large", ArtifactKind: "precursor",
				Payload: make([]byte, int64(math.MaxInt32)+1),
			})
			err := writer.Commit(t.Context())
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "one payload exceeds Arrow Binary")
			after, err := catalog.Load(t.Context(), tables.Witnesses)
			So(err, ShouldBeNil)
			So(after.MetadataLocation(), ShouldEqual, loaded.MetadataLocation())
		})
	})
}
