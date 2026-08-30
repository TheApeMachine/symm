package hindsight

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func testRun() Run {
	return Run{ID: "run-1"}
}

func testFrame(sequencer *Sequencer, stream Stream, payload []byte) RawFrame {
	identity, _ := sequencer.Assign(stream)

	return RawFrame{
		Identity:   identity,
		ReceivedAt: time.Now().UTC(),
		Endpoint:   "wss://example",
		Kind:       "ticker",
		Payload:    payload,
	}
}

func TestTapeWriteFrameTest(t *testing.T) {
	Convey("Given a Tape for a Run", t, func() {
		tape, err := NewTape(testRun())
		So(err, ShouldBeNil)
		sequencer, _ := NewSequencer("run-1")

		Convey("A frame written under an identity is retrievable by that exact identity", func() {
			identity, _ := sequencer.Assign("spot.public")
			frame := RawFrame{
				Identity: identity,
				Endpoint: "wss://example",
				Kind:     "ticker",
				Payload:  []byte(`{"a":1}`),
			}

			err := tape.WriteFrame(frame)
			So(err, ShouldBeNil)

			stored, ok := tape.Frame(identity)
			So(ok, ShouldBeTrue)
			So(stored.Identity, ShouldResemble, identity)
			So(stored.PayloadHash, ShouldEqual, hashPayload(frame.Payload))
		})

		Convey("A frame with an invalid identity is rejected", func() {
			err := tape.WriteFrame(RawFrame{Payload: []byte(`x`)})
			So(err, ShouldNotBeNil)
		})

		Convey("A duplicate identity is rejected rather than overwritten", func() {
			identity, _ := sequencer.Assign("spot.public")
			frame := RawFrame{Identity: identity, Payload: []byte(`x`)}

			So(tape.WriteFrame(frame), ShouldBeNil)
			So(tape.WriteFrame(frame), ShouldNotBeNil)
		})
	})

	Convey("Given a blank Run ID", t, func() {
		Convey("NewTape rejects it", func() {
			tape, err := NewTape(Run{})
			So(tape, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestTapeFramesOrderTest(t *testing.T) {
	Convey("Given frames captured out of venue order", t, func() {
		tape, _ := NewTape(testRun())
		sequencer, _ := NewSequencer("run-1")

		first := testFrame(sequencer, "spot.public", []byte(`first`))
		second := testFrame(sequencer, "spot.public", []byte(`second`))

		So(tape.WriteFrame(first), ShouldBeNil)
		So(tape.WriteFrame(second), ShouldBeNil)

		Convey("Frames returns them in capture-sequence order, not receive order", func() {
			frames := tape.Frames()

			So(len(frames), ShouldEqual, 2)
			So(frames[0].Identity.Sequence, ShouldBeLessThan, frames[1].Identity.Sequence)
		})
	})
}

func TestTapeManifestFanOutTest(t *testing.T) {
	Convey("Given one raw frame producing several envelopes", t, func() {
		tape, _ := NewTape(testRun())
		sequencer, _ := NewSequencer("run-1")

		identity, _ := sequencer.Assign("spot.trade")
		So(tape.WriteFrame(RawFrame{Identity: identity, Payload: []byte(`t`)}), ShouldBeNil)

		refOne := EnvelopeRef{Origin: identity, Ordinal: 0}
		refTwo := EnvelopeRef{Origin: identity, Ordinal: 1}

		So(tape.WriteManifest(EnvelopeManifest{Envelope: refOne, Workload: "trade"}), ShouldBeNil)
		So(tape.WriteManifest(EnvelopeManifest{Envelope: refTwo, Workload: "trade"}), ShouldBeNil)

		Convey("ManifestsFor returns both envelopes in deterministic ordinal order", func() {
			refs := tape.ManifestsFor(identity)

			So(len(refs), ShouldEqual, 2)
			So(refs[0].Ordinal, ShouldEqual, uint64(0))
			So(refs[1].Ordinal, ShouldEqual, uint64(1))
			So(refs[0].Origin, ShouldResemble, identity)
			So(refs[1].Origin, ShouldResemble, identity)
		})
	})
}

func TestVerifyCompleteTest(t *testing.T) {
	Convey("Given a fully populated, gap-free tape", t, func() {
		tape, _ := NewTape(testRun())
		sequencer, _ := NewSequencer("run-1")

		identity, _ := sequencer.Assign("spot.public")
		So(tape.WriteFrame(RawFrame{Identity: identity, Payload: []byte(`x`)}), ShouldBeNil)
		So(tape.WriteManifest(EnvelopeManifest{Envelope: EnvelopeRef{Origin: identity}}), ShouldBeNil)

		Convey("Verify reports COMPLETE", func() {
			integrity, gaps := NewVerifier().Verify(tape)

			So(integrity, ShouldEqual, IntegrityComplete)
			So(gaps, ShouldBeEmpty)
		})
	})
}

func TestVerifyCaptureGapTest(t *testing.T) {
	Convey("Given a run with a missing CaptureSequence", t, func() {
		tape, _ := NewTape(testRun())
		sequencer, _ := NewSequencer("run-1")

		identity, _ := sequencer.Assign("spot.public")
		So(tape.WriteFrame(RawFrame{Identity: identity, Payload: []byte(`x`)}), ShouldBeNil)

		// Skip the next capture sequence by removing one frame from the run.
		// Simulate by writing a frame two sequences ahead directly.
		gapFrame := RawFrame{
			Identity: CaptureIdentity{
				Run:            "run-1",
				Sequence:       identity.Sequence + 2,
				Stream:         "spot.public",
				StreamEpoch:    1,
				StreamSequence: 1,
			},
			Payload: []byte(`y`),
		}
		So(tape.WriteFrame(gapFrame), ShouldBeNil)

		Convey("Verify reports GAPPED with a missing-sequence gap", func() {
			integrity, gaps := NewVerifier().Verify(tape)

			So(integrity, ShouldEqual, IntegrityGapped)
			So(gaps, ShouldNotBeEmpty)
			So(gaps[0].Encoding, ShouldEqual, GapMissingSequence)
		})
	})
}

func TestVerifyDanglingEnvelopeTest(t *testing.T) {
	Convey("Given an envelope manifest whose origin has no raw frame", t, func() {
		tape, _ := NewTape(testRun())
		sequencer, _ := NewSequencer("run-1")

		identity, _ := sequencer.Assign("spot.public")

		// Manifest without a corresponding WriteFrame.
		So(tape.WriteManifest(EnvelopeManifest{Envelope: EnvelopeRef{Origin: identity}}), ShouldBeNil)

		Convey("Verify reports GAPPED with a dangling-envelope gap", func() {
			integrity, gaps := NewVerifier().Verify(tape)

			So(integrity, ShouldEqual, IntegrityGapped)
			So(gaps, ShouldNotBeEmpty)
			So(gaps[0].Encoding, ShouldEqual, GapDanglingEnvelope)
		})
	})
}

func TestVerifyDanglingWitnessTest(t *testing.T) {
	Convey("Given a witness whose envelope has no manifest", t, func() {
		tape, _ := NewTape(testRun())
		sequencer, _ := NewSequencer("run-1")

		identity, _ := sequencer.Assign("spot.public")
		So(tape.WriteFrame(RawFrame{Identity: identity, Payload: []byte(`x`)}), ShouldBeNil)

		ref := EnvelopeRef{Origin: identity, Ordinal: 0}

		// Witness without a corresponding manifest.
		So(tape.WriteWitness(ArtifactWitness{
			Envelope: ref,
			Artifact: ArtifactID{Kind: "measurement", Identity: "m1"},
		}), ShouldBeNil)

		Convey("Verify reports GAPPED with a dangling-witness gap", func() {
			integrity, gaps := NewVerifier().Verify(tape)

			So(integrity, ShouldEqual, IntegrityGapped)
			So(gaps, ShouldNotBeEmpty)
			So(gaps[0].Encoding, ShouldEqual, GapDanglingWitness)
		})
	})
}

func TestExactProvenanceTest(t *testing.T) {
	Convey("Given a Measurement produced from one envelope", t, func() {
		tape, _ := NewTape(testRun())
		sequencer, _ := NewSequencer("run-1")

		identity, _ := sequencer.Assign("spot.trade")
		So(tape.WriteFrame(RawFrame{Identity: identity, Payload: []byte(`t`)}), ShouldBeNil)

		envelope := EnvelopeRef{Origin: identity, Ordinal: 0}
		So(tape.WriteManifest(EnvelopeManifest{Envelope: envelope, Workload: "trade"}), ShouldBeNil)

		artifact := ArtifactID{Kind: "measurement", Identity: "cvd-1"}

		So(tape.WriteWitness(ArtifactWitness{
			Envelope:         envelope,
			Boundary:         "after-signals",
			Artifact:         artifact,
			ImmediateParents: []EnvelopeRef{envelope},
		}), ShouldBeNil)

		Convey("the chain artifact -> envelope -> raw frame is exact, with no timestamp search", func() {
			witness, ok := tape.Artifact(artifact)
			So(ok, ShouldBeTrue)

			refs := tape.ManifestsFor(witness.Envelope.Origin)
			So(len(refs), ShouldEqual, 1)
			So(refs[0], ShouldResemble, envelope)

			frame, ok := tape.Frame(envelope.Origin)
			So(ok, ShouldBeTrue)
			So(frame.Identity, ShouldResemble, identity)
		})
	})
}

func TestFutureLeakageTest(t *testing.T) {
	Convey("Given a snapshot built only from state causally available at an anchor", t, func() {
		tape, _ := NewTape(testRun())
		sequencer, _ := NewSequencer("run-1")

		anchor, _ := sequencer.Assign("spot.public")
		So(tape.WriteFrame(RawFrame{Identity: anchor, Payload: []byte(`a`)}), ShouldBeNil)

		first := tape.Frames()

		Convey("Appending post-anchor inputs does not change the frames already captured", func() {
			later, _ := sequencer.Assign("spot.public")
			So(tape.WriteFrame(RawFrame{Identity: later, Payload: []byte(`b`)}), ShouldBeNil)

			again := tape.Frames()

			So(len(again), ShouldBeGreaterThan, len(first))
			So(again[0].Identity, ShouldResemble, anchor)
			So(again[0].Payload, ShouldResemble, []byte(`a`))
		})
	})
}

func TestVerifyHashMismatchTest(t *testing.T) {
	Convey("Given a tape hydrated with a stored hash that disagrees with its bytes", t, func() {
		tape, _ := NewTape(testRun())
		sequencer, _ := NewSequencer("run-1")

		identity, _ := sequencer.Assign("spot.public")
		frame := RawFrame{
			Identity: identity,
			Payload:  []byte(`payload`),
		}

		So(tape.WriteFrame(frame), ShouldBeNil)

		// Mutation-kill (§65.18): tamper with the stored hash directly, the
		// way a backend read whose hash column disagrees with its payload
		// column would present to the verifier.
		tampered := tape.frames[identity]
		tampered.PayloadHash = "0000000000000000000000000000000000000000000000000000000000000000"
		tape.frames[identity] = tampered

		Convey("Verify reports CORRUPT with a hash-mismatch gap", func() {
			integrity, gaps := NewVerifier().Verify(tape)

			So(integrity, ShouldEqual, IntegrityCorrupt)
			So(gaps, ShouldNotBeEmpty)
			So(gaps[0].Encoding, ShouldEqual, GapHashMismatch)
		})
	})
}

func TestVerifyBrokenEpochTest(t *testing.T) {
	Convey("Given a stream whose epoch regresses", t, func() {
		tape, _ := NewTape(testRun())

		first := RawFrame{
			Identity: CaptureIdentity{
				Run:            "run-1",
				Sequence:       1,
				Stream:         "spot.public",
				StreamEpoch:    2,
				StreamSequence: 1,
			},
			Payload: []byte(`a`),
		}

		regressed := RawFrame{
			Identity: CaptureIdentity{
				Run:            "run-1",
				Sequence:       2,
				Stream:         "spot.public",
				StreamEpoch:    1,
				StreamSequence: 2,
			},
			Payload: []byte(`b`),
		}

		So(tape.WriteFrame(first), ShouldBeNil)
		So(tape.WriteFrame(regressed), ShouldBeNil)

		Convey("Verify reports CORRUPT with a broken-epoch gap", func() {
			integrity, gaps := NewVerifier().Verify(tape)

			So(integrity, ShouldEqual, IntegrityCorrupt)
			So(gaps, ShouldNotBeEmpty)
			So(gaps[0].Encoding, ShouldEqual, GapBrokenEpoch)
		})
	})
}

func TestWriteManifestRejectsDuplicateEnvelopeTest(t *testing.T) {
	Convey("Given two envelopes claiming the same origin and ordinal", t, func() {
		tape, _ := NewTape(testRun())
		sequencer, _ := NewSequencer("run-1")

		identity, _ := sequencer.Assign("spot.trade")
		So(tape.WriteFrame(RawFrame{Identity: identity, Payload: []byte(`t`)}), ShouldBeNil)

		ref := EnvelopeRef{Origin: identity, Ordinal: 0}

		Convey("The second manifest is rejected rather than silently collapsing identities", func() {
			So(tape.WriteManifest(EnvelopeManifest{Envelope: ref}), ShouldBeNil)
			So(tape.WriteManifest(EnvelopeManifest{Envelope: ref}), ShouldNotBeNil)
		})
	})
}
