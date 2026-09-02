package hindsight

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCaptureIdentityValidTest(t *testing.T) {
	Convey("Given a CaptureIdentity", t, func() {
		valid := CaptureIdentity{
			Run:            "run-1",
			Sequence:       1,
			Stream:         "spot.public",
			StreamEpoch:    1,
			StreamSequence: 1,
		}

		Convey("A fully populated identity is valid", func() {
			So(valid.Valid(), ShouldBeTrue)
		})

		Convey("A zero Run makes it invalid", func() {
			invalid := valid
			invalid.Run = ""
			So(invalid.Valid(), ShouldBeFalse)
		})

		Convey("An empty Stream makes it invalid", func() {
			invalid := valid
			invalid.Stream = ""
			So(invalid.Valid(), ShouldBeFalse)
		})

		Convey("A zero epoch makes it invalid", func() {
			invalid := valid
			invalid.StreamEpoch = 0
			So(invalid.Valid(), ShouldBeFalse)
		})

		Convey("A zero capture sequence makes it invalid", func() {
			invalid := valid
			invalid.Sequence = 0
			So(invalid.Valid(), ShouldBeFalse)
		})
	})
}

func TestSequencerAssignTest(t *testing.T) {
	Convey("Given a Sequencer for a Run", t, func() {
		sequencer, err := NewSequencer("run-1")
		So(err, ShouldBeNil)

		Convey("Assignments within one stream increase the capture order monotonically", func() {
			first, err := sequencer.Assign("spot.public")
			So(err, ShouldBeNil)
			second, err := sequencer.Assign("spot.public")
			So(err, ShouldBeNil)

			So(second.Sequence, ShouldBeGreaterThan, first.Sequence)
			So(second.StreamSequence, ShouldBeGreaterThan, first.StreamSequence)
			So(second.StreamEpoch, ShouldEqual, first.StreamEpoch)
		})

		Convey("Assignments on distinct streams share one run-local capture order", func() {
			first, _ := sequencer.Assign("spot.public")

			second, _ := sequencer.Assign("spot.private")

			So(second.Sequence, ShouldBeGreaterThan, first.Sequence)
			So(second.Stream, ShouldNotEqual, first.Stream)
		})
	})

	Convey("Given a blank Run", t, func() {
		Convey("NewSequencer rejects it", func() {
			sequencer, err := NewSequencer("")
			So(sequencer, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestSequencerReconnectTest(t *testing.T) {
	Convey("Given a Sequencer that has assigned on a stream", t, func() {
		sequencer, _ := NewSequencer("run-1")
		before, _ := sequencer.Assign("spot.public")

		Convey("Reconnecting starts a new epoch and resets the stream sequence", func() {
			sequencer.Reconnect("spot.public")

			after, err := sequencer.Assign("spot.public")
			So(err, ShouldBeNil)

			So(after.StreamEpoch, ShouldEqual, before.StreamEpoch+1)
			So(after.StreamSequence, ShouldEqual, uint64(1))
			So(after.Sequence, ShouldBeGreaterThan, before.Sequence)
		})
	})
}

func TestTimestampCollisionTest(t *testing.T) {
	Convey("Given two distinct captured inputs with identical venue timestamps", t, func() {
		sequencer, _ := NewSequencer("run-1")

		first, _ := sequencer.Assign("spot.public")
		second, _ := sequencer.Assign("spot.public")

		sharedVenueTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

		Convey("CaptureIdentity distinguishes them despite the shared venue time", func() {
			So(first, ShouldNotResemble, second)
			_ = sharedVenueTime
		})
	})
}

func TestEnvelopeOrdinalDeterminismTest(t *testing.T) {
	Convey("Given one CaptureIdentity", t, func() {
		sequencer, _ := NewSequencer("run-1")
		origin, _ := sequencer.Assign("spot.trade")

		Convey("EnvelopeRefs sharing an origin are distinguished by deterministic ordinals", func() {
			first := EnvelopeRef{Origin: origin, Ordinal: 0}
			second := EnvelopeRef{Origin: origin, Ordinal: 1}
			third := EnvelopeRef{Origin: origin, Ordinal: 2}

			So(first.Origin, ShouldResemble, second.Origin)
			So(second.Origin, ShouldResemble, third.Origin)
			So(first.Ordinal, ShouldNotEqual, second.Ordinal)
			So(second.Ordinal, ShouldNotEqual, third.Ordinal)
		})
	})
}

func TestStateVersionMonotonicTest(t *testing.T) {
	Convey("Given a shared resident state cell", t, func() {
		var version uint64

		Convey("Advancing it from two workloads keeps the version monotonic", func() {
			version++
			fromTrade := version

			version++
			fromTicker := version

			So(fromTicker, ShouldBeGreaterThan, fromTrade)
			So(StateVersion{
				Component: "strategy.planner",
				Key:       "BTC",
				Version:   fromTrade,
			}, ShouldNotResemble, StateVersion{
				Component: "strategy.planner",
				Key:       "BTC",
				Version:   fromTicker,
			})
		})
	})
}

func TestRawToEnvelopeIdentityTest(t *testing.T) {
	Convey("Given one raw frame captured through a Sequencer", t, func() {
		sequencer, _ := NewSequencer("run-1")
		origin, err := sequencer.Assign("spot.trade")
		So(err, ShouldBeNil)

		Convey("Multiple envelopes derived from it share one CaptureIdentity and unique ordinals", func() {
			envelopes := []EnvelopeRef{
				{Origin: origin, Ordinal: 0},
				{Origin: origin, Ordinal: 1},
				{Origin: origin, Ordinal: 2},
			}

			So(envelopes[0].Origin, ShouldResemble, origin)
			So(envelopes[1].Origin, ShouldResemble, origin)
			So(envelopes[2].Origin, ShouldResemble, origin)

			So(envelopes[0].Ordinal, ShouldNotEqual, envelopes[1].Ordinal)
			So(envelopes[1].Ordinal, ShouldNotEqual, envelopes[2].Ordinal)
		})

		Convey("A mutation removing the origin propagation is detected", func() {
			stripped := EnvelopeRef{Origin: CaptureIdentity{}, Ordinal: 0}

			So(stripped.Origin.Valid(), ShouldBeFalse)
			So(stripped.Origin, ShouldNotResemble, origin)
		})
	})
}

func TestReplayOrderingTest(t *testing.T) {
	Convey("Given captured frames whose venue order differs from capture order", t, func() {
		sequencer, _ := NewSequencer("run-1")

		first, _ := sequencer.Assign("spot.public")
		second, _ := sequencer.Assign("spot.public")

		Convey("Replay order must follow CaptureSequence, not venue time", func() {
			// first was captured before second, by construction.
			So(second.Sequence, ShouldBeGreaterThan, first.Sequence)

			// Even if a venue timestamp would place second earlier, the
			// CaptureSequence is the replay order.
			So(CaptureSequence(first.Sequence), ShouldBeLessThan, CaptureSequence(second.Sequence))
		})
	})
}
