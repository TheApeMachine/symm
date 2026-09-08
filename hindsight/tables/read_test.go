package tables_test

import (
	"testing"

	"github.com/apache/arrow-go/v18/arrow/compute"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/hindsight/tables/tablestest"
)

func TestCatalogCaptures(t *testing.T) {
	Convey("Given captures appended out of sequence order", t, func() {
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)
		received := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

		for _, sequence := range []int64{3, 1, 2} {
			writer.AddCapture(tables.CaptureRow{
				Run:            "run-a",
				Sequence:       sequence,
				Stream:         "spot-public",
				StreamEpoch:    1,
				StreamSequence: sequence,
				ReceivedAt:     received.Add(time.Duration(sequence) * time.Second),
				Endpoint:       "wss://spot",
				Kind:           "trade",
				PayloadHash:    "hash",
				Payload:        []byte(`{"n":` + string(rune('0'+sequence)) + `}`),
			})
		}

		So(writer.Commit(t.Context()), ShouldBeNil)

		Convey("They read back in capture sequence order", func() {
			rows, err := catalog.Captures(t.Context(), "run-a", 0)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 3)
			So(rows[0].Sequence, ShouldEqual, 1)
			So(rows[1].Sequence, ShouldEqual, 2)
			So(rows[2].Sequence, ShouldEqual, 3)
		})

		Convey("Every column survives the round trip", func() {
			rows, err := catalog.Captures(t.Context(), "run-a", 0)

			So(err, ShouldBeNil)
			So(rows[0].Run, ShouldEqual, "run-a")
			So(rows[0].Stream, ShouldEqual, "spot-public")
			So(rows[0].StreamEpoch, ShouldEqual, 1)
			So(rows[0].Kind, ShouldEqual, "trade")
			So(rows[0].PayloadHash, ShouldEqual, "hash")
			So(string(rows[0].Payload), ShouldEqual, `{"n":1}`)
			So(rows[0].ReceivedAt.Equal(received.Add(time.Second)), ShouldBeTrue)
		})

		Convey("A different run is pruned out", func() {
			rows, err := catalog.Captures(t.Context(), "run-b", 0)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 0)
		})

		Convey("Successive reads release Arrow storage and retain owned payloads", func() {
			allocator := memory.NewCheckedAllocator(memory.DefaultAllocator)
			ctx := compute.WithAllocator(t.Context(), allocator)
			rows, err := catalog.Captures(ctx, "run-a", 0)

			So(err, ShouldBeNil)
			allocator.AssertSize(t, 0)

			writer.AddCapture(tables.CaptureRow{
				Run: "run-b", Sequence: 4, Kind: "trade", Payload: []byte("other run"),
			})
			writer.AddCapture(tables.CaptureRow{
				Run: "run-a", Sequence: 4, Kind: "trade", Payload: []byte("new bytes"),
			})
			So(writer.Commit(t.Context()), ShouldBeNil)
			tail, err := catalog.Captures(ctx, "run-a", 3)

			So(err, ShouldBeNil)
			So(len(tail), ShouldEqual, 1)
			So(tail[0].Run, ShouldEqual, "run-a")
			So(string(tail[0].Payload), ShouldEqual, "new bytes")
			So(string(rows[0].Payload), ShouldEqual, `{"n":1}`)
			allocator.AssertSize(t, 0)

			empty, err := catalog.Captures(ctx, "run-a", 4)

			So(err, ShouldBeNil)
			So(len(empty), ShouldEqual, 0)
			allocator.AssertSize(t, 0)
		})

		Convey("An incremental read returns only what is newer", func() {
			rows, err := catalog.Captures(t.Context(), "run-a", 1)

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 2)
			So(rows[0].Sequence, ShouldEqual, 2)
		})
	})
}

func TestCatalogOutcomes(t *testing.T) {
	Convey("Given a graded decision carrying venue decimals", t, func() {
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)

		// A price with more fractional digits than a float64 can hold exactly,
		// which is the whole reason these columns are decimal and not double.
		price, err := decimal.NewFromString("12345.678901234567")

		So(err, ShouldBeNil)
		outcome := 0.25

		writer.AddOutcome(tables.OutcomeRow{
			Run: "run-a", DecisionID: 7, Trader: 1, Label: "BTC/USD",
			At:         time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC),
			ActionKind: "enter", ActionPower: 3, ActionReduce: false,
			Authority: 0.5, Outcome: &outcome, Context: []int64{11, 12},
			Value: 1.5, Complete: true, Forced: false,
			Reference: price,
		})

		So(writer.Commit(t.Context()), ShouldBeNil)

		Convey("The decimal is stored without a float round trip", func() {
			rows, err := catalog.Outcomes(t.Context(), "run-a")

			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 1)

			// The column has a fixed scale, so the value comes back padded to
			// it. That is a change of representation, not of value: every
			// significant digit survives, which a float64 column would not
			// have managed.
			So(rows[0].Reference.Cmp(price), ShouldEqual, 0)
			So(rows[0].Reference.String(), ShouldStartWith, "12345.678901234567")
			So(rows[0].DecisionID, ShouldEqual, 7)
			So(rows[0].Context, ShouldResemble, []int64{11, 12})
			So(*rows[0].Outcome, ShouldEqual, 0.25)
		})
	})
}

func TestCatalogWitnesses(t *testing.T) {
	Convey("Given witnesses of mixed artifact kind", t, func() {
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)

		for _, kind := range []string{"state", "observe", "state"} {
			writer.AddWitness(tables.WitnessRow{
				Run:              "run-a",
				Envelope:         tables.EnvelopeRefRow{Run: "run-a", Sequence: 1, Ordinal: 0},
				Boundary:         "workspace",
				ArtifactKind:     kind,
				ArtifactIdentity: kind + "-1",
				ImmediateParents: []tables.EnvelopeRefRow{{Run: "run-a", Sequence: 0, Ordinal: 0}},
				SemanticParents:  []string{"cvd"},
				Payload:          []byte("bytes"),
			})
		}

		So(writer.Commit(t.Context()), ShouldBeNil)

		Convey("Resident state selects by predicate, not by a separate table", func() {
			writer.AddWitness(tables.WitnessRow{
				Run: "run-b", ArtifactKind: "state", Payload: []byte("another run"),
			})
			So(writer.Commit(t.Context()), ShouldBeNil)

			states, err := catalog.Witnesses(t.Context(), "run-a", "state")

			So(err, ShouldBeNil)
			So(len(states), ShouldEqual, 2)
			So(states[0].SemanticParents, ShouldResemble, []string{"cvd"})
			So(len(states[0].ImmediateParents), ShouldEqual, 1)
		})

		Convey("An empty kind returns every witness", func() {
			all, err := catalog.Witnesses(t.Context(), "run-a", "")

			So(err, ShouldBeNil)
			So(len(all), ShouldEqual, 3)
		})
	})
}

func BenchmarkCatalogCaptures(b *testing.B) {
	// Two committed files model a consumed archive prefix and newly landed
	// frames. The fixture sizes describe storage shapes, not market windows.
	catalog := tablestest.New(b)
	writer := tables.NewWriter(catalog)

	for sequence := int64(1); sequence <= 4096; sequence++ {
		writer.AddCapture(tables.CaptureRow{
			Run: "bench", Sequence: sequence, Kind: "trade",
			Payload: []byte(`{"channel":"trade","data":[{"symbol":"BTC/USD","price":50000,"qty":0.01}]}`),
		})
	}

	if err := writer.Commit(b.Context()); err != nil {
		b.Fatal(err)
	}

	for sequence := int64(4097); sequence <= 4112; sequence++ {
		writer.AddCapture(tables.CaptureRow{
			Run: "bench", Sequence: sequence, Kind: "trade", Payload: []byte(`{"data":[]}`),
		})
	}

	if err := writer.Commit(b.Context()); err != nil {
		b.Fatal(err)
	}

	for _, fixture := range []struct {
		name  string
		after int64
		rows  int
	}{
		{"whole_run", 0, 4112},
		{"new_suffix", 4096, 16},
		{"caught_up", 4112, 0},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				rows, err := catalog.Captures(b.Context(), "bench", fixture.after)

				if err != nil {
					b.Fatal(err)
				}

				if len(rows) != fixture.rows {
					b.Fatalf("got %d rows, want %d", len(rows), fixture.rows)
				}
			}
		})
	}
}
