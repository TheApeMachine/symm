package recording

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/hindsight/tables/tablestest"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/nomagique/data"
	"golang.design/x/lockfree/lf"
)

/*
TestSessionCapture exercises the real production capture →
envelope → semantic → witness → persisted-record chain. One raw Kraken trade
frame yielding two trades is captured through the real Archive store + sequencer
+ writer, parsed by the real ingest path into two envelopes with deterministic
ordinals, advanced through the real category solver, and its artifacts witnessed
and persisted. The test then answers, from persisted identities alone: what
exact exchange bytes, parsed envelope, and processing transition caused a
resulting semantic artifact.
*/
func TestSessionCapture(t *testing.T) {
	Convey("Given the real capture and semantic stack", t, func() {

		runID, err := hindsight.NewRunID(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
		So(err, ShouldBeNil)
		writer, engine := newSessionFixture(t, runID)

		Convey("One raw frame yielding two trades is captured, ingested, witnessed, and traversable by identity", func() {
			raw := []byte(`{"channel":"trade","type":"update","data":[
				{"symbol":"XBT/USD","side":"buy","price":"34000.5","qty":0.1,"ord_type":"market","trade_id":1,"timestamp":"2026-01-02T03:04:05Z"},
				{"symbol":"XBT/USD","side":"sell","price":"34001.0","qty":0.2,"ord_type":"market","trade_id":2,"timestamp":"2026-01-02T03:04:05Z"}
			]}`)

			// 1. Capture the raw frame: mint + persist identity.
			captureID, err := writer.Capture("trade", "wss://example", raw, time.Now(), hindsight.StreamRef{
				Stream:   hindsight.Stream("wss://example:test"),
				Epoch:    1,
				Sequence: 1,
			})
			So(err, ShouldBeNil)
			So(captureID.Valid(), ShouldBeTrue)

			// 2. Parse via the production ingest path: two envelopes, one origin.
			parsed := kraken.NewTrade(raw)
			envelopes, manifests := websocket.IngestEnvelopes("trade", parsed, captureID)

			So(len(envelopes), ShouldEqual, 2)
			So(len(manifests), ShouldEqual, 2)
			So(envelopes[0].CaptureID, ShouldResemble, captureID)
			So(envelopes[1].CaptureID, ShouldResemble, captureID)
			So(envelopes[0].CaptureOrdinal, ShouldEqual, uint64(0))
			So(envelopes[1].CaptureOrdinal, ShouldEqual, uint64(1))

			// 3. Persist the envelope manifests (the live ingress does this too).
			for _, manifest := range manifests {
				So(writer.WriteManifest(manifest), ShouldBeNil)
			}

			// 4. Advance one envelope through the real category solver to produce
			// a semantic artifact (a Measurement is the category solver's input).
			solver := category.NewSolver(context.Background())

			measurement := data.NewMeasurement[float64](
				"cvd-1", "XBT/USD", "cvd",
				time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC),
			)
			measurement.PutMetric(data.Metric[float64]{
				Label: "signed_net_fraction_zscore",
				Raw:   1.5,
			})

			envelopes[0].CVD = measurement
			solver.Step(envelopes[0])

			// 5. Witness the semantic artifact (the Measurement) on the first
			// envelope, exactly as the live witnessNode records it.
			ref := hindsight.EnvelopeRef{Origin: captureID, Ordinal: 0}

			writer.record(hindsight.ArtifactWitness{
				Envelope:         ref,
				Boundary:         "after-signals",
				Artifact:         hindsight.ArtifactID{Kind: "measurement", Identity: "cvd-1"},
				ImmediateParents: []hindsight.EnvelopeRef{ref},
			})
			So(writer.Close(), ShouldBeNil)

			// 6. From persisted identities alone, traverse back to the exact
			// exchange bytes. The raw frame is stored alongside its identity.
			captures, err := engine.Captures(context.Background(), string(captureID.Run))
			So(err, ShouldBeNil)
			So(captures, ShouldHaveLength, 1)
			So(captures[0].Sequence, ShouldEqual, int64(captureID.Sequence))
			So(string(captures[0].Payload), ShouldEqual, string(raw))

			// The witness is persisted, keyed by the same origin so a consumer
			// can walk witness → EnvelopeRef → raw frame without any timestamp.
			witnesses, err := engine.Witnesses(context.Background(), string(runID), "")
			So(err, ShouldBeNil)
			So(witnesses, ShouldHaveLength, 1)

			witness := witnesses[0]
			So(witness.ArtifactKind, ShouldEqual, "measurement")
			So(witness.ArtifactIdentity, ShouldEqual, "cvd-1")
			So(witness.Envelope.Run, ShouldEqual, string(captureID.Run))
			So(witness.Envelope.Sequence, ShouldEqual, int64(captureID.Sequence))
			So(witness.Envelope.Ordinal, ShouldEqual, int64(0))
			So(witness.ImmediateParents, ShouldHaveLength, 1)
			So(witness.ImmediateParents[0].Sequence, ShouldEqual, int64(ref.Origin.Sequence))
		})
	})
}

func newSessionFixture(t *testing.T, run hindsight.RunID) (*Session, *tables.Catalog) {
	t.Helper()
	catalog := tablestest.New(t)
	writer, err := NewSession(context.Background(), tables.NewWriter(catalog), hindsight.Run{ID: run, StartedAt: time.Unix(1, 0)}, 8, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Errorf("close recorder: %v", err)
		}
	})

	return writer, catalog
}

/*
TestSessionPersist asserts that batching loses nothing.

The old fixture counted uploaded objects, because a batch was an object. A
batch is now an Iceberg commit spanning whatever families it touched, so the
property worth asserting is the one that always mattered: every accepted record
reaches the table, exactly once, in the order its producer emitted it.
*/
func TestSessionPersist(t *testing.T) {
	Convey("Given more queued records than one batch holds", t, func() {
		catalog := tablestest.New(t)
		session := &Session{
			writer: tables.NewWriter(catalog),
			run:    hindsight.Run{ID: "persist"},
			queue:  lf.NewQueue[pending](),
			done:   make(chan struct{}),
		}

		for index := range 82 {
			session.queue.Enqueue(pending{family: tables.Captures, row: tables.CaptureRow{
				Run: "persist", Sequence: int64(index + 1), Stream: "spot",
				StreamEpoch: 1, StreamSequence: int64(index + 1),
				ReceivedAt: time.Unix(int64(index), 0), Kind: "trade", PayloadHash: "hash",
			}})
		}

		session.closed = true
		session.persist(context.Background(), 8, time.Millisecond)

		Convey("Every record is persisted exactly once and in order", func() {
			rows, err := catalog.Captures(context.Background(), "persist")

			So(err, ShouldBeNil)
			So(rows, ShouldHaveLength, 82)

			for index, row := range rows {
				So(row.Sequence, ShouldEqual, int64(index+1))
			}
		})
	})
}

func BenchmarkSessionPersist(b *testing.B) {
	catalog := tablestest.New(&testing.T{})
	b.ReportAllocs()

	for b.Loop() {
		session := &Session{
			writer: tables.NewWriter(catalog),
			run:    hindsight.Run{ID: "bench"},
			queue:  lf.NewQueue[pending](),
			done:   make(chan struct{}),
		}

		for sequence := range 1024 {
			session.queue.Enqueue(pending{family: tables.Captures, row: tables.CaptureRow{
				Run: "bench", Sequence: int64(sequence + 1), Stream: "spot",
				StreamEpoch: 1, StreamSequence: int64(sequence + 1),
				ReceivedAt: time.Unix(int64(sequence), 0), Kind: "trade", PayloadHash: "hash",
				Payload: bytes.Repeat([]byte("x"), 1024),
			}})
		}

		session.closed = true
		session.persist(context.Background(), 256, time.Second)
	}
}

/*
TestSessionWriteOutcome asserts that concurrent producers neither lose records
nor observe each other's, and that a persistence failure latches.
*/
func TestSessionWriteOutcome(t *testing.T) {
	Convey("Given many producers writing at once", t, func() {
		catalog := tablestest.New(t)
		session, err := NewSession(
			context.Background(), tables.NewWriter(catalog),
			hindsight.Run{ID: "burst", StartedAt: time.Unix(1, 0)}, 256, time.Millisecond,
		)

		So(err, ShouldBeNil)

		var producers sync.WaitGroup

		for producer := range 4 {
			producers.Go(func() {
				for sequence := range 256 {
					if err := session.WriteOutcome(tables.OutcomeRow{
						DecisionID: int64(sequence), Trader: int32(producer),
						Label: "BTC/USD", At: time.Unix(int64(sequence), 0),
						ActionKind: "enter", Authority: 1, Value: 1, Complete: true,
					}); err != nil {
						t.Error(err)

						return
					}
				}
			})
		}

		producers.Wait()
		So(session.Close(), ShouldBeNil)

		Convey("Every accepted record is persisted exactly once", func() {
			rows, err := catalog.Outcomes(context.Background(), "burst")

			So(err, ShouldBeNil)
			So(rows, ShouldHaveLength, 4*256)

			seen := map[int32]map[int64]bool{}

			for _, row := range rows {
				if seen[row.Trader] == nil {
					seen[row.Trader] = map[int64]bool{}
				}

				So(seen[row.Trader][row.DecisionID], ShouldBeFalse)
				seen[row.Trader][row.DecisionID] = true
			}

			So(len(seen), ShouldEqual, 4)
		})

		Convey("A closed session accepts nothing further", func() {
			So(session.WriteOutcome(tables.OutcomeRow{}), ShouldNotBeNil)
		})
	})

	Convey("Given a catalog whose tables do not exist", t, func() {
		// Wrapping without Ensure leaves every append with no table to write
		// to, which is the closest reproduction of a persistence failure that
		// does not require faking the storage layer.
		catalog := tablestest.Empty(t)
		session := &Session{
			writer: tables.NewWriter(catalog),
			run:    hindsight.Run{ID: "failure"},
			queue:  lf.NewQueue[pending](),
			wake:   make(chan struct{}, 1),
			done:   make(chan struct{}),
			Errors: make(chan error, 1),
		}

		go session.persist(context.Background(), 1, time.Hour)

		So(session.WriteOutcome(tables.OutcomeRow{Label: "accepted"}), ShouldBeNil)

		Convey("The failure is reported and latches admission", func() {
			So(<-session.Errors, ShouldNotBeNil)

			So(session.WriteOutcome(tables.OutcomeRow{Label: "rejected"}), ShouldNotBeNil)
			So(session.Close(), ShouldNotBeNil)
		})
	})
}
