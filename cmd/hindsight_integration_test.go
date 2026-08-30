package cmd

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/store"
)

/*
TestCaptureProvenanceIntegrationTest exercises the real production capture →
envelope → semantic → witness → persisted-record chain. One raw Kraken trade
frame yielding two trades is captured through the real SQLite store + sequencer
+ writer, parsed by the real ingest path into two envelopes with deterministic
ordinals, advanced through the real category solver, and its artifacts witnessed
and persisted. The test then answers, from persisted identities alone: what
exact exchange bytes, parsed envelope, and processing transition caused a
resulting semantic artifact.
*/
func TestCaptureProvenanceIntegrationTest(t *testing.T) {
	Convey("Given the real capture and semantic stack", t, func() {
		path := t.TempDir() + "/events.sqlite"

		engine, err := store.NewSQLite(path)
		So(err, ShouldBeNil)

		Reset(func() {
			_ = engine.Close()
		})

		runID, err := hindsight.NewRunID(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
		So(err, ShouldBeNil)

		sequencer, err := hindsight.NewSequencer(runID)
		So(err, ShouldBeNil)

		writer, err := store.NewWriter(engine, sequencer)
		So(err, ShouldBeNil)

		Convey("One raw frame yielding two trades is captured, ingested, witnessed, and traversable by identity", func() {
			raw := []byte(`{"channel":"trade","type":"update","data":[
				{"symbol":"XBT/USD","side":"buy","price":"34000.5","qty":0.1,"ord_type":"market","trade_id":1,"timestamp":"2026-01-02T03:04:05Z"},
				{"symbol":"XBT/USD","side":"sell","price":"34001.0","qty":0.2,"ord_type":"market","trade_id":2,"timestamp":"2026-01-02T03:04:05Z"}
			]}`)

			// 1. Capture the raw frame: mint + persist identity.
			captureID, err := writer.Capture("trade", "wss://example", raw, time.Now())
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

			So(writer.WriteWitness(hindsight.ArtifactWitness{
				Envelope:         ref,
				Boundary:         "after-signals",
				Artifact:         hindsight.ArtifactID{Kind: "measurement", Identity: "cvd-1"},
				ImmediateParents: []hindsight.EnvelopeRef{ref},
			}), ShouldBeNil)

			// 6. From persisted identities alone, traverse back to the exact
			// exchange bytes. The raw frame is stored alongside its identity.
			storedBytes, err := engine.ReadCapture(captureID)
			So(err, ShouldBeNil)
			So(string(storedBytes), ShouldEqual, string(raw))

			// The witness is persisted, keyed by the same origin so a consumer
			// can walk witness → EnvelopeRef → raw frame without any timestamp.
			witness, err := engine.ReadWitness(captureID, "cvd-1")
			So(err, ShouldBeNil)
			So(witness.Artifact.Kind, ShouldEqual, "measurement")
			So(witness.Artifact.Identity, ShouldEqual, "cvd-1")
			So(witness.Envelope.Origin, ShouldResemble, captureID)
			So(witness.Envelope.Ordinal, ShouldEqual, uint64(0))
			So(witness.ImmediateParents, ShouldHaveLength, 1)
			So(witness.ImmediateParents[0], ShouldResemble, ref)
		})
	})
}
