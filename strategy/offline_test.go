package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/tests/market"
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

func TestReconcileExcursionsSupersedesOverlappingProvisionalFragments(t *testing.T) {
	Convey("Canonical hindsight excursions supersede overlapping provisional live fragments", t, func() {
		// Live detection fragmented an excursion into two premature fragments:
		frag1 := tables.ExcursionRecord{
			ID:                 "live-frag-1",
			Symbol:             "BTC/USD",
			Direction:          "up",
			PrecursorStartTick: 10,
			AnchorTick:         15,
			ExitTick:           25,
			PostEndTick:        28,
		}
		frag2 := tables.ExcursionRecord{
			ID:                 "live-frag-2",
			Symbol:             "BTC/USD",
			Direction:          "up",
			PrecursorStartTick: 29,
			AnchorTick:         35,
			ExitTick:           45,
			PostEndTick:        50,
		}
		// A non-overlapping live record for ETH/USD:
		frag3ETH := tables.ExcursionRecord{
			ID:                 "live-eth",
			Symbol:             "ETH/USD",
			Direction:          "up",
			PrecursorStartTick: 100,
			AnchorTick:         105,
			ExitTick:           120,
			PostEndTick:        125,
		}
		// A non-overlapping live record for BTC/USD much later:
		frag4BTC := tables.ExcursionRecord{
			ID:                 "live-btc-later",
			Symbol:             "BTC/USD",
			Direction:          "down",
			PrecursorStartTick: 200,
			AnchorTick:         205,
			ExitTick:           220,
			PostEndTick:        225,
		}

		provisional := []tables.ExcursionRecord{frag1, frag2, frag3ETH, frag4BTC}

		// Canonical hindsight detection over complete raw tape shows one continuous excursion:
		canonical1 := tables.ExcursionRecord{
			ID:                 "canonical-btc-1",
			Symbol:             "BTC/USD",
			Direction:          "up",
			PrecursorStartTick: 10,
			AnchorTick:         15,
			ExitTick:           45,
			PostEndTick:        55,
		}

		canonical := []tables.ExcursionRecord{canonical1}

		kept, persistIDs := ReconcileExcursions(canonical, provisional)

		// Both frag1 and frag2 overlap canonical1 on [10, 55] -> superseded!
		// They must NEVER teach the model.
		So(len(kept), ShouldEqual, 3)

		keptIDs := make(map[string]bool)
		for _, r := range kept {
			keptIDs[r.ID] = true
		}

		So(keptIDs["canonical-btc-1"], ShouldBeTrue)
		So(keptIDs["live-eth"], ShouldBeTrue)
		So(keptIDs["live-btc-later"], ShouldBeTrue)

		// Verify superseded fragments are gone
		So(keptIDs["live-frag-1"], ShouldBeFalse)
		So(keptIDs["live-frag-2"], ShouldBeFalse)

		// Only canonical1 should be flagged for persistence (not existing durable live-eth or live-btc-later)
		So(persistIDs["canonical-btc-1"], ShouldBeTrue)
		So(persistIDs["live-eth"], ShouldBeFalse)
		So(persistIDs["live-btc-later"], ShouldBeFalse)
	})
}

func TestReplayDensityAndFragmentationRobustness(t *testing.T) {
	Convey("Market profiles: long staircase, chop whipsaw, and plateaus maintain correct excursion granularity", t, func() {
		Convey("A multi-plateau FOMO rise produces exactly one excursion spanning the full staircase", func() {
			records := collect(NewDetector(priced(t, "BTC/USD", 0.1)), "BTC/USD", calmThenLongFOMO())
			So(len(records), ShouldEqual, 1)
			So(records[0].Direction, ShouldEqual, "up")
			So(records[0].ClearsFriction, ShouldBeTrue)
			So(records[0].ExitTick-records[0].AnchorTick, ShouldBeGreaterThan, 80)
			So(records[0].ExtremumPrice, ShouldBeGreaterThan, 350)
		})

		Convey("High-frequency chop whipsaw does not emit profitable excursions", func() {
			chopTape := market.NewChopWhipsawTape("BTC/USD", 50000.0, 5.0, 60)
			detector := NewDetector(priced(t, "BTC/USD", 0.1))
			records, err := DetectExcursions(detector, chopTape)
			So(err, ShouldBeNil)
			// Chop within the deadband should never produce false profitable up excursions
			for _, r := range records {
				So(r.ClearsFriction, ShouldBeFalse)
			}
		})
	})
}
