package ui

import (
	"fmt"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/hindsight/tables/tablestest"
	"testing"
	"time"
)

func TestInspectionWitness(t *testing.T) {
	Convey("Iceberg references resolve to their recorded transport identities", t, func() {
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)
		writer.AddCapture(tables.CaptureRow{Run: "run", Sequence: 7, Stream: "public", StreamEpoch: 2, StreamSequence: 5, Kind: "ticker", Payload: []byte(`{}`)})
		So(writer.Commit(t.Context()), ShouldBeNil)
		reader := newInspection(t.Context(), catalog)
		row := tables.WitnessRow{Envelope: tables.EnvelopeRefRow{Run: "run", Sequence: 7, Ordinal: 1}, ArtifactKind: "state", Payload: []byte{1, 2, 3}}
		result, err := reader.witness(row)
		So(err, ShouldBeNil)
		So(string(result.Envelope.Origin.Stream), ShouldEqual, "public")
		So(result.Envelope.Origin.StreamEpoch, ShouldEqual, 2)
		So(result.Envelope.Ordinal, ShouldEqual, 1)
		So(result.Payload, ShouldResemble, row.Payload)
		Convey("A dangling parent remains an error rather than an invented identity", func() {
			row.ImmediateParents = []tables.EnvelopeRefRow{{Run: "run", Sequence: 8}}
			_, err := reader.witness(row)
			So(err, ShouldNotBeNil)
		})
	})
}

// inspectionArchive populates the real catalog with two ordinals, a parent,
// precise execution economics, a gap and a second run with overlapping IDs.
func inspectionArchive(t testing.TB) *Hub {
	t.Helper()
	catalog := tablestest.New(t)
	writer := tables.NewWriter(catalog)
	at := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	writer.AddRun(tables.RunRow{ID: "run", StartedAt: at, Integrity: "GAPPED", SchemaVersions: map[string]string{"state": "v1"}})
	for _, run := range []string{"run", "other"} {
		for sequence := int64(1); sequence <= 3; sequence++ {
			writer.AddCapture(tables.CaptureRow{Run: run, Sequence: sequence, Stream: "public", StreamEpoch: 2, StreamSequence: sequence + 10, ReceivedAt: at.Add(time.Duration(sequence) * time.Second), Kind: "ticker", Payload: []byte(fmt.Sprintf(`{"data":[{"symbol":"BTC/USD","bid":%d,"ask":%d}]}`, 100+sequence, 102+sequence))})
			writer.AddManifest(tables.ManifestRow{Run: run, Envelope: tables.EnvelopeRefRow{Run: run, Sequence: sequence}, Workload: "ticker", Symbol: "BTC/USD"})
		}
		for ordinal := int64(0); ordinal < 2; ordinal++ {
			writer.AddWitness(tables.WitnessRow{Run: run, Envelope: tables.EnvelopeRefRow{Run: run, Sequence: 2, Ordinal: ordinal}, ArtifactKind: "state", ArtifactIdentity: fmt.Sprint(ordinal), Boundary: "observe", Payload: []byte{byte(ordinal + 1)}, ImmediateParents: []tables.EnvelopeRefRow{{Run: run, Sequence: 1}}})
		}
	}
	price, err := decimal.NewFromString("123.4567890123")
	if err != nil {
		t.Fatal(err)
	}
	fee, err := decimal.NewFromString("0.0123")
	if err != nil {
		t.Fatal(err)
	}
	writer.AddLifecycle(tables.LifecycleRow{Run: "run", DecisionID: "decision", Symbol: "BTC/USD", Kind: "position_open", At: at, CaptureSeq: 2})
	writer.AddLifecycle(tables.LifecycleRow{Run: "run", DecisionID: "decision", Symbol: "BTC/USD", Kind: "entry_fill", At: at.Add(time.Second), CaptureSeq: 2, Exec: &tables.ExecutionRow{OrderID: "order", Side: "buy", OrderStatus: "filled", At: at, AvgPrice: price, FeeUsdEquiv: fee}})
	writer.AddGap(tables.GapRow{Run: "run", Sequence: 3, Encoding: "missing_payload"})
	if err := writer.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	hub := NewHub(t.Context())
	hub.SetHindsightStore(catalog)
	t.Cleanup(func() {
		if err := hub.Close(); err != nil {
			t.Error(err)
		}
	})
	return hub
}
