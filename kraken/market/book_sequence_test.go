package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBookSequenceCanAccept(t *testing.T) {
	Convey("Given a fresh book sequence", t, func() {
		sequence := BookSequence{}
		delta := BookUpdate{Symbol: "BTC/EUR"}

		Convey("It should reject deltas before the first snapshot", func() {
			So(sequence.CanAccept(delta), ShouldBeFalse)
		})

		Convey("It should accept a snapshot and subsequent deltas after AdmitSnapshot", func() {
			snapshot := BookUpdate{Symbol: "BTC/EUR"}
			snapshot.SetEnvelopeType(BookSnapshot)

			So(sequence.CanAccept(snapshot), ShouldBeTrue)
			sequence.AdmitSnapshot()
			So(sequence.CanAccept(delta), ShouldBeTrue)
		})
	})
}
