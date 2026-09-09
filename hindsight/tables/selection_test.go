package tables_test

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/hindsight/tables/tablestest"
	"testing"
)

func selectionCatalog(t *testing.T) *tables.Catalog {
	t.Helper()
	catalog := tablestest.New(t)
	writer := tables.NewWriter(catalog)
	for _, run := range []string{"selected", "other"} {
		for sequence := int64(1); sequence <= 2; sequence++ {
			writer.AddCapture(tables.CaptureRow{Run: run, Sequence: sequence, Kind: "ticker", Payload: []byte(run)})
			for ordinal := int64(0); ordinal <= 1; ordinal++ {
				reference := tables.EnvelopeRefRow{Run: run, Sequence: sequence, Ordinal: ordinal}
				writer.AddManifest(tables.ManifestRow{Run: run, Envelope: reference, Workload: "ticker"})
				for _, kind := range []string{"state", "decision"} {
					writer.AddWitness(tables.WitnessRow{Run: run, Envelope: reference, ArtifactKind: kind, Boundary: "observe", Payload: []byte{byte(sequence), byte(ordinal)}})
				}
			}
		}
	}
	if err := writer.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestCatalogCapture(t *testing.T) {
	Convey("Exact capture reads retain the selected run and sequence", t, func() {
		catalog := selectionCatalog(t)
		row, found, err := catalog.Capture(t.Context(), "selected", 2)
		So(err, ShouldBeNil)
		So(found, ShouldBeTrue)
		So(row.Sequence, ShouldEqual, 2)
		So(string(row.Payload), ShouldEqual, "selected")
		_, found, err = catalog.Capture(t.Context(), "selected", 3)
		So(err, ShouldBeNil)
		So(found, ShouldBeFalse)
	})
}

func TestCatalogWitnessesAt(t *testing.T) {
	Convey("Witness selection combines run, capture, artifact family and ordinal", t, func() {
		catalog := selectionCatalog(t)
		ordinal := int64(1)
		rows, err := catalog.WitnessesAt(t.Context(), "selected", "state", 2, &ordinal)
		So(err, ShouldBeNil)
		So(len(rows), ShouldEqual, 1)
		So(rows[0].Envelope, ShouldResemble, tables.EnvelopeRefRow{Run: "selected", Sequence: 2, Ordinal: 1})
		So(rows[0].Payload, ShouldResemble, []byte{2, 1})
		rows, err = catalog.WitnessesAt(t.Context(), "selected", "", 2, nil)
		So(err, ShouldBeNil)
		So(len(rows), ShouldEqual, 4)
		ordinal = 3
		rows, err = catalog.WitnessesAt(t.Context(), "selected", "state", 2, &ordinal)
		So(err, ShouldBeNil)
		So(rows, ShouldBeEmpty)
	})
}

func TestCatalogManifestsAt(t *testing.T) {
	Convey("Manifest selection retains all ordinals for only the selected capture", t, func() {
		catalog := selectionCatalog(t)
		rows, err := catalog.ManifestsAt(t.Context(), "selected", 2)
		So(err, ShouldBeNil)
		So(len(rows), ShouldEqual, 2)
		for _, row := range rows {
			So(row.Run, ShouldEqual, "selected")
			So(row.Envelope.Sequence, ShouldEqual, 2)
		}
	})
}

func TestCatalogWitnessesUnseen(t *testing.T) {
	Convey("Exact seen identities retain older late-arriving witnesses", t, func() {
		catalog := selectionCatalog(t)
		seen := map[tables.EnvelopeRefRow]bool{{Run: "selected", Sequence: 2, Ordinal: 1}: true}
		rows, err := catalog.WitnessesUnseen(t.Context(), "selected", "state", seen)
		So(err, ShouldBeNil)
		So(len(rows), ShouldEqual, 3)

		for _, row := range rows {
			So(seen[row.Envelope], ShouldBeFalse)
		}
	})
}

func TestCatalogCaptureReferences(t *testing.T) {
	Convey("Batched identity joins exclude other runs and never copy payload bytes", t, func() {
		catalog := selectionCatalog(t)
		rows, err := catalog.CaptureReferences(t.Context(), "selected", []int64{1, 2, 7})
		So(err, ShouldBeNil)
		So(len(rows), ShouldEqual, 2)
		So(rows[1].Run, ShouldEqual, "selected")
		So(rows[1].Payload, ShouldBeNil)
		So(rows[2].Sequence, ShouldEqual, 2)
	})
}
