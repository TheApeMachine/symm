package tables_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/tests/market"
)

func TestStreamingDetectorProcess(t *testing.T) {
	Convey("Volume regime changes resolve actual anchors with authoritative quote costs", t, func() {
		detector := tables.NewStreamingDetector(1, market.TrainingPrice(t.Context()))
		var records []tables.ExcursionRecord
		for _, frame := range market.TrainingTape(6) {
			for _, measurement := range frame.Peers {
				record, err := detector.Process(measurement)
				So(err, ShouldBeNil)
				if record != nil {
					records = append(records, *record)
				}
			}
		}
		So(len(records), ShouldBeGreaterThan, 1)
		var profitable, losing bool
		for _, record := range records {
			So(record.AnchorTick, ShouldBeGreaterThan, 0)
			So(record.ExitTick, ShouldBeGreaterThan, record.AnchorTick)
			So(record.Fee, ShouldBeGreaterThan, 0)
			profitable = profitable || record.Profit > 0
			losing = losing || record.Profit < 0
		}
		So(profitable, ShouldBeTrue)
		So(losing, ShouldBeTrue)
	})

	Convey("Futures and quotes do not advance the volume baseline", t, func() {
		detector := tables.NewStreamingDetector(1, market.TrainingPrice(t.Context()))
		for _, frame := range market.TrainingTape(2) {
			quote, trade := frame.Peers[0], frame.Peers[1]
			trade.Metadata["volume-unit"] = "contracts"
			for _, measurement := range []*data.Measurement[float64]{quote, trade} {
				record, err := detector.Process(measurement)
				So(err, ShouldBeNil)
				So(record, ShouldBeNil)
			}
		}
		So(detector.Anchor("BTC/USD"), ShouldEqual, 0)
	})
}

func BenchmarkStreamingDetectorProcess(b *testing.B) {
	detector := tables.NewStreamingDetector(1, market.TrainingPrice(b.Context()))
	frames := market.TrainingTape(6)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		for _, measurement := range frames[index%len(frames)].Peers {
			if _, err := detector.Process(measurement); err != nil {
				b.Fatal(err)
			}
		}
	}
}
