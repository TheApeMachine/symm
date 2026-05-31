package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBookSequenceAccepts(t *testing.T) {
	Convey("Given a fresh book sequence", t, func() {
		sequence := BookSequence{}
		delta := BookUpdate{Symbol: "BTC/EUR"}

		Convey("It should reject deltas before the first snapshot", func() {
			So(sequence.Accepts(delta), ShouldBeFalse)
		})

		Convey("It should accept a snapshot and subsequent deltas", func() {
			snapshot := BookUpdate{Symbol: "BTC/EUR"}
			snapshot.SetEnvelopeType(BookSnapshot)

			So(sequence.Accepts(snapshot), ShouldBeTrue)
			So(sequence.Accepts(delta), ShouldBeTrue)
		})
	})
}
