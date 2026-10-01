package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestDetectExcursionsMatchesLiveWalk(t *testing.T) {
	Convey("Offline DetectExcursions equals walking Detector.Observe", t, func() {
		mids := calmThenRise()
		tape := make([]*data.Measurement[float64], 0, len(mids))

		for seq, mid := range mids {
			tape = append(tape, quoteAt("BTC/USD", int64(seq+1), mid))
		}

		live := NewDetector(priced(t, "BTC/USD", 0.1))
		liveRecords := collect(live, "BTC/USD", mids)

		offline := NewDetector(priced(t, "BTC/USD", 0.1))
		got, err := DetectExcursions(offline, tape)
		So(err, ShouldBeNil)
		So(len(got), ShouldEqual, len(liveRecords))

		if len(got) > 0 && len(liveRecords) > 0 {
			So(got[0].ID, ShouldEqual, liveRecords[0].ID)
			So(got[0].AnchorTick, ShouldEqual, liveRecords[0].AnchorTick)
			So(got[0].ExitTick, ShouldEqual, liveRecords[0].ExitTick)
			So(got[0].ClearsFriction, ShouldEqual, liveRecords[0].ClearsFriction)
		}
	})
}

func TestDetectExcursionsSkipsNonQuoteRows(t *testing.T) {
	Convey("Rows without bid/ask are ignored", t, func() {
		bare := data.NewMeasurement[float64]("resonance", nil)
		bare.Label = "BTC/USD"
		bare.SeqIdx = 1
		bare.SetMetric("energy", data.Metric[float64]{Raw: 1})

		got, err := DetectExcursions(NewDetector(priced(t, "BTC/USD", 0.1)), []*data.Measurement[float64]{bare})
		So(err, ShouldBeNil)
		So(len(got), ShouldEqual, 0)
	})
}
