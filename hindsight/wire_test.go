package hindsight

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewRunIDTest(t *testing.T) {
	Convey("Given process run identity generation", t, func() {
		startedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

		Convey("Two runs in the same instant generate distinct IDs", func() {
			first, err := NewRunID(startedAt)
			So(err, ShouldBeNil)

			second, err := NewRunID(startedAt)
			So(err, ShouldBeNil)

			So(first, ShouldNotEqual, second)
		})

		Convey("A zero start instant is rejected", func() {
			id, err := NewRunID(time.Time{})
			So(id, ShouldEqual, RunID(""))
			So(err, ShouldNotBeNil)
		})
	})
}

func TestIdentityMarshalTest(t *testing.T) {
	Convey("Given a valid CaptureIdentity", t, func() {
		identity := CaptureIdentity{
			Run:            "run-1",
			Sequence:       7,
			Stream:         "spot.public",
			StreamEpoch:    3,
			StreamSequence: 42,
		}

		encoded, err := MarshalIdentity(identity)
		So(err, ShouldBeNil)

		Convey("It survives persistence and retrieval unchanged", func() {
			decoded, err := UnmarshalIdentity(encoded)
			So(err, ShouldBeNil)
			So(decoded, ShouldResemble, identity)
		})

		Convey("An invalid persisted identity fails loudly, not silently to zero", func() {
			_, err := UnmarshalIdentity(`{"Run":"","Sequence":0}`)
			So(err, ShouldNotBeNil)
		})

		Convey("Garbage bytes fail loudly", func() {
			_, err := UnmarshalIdentity(`not-json`)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestEnvelopeRefMarshalTest(t *testing.T) {
	Convey("Given an EnvelopeRef", t, func() {
		ref := EnvelopeRef{
			Origin: CaptureIdentity{
				Run:            "run-1",
				Sequence:       9,
				Stream:         "spot.trade",
				StreamEpoch:    1,
				StreamSequence: 5,
			},
			Ordinal: 2,
		}

		encoded, err := MarshalEnvelopeRef(ref)
		So(err, ShouldBeNil)

		Convey("It round-trips exactly", func() {
			decoded, err := UnmarshalEnvelopeRef(encoded)
			So(err, ShouldBeNil)
			So(decoded, ShouldResemble, ref)
		})

		Convey("An invalid ref fails loudly", func() {
			_, err := UnmarshalEnvelopeRef(`{"Origin":{},"Ordinal":0}`)
			So(err, ShouldNotBeNil)
		})
	})
}
