package store

import (
	"context"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
)

func TestAsyncWitnessWriter(t *testing.T) {
	Convey("Given an async witness writer over a real SQLite repository", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		engine, err := NewSQLite(t.TempDir() + "/witnesses.sqlite")
		So(err, ShouldBeNil)

		runID, err := hindsight.NewRunID(time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC))
		So(err, ShouldBeNil)
		So(engine.WriteRun(hindsight.Run{
			ID: runID, StartedAt: time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC),
		}), ShouldBeNil)

		sequencer, err := hindsight.NewSequencer(runID)
		So(err, ShouldBeNil)

		capture, err := sequencer.Assign(hindsight.Stream("wss://example:test"))
		So(err, ShouldBeNil)
		So(capture.Valid(), ShouldBeTrue)

		writer := NewAsyncWitnessWriter(ctx, engine, 64, 10*time.Millisecond)

		Reset(func() {
			cancel()
			_ = writer.Close()
			_ = engine.Close()
		})

		Convey("enqueued witnesses are persisted by the background worker", func() {
			ref := hindsight.EnvelopeRef{Origin: capture, Ordinal: 0}

			writer.Enqueue(hindsight.ArtifactWitness{
				Envelope: ref,
				Boundary: "observe",
				Artifact: hindsight.ArtifactID{
					Kind:     "state",
					Identity: "batch-1",
				},
				Payload: []byte("frame-state"),
			})

			// Drain: close the writer, which flushes the queue before
			// returning, then read the witness back through the engine.
			cancel()
			So(writer.Close(), ShouldBeNil)

			witness, err := engine.ReadWitness(capture, "batch-1")
			So(err, ShouldBeNil)
			So(witness.Artifact.Kind, ShouldEqual, "state")
			So(witness.Envelope, ShouldResemble, ref)
		})

		Convey("a burst is committed through the batched transactional path", func() {
			const count = 32

			for index := 0; index < count; index++ {
				writer.Enqueue(hindsight.ArtifactWitness{
					Envelope: hindsight.EnvelopeRef{Origin: capture, Ordinal: uint64(index)},
					Boundary: "after-signals",
					Artifact: hindsight.ArtifactID{
						Kind:     "measurement",
						Identity: "burst-" + strconv.Itoa(index),
					},
				})
			}

			cancel()
			So(writer.Close(), ShouldBeNil)

			for index := 0; index < count; index++ {
				witness, readErr := engine.ReadWitness(capture, "burst-"+strconv.Itoa(index))

				So(readErr, ShouldBeNil)
				So(witness.Artifact.Kind, ShouldEqual, "measurement")
			}
		})

		Convey("a full queue drops rather than blocking the producer", func() {
			tiny := NewAsyncWitnessWriter(ctx, engine, 1, time.Hour)

			Reset(func() {
				_ = tiny.Close()
			})

			for index := 0; index < 8; index++ {
				tiny.Enqueue(hindsight.ArtifactWitness{
					Envelope: hindsight.EnvelopeRef{Origin: capture, Ordinal: uint64(index)},
					Boundary: "observe",
					Artifact: hindsight.ArtifactID{
						Kind:     "state",
						Identity: "overflow",
					},
				})
			}

			So(tiny.Dropped(), ShouldBeGreaterThan, 0)
			So(tiny.Close(), ShouldBeNil)

			gaps, err := engine.ListGaps(string(runID))
			So(err, ShouldBeNil)
			So(gaps, ShouldNotBeEmpty)
			So(gaps[0].Encoding, ShouldEqual, "witness_queue_overflow")
		})
	})
}
