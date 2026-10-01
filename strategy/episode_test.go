package strategy

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestDetectorConfirmBeforeRecord(t *testing.T) {
	Convey("Given a calm tape and a sustained move", t, func() {
		detector := NewDetector(priced(t, "BTC/USD", 0.001))
		emitted := map[int64]bool{}
		var records []*tables.ExcursionRecord

		for seq, mid := range calmThenRise() {
			record, err := detector.Observe(quoteAt("BTC/USD", int64(seq+1), mid))
			So(err, ShouldBeNil)

			if record != nil {
				emitted[int64(seq+1)] = true
				records = append(records, record)
			}
		}

		So(len(records), ShouldEqual, 1)
		So(emitted[records[0].AnchorTick], ShouldBeFalse)
		So(emitted[records[0].AnchorTick+1], ShouldBeFalse)
		So(emitted[records[0].ExitTick], ShouldBeTrue)
		So(records[0].ExitTick, ShouldBeGreaterThan, records[0].AnchorTick)
		So(records[0].Direction, ShouldEqual, "up")
		So(records[0].ClearsFriction, ShouldBeTrue)
		So(records[0].Status, ShouldEqual, "resolved")
		So(records[0].PostEndTick, ShouldEqual, records[0].ExitTick)
	})
}

func TestDetectorFeeGates(t *testing.T) {
	Convey("Given an up move smaller than the fee", t, func() {
		detector := NewDetector(priced(t, "BTC/USD", 0.02))
		var records []*tables.ExcursionRecord

		for seq, mid := range calmThenSmallRise() {
			record, err := detector.Observe(quoteAt("BTC/USD", int64(seq+1), mid))
			So(err, ShouldBeNil)

			if record != nil {
				records = append(records, record)
			}
		}

		So(len(records), ShouldEqual, 1)
		So(records[0].Direction, ShouldEqual, "up")
		So(records[0].ClearsFriction, ShouldBeFalse)
	})

	Convey("Given a missing fee", t, func() {
		detector := NewDetector(broker.NewPrice(context.Background(), nil, nil, nil))
		var emitted *tables.ExcursionRecord
		var failed error

		for seq, mid := range calmThenRise() {
			record, err := detector.Observe(quoteAt("BTC/USD", int64(seq+1), mid))

			if err != nil {
				failed = err
			}

			if record != nil {
				emitted = record
			}
		}

		So(failed, ShouldNotBeNil)
		So(emitted, ShouldBeNil)
		So(detector.Error(), ShouldNotBeNil)
	})
}

func TestDetectorDownChopFlatAndDuplicates(t *testing.T) {
	Convey("Given a downward move", t, func() {
		detector := NewDetector(priced(t, "BTC/USD", 0.001))
		records := collect(detector, "BTC/USD", calmThenDrop())

		So(len(records), ShouldEqual, 1)
		So(records[0].Direction, ShouldEqual, "down")
		So(records[0].ClearsFriction, ShouldBeFalse)
	})

	Convey("Given both sides leave the calm and the long does not clear", t, func() {
		detector := NewDetector(priced(t, "BTC/USD", 0.02))
		record := firstRecord(detector, "BTC/USD", calmThenChop())

		So(record, ShouldNotBeNil)
		So(record.Direction, ShouldEqual, "chop")
		So(record.ClearsFriction, ShouldBeFalse)
	})

	Convey("Given a completed impulse followed by a longer calm", t, func() {
		detector := NewDetector(priced(t, "BTC/USD", 0.001))
		mids := calmThenRise()
		impulse, used := untilRecord(detector, "BTC/USD", mids)

		So(impulse, ShouldNotBeNil)

		exitMid := mids[used-1]
		var flat *tables.ExcursionRecord

		for index := 0; index < int(impulse.ObservationCount)+2; index++ {
			record, err := detector.Observe(quoteAt("BTC/USD", int64(used+index+1), exitMid))
			So(err, ShouldBeNil)

			if record != nil && record.Direction == "flat" {
				flat = record
			}
		}

		So(flat, ShouldNotBeNil)
		So(flat.ClearsFriction, ShouldBeFalse)
		So(flat.AnchorTick, ShouldEqual, flat.PrecursorStartTick)
	})

	Convey("Given a repeated sequence", t, func() {
		detector := NewDetector(priced(t, "BTC/USD", 0.001))
		frame := quoteAt("BTC/USD", 1, 100)
		_, err := detector.Observe(frame)
		So(err, ShouldBeNil)
		record, err := detector.Observe(frame)
		So(err, ShouldBeNil)
		So(record, ShouldBeNil)
	})

	Convey("Given two symbols on one detector", t, func() {
		detector := NewDetector(priced(t, "BTC/USD", 0.001))
		detector.price.SetFee("ETH/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.001)})
		btc := collect(detector, "BTC/USD", calmThenRise())
		eth := collect(detector, "ETH/USD", calmThenDrop())

		So(btc[0].Symbol, ShouldEqual, "BTC/USD")
		So(eth[0].Symbol, ShouldEqual, "ETH/USD")
		So(btc[0].Direction, ShouldEqual, "up")
		So(eth[0].Direction, ShouldEqual, "down")
	})
}

func priced(t *testing.T, symbol string, fee float64) *broker.Price {
	t.Helper()
	price := broker.NewPrice(context.Background(), nil, nil, nil)
	price.SetFee(symbol, kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(fee)})

	return price
}

func firstRecord(detector *Detector, symbol string, mids []float64) *tables.ExcursionRecord {
	record, _ := untilRecord(detector, symbol, mids)

	return record
}

func untilRecord(detector *Detector, symbol string, mids []float64) (*tables.ExcursionRecord, int) {
	for index, mid := range mids {
		record, err := detector.Observe(quoteAt(symbol, int64(index+1), mid))
		So(err, ShouldBeNil)

		if record != nil {
			return record, index + 1
		}
	}

	return nil, len(mids)
}

func collect(detector *Detector, symbol string, mids []float64) []*tables.ExcursionRecord {
	var records []*tables.ExcursionRecord

	for seq, mid := range mids {
		record, err := detector.Observe(quoteAt(symbol, int64(seq+1), mid))
		So(err, ShouldBeNil)

		if record != nil {
			records = append(records, record)
		}
	}

	return records
}

func quoteAt(symbol string, seq int64, mid float64) *data.Measurement[float64] {
	measurement := data.NewMeasurement[float64]("websocket", nil)
	measurement.Label = symbol
	measurement.SeqIdx = seq
	bid := mid - 0.01
	ask := mid + 0.01
	measurement.SetMetric("bid", data.Metric[float64]{Label: "bid", Raw: bid, Exact: decimal.NewFromFloat64(bid)})
	measurement.SetMetric("ask", data.Metric[float64]{Label: "ask", Raw: ask, Exact: decimal.NewFromFloat64(ask)})

	return measurement
}

func calmThenRise() []float64 {
	mids := calm(40, 100, 0.05)
	mids = append(mids, ramp(100.2, 0.4, 30)...)
	mids = append(mids, ramp(mids[len(mids)-1]-0.15, -0.35, 25)...)

	return mids
}

func calmThenSmallRise() []float64 {
	mids := calm(40, 100, 0.01)
	mids = append(mids, ramp(100.05, 0.02, 20)...)
	mids = append(mids, ramp(mids[len(mids)-1]-0.01, -0.015, 20)...)

	return mids
}

func calmThenDrop() []float64 {
	mids := calm(40, 100, 0.05)
	mids = append(mids, ramp(99.8, -0.4, 30)...)
	mids = append(mids, ramp(mids[len(mids)-1]+0.15, 0.35, 25)...)

	return mids
}

func calmThenChop() []float64 {
	mids := calm(40, 100, 0.05)
	mids = append(mids, ramp(100.4, 0.35, 24)...)
	peak := mids[len(mids)-1]
	mids = append(mids, peak-12)
	mids = append(mids, calm(18, peak-8, 0.03)...)

	return mids
}

func calm(count int, center float64, amplitude float64) []float64 {
	mids := make([]float64, count)

	for index := range mids {
		sign := 1.0

		if index%2 == 1 {
			sign = -1
		}

		mids[index] = center + sign*amplitude
	}

	return mids
}

func ramp(start float64, step float64, count int) []float64 {
	mids := make([]float64, count)

	for index := range mids {
		mids[index] = start + step*float64(index)
	}

	return mids
}
