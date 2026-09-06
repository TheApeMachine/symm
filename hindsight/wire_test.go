package hindsight

import (
	"encoding/json"
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
			var decoded EnvelopeRef
			err := json.Unmarshal([]byte(encoded), &decoded)
			So(err, ShouldBeNil)
			So(decoded, ShouldResemble, ref)
		})

	})
}
