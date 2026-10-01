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
		detector := NewDetector(priced(t, "BTC/USD", 0.1))
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
		detector := NewDetector(priced(t, "BTC/USD", 2.0))
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
		detector := NewDetector(broker.NewPrice(context.Background(), nil, nil, nil, nil))
		var emitted *tables.ExcursionRecord

		for seq, mid := range calmThenRise() {
			record, err := detector.Observe(quoteAt("BTC/USD", int64(seq+1), mid))
			So(err, ShouldBeNil)

			if record != nil {
				emitted = record
			}
		}

		So(emitted, ShouldBeNil)
		So(detector.Error(), ShouldBeNil)
	})
}

func TestDetectorDownChopFlatAndDuplicates(t *testing.T) {
	Convey("Given a downward move", t, func() {
		detector := NewDetector(priced(t, "BTC/USD", 0.1))
		records := collect(detector, "BTC/USD", calmThenDrop())

		So(len(records), ShouldEqual, 1)
		So(records[0].Direction, ShouldEqual, "down")
		So(records[0].ClearsFriction, ShouldBeFalse)
	})

	Convey("Given both sides leave the calm and the long does not clear", t, func() {
		detector := NewDetector(priced(t, "BTC/USD", 2.0))
		record := firstRecord(detector, "BTC/USD", calmThenChop())

		So(record, ShouldNotBeNil)
		So(record.Direction, ShouldEqual, "chop")
		So(record.ClearsFriction, ShouldBeFalse)
	})

	Convey("Given a completed impulse followed by a longer calm", t, func() {
		detector := NewDetector(priced(t, "BTC/USD", 0.1))
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
		detector := NewDetector(priced(t, "BTC/USD", 0.1))
		frame := quoteAt("BTC/USD", 1, 100)
		_, err := detector.Observe(frame)
		So(err, ShouldBeNil)
		record, err := detector.Observe(frame)
		So(err, ShouldBeNil)
		So(record, ShouldBeNil)
	})

	Convey("Given two symbols on one detector", t, func() {
		detector := NewDetector(priced(t, "BTC/USD", 0.1))
		detector.price.SetFee("ETH/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.1)})
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
	price := broker.NewPrice(context.Background(), nil, nil, nil, nil)
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

func TestDecimalRatioAvoidsDivPanic(t *testing.T) {
	Convey("Given a scale-mismatched Decimal.Div that panics", t, func() {
		// NewFromFloat64(100) has scale 0; 1e-7 SetScale(0) rounds to 0 and
		// Decimal.Div panics inside BankersRound even though Sign() != 0.
		num := decimal.NewFromFloat64(100.0)
		den := decimal.NewFromFloat64(1e-7)

		So(func() { _ = num.Sub(den).Div(den) }, ShouldPanic)

		ratio, ok := decimalRatio(num.Sub(den), den)
		So(ok, ShouldBeTrue)
		So(ratio, ShouldBeGreaterThan, 0)
	})

	Convey("Given a zero divisor", t, func() {
		num := decimal.NewFromInt64(1)
		den := decimal.NewFromInt64(0)
		ratio, ok := decimalRatio(num, den)
		So(ok, ShouldBeFalse)
		So(ratio, ShouldEqual, 0)
	})
}

func TestRecordRejectsNonPositiveReference(t *testing.T) {
	Convey("Given a resolved path with a zero prior mean", t, func() {
		ask := decimal.NewFromFloat64(100.1)
		bid := decimal.NewFromFloat64(99.9)
		fee := decimal.NewFromFloat64(0.001)
		path := &series{
			symbol:    "BTC/USD",
			priorMean: 0,
			entryAsk:  ask,
			highMid:   110,
			lowMid:    90,
			highBid:   bid,
			lowAsk:    ask,
			highTick:  10,
			lowTick:   8,
			anchor:    5,
			precursor: 1,
			openCount: 4,
		}
		one := decimal.NewFromInt64(1)
		cost := ask.Mul(one.Add(fee))
		proceeds := bid.Mul(one.Sub(fee))
		seen := quote{symbol: "BTC/USD", seq: 20, bid: bid, ask: ask, mid: 100}

		So(func() {
			_, err := path.record(seen, fee, cost, proceeds, "up", true, false)
			So(err, ShouldNotBeNil)
		}, ShouldNotPanic)
	})

	Convey("Given a zero cost", t, func() {
		ask := decimal.NewFromFloat64(100.1)
		bid := decimal.NewFromFloat64(99.9)
		fee := decimal.NewFromFloat64(0.001)
		path := &series{
			symbol:    "BTC/USD",
			priorMean: 100,
			highMid:   110,
			highBid:   bid,
			highTick:  10,
			anchor:    5,
			precursor: 1,
			openCount: 4,
		}
		seen := quote{symbol: "BTC/USD", seq: 20, bid: bid, ask: ask, mid: 100}
		cost := decimal.NewFromInt64(0)
		proceeds := bid

		So(func() {
			_, err := path.record(seen, fee, cost, proceeds, "up", false, false)
			So(err, ShouldNotBeNil)
		}, ShouldNotPanic)
	})
}

func TestDetectorLongFOMOStaysOneEpisode(t *testing.T) {
	Convey("A multi-plateau FOMO rise is one excursion that can clear friction", t, func() {
		detector := NewDetector(priced(t, "BTC/USD", 0.1))
		records := collect(detector, "BTC/USD", calmThenLongFOMO())

		So(len(records), ShouldEqual, 1)
		So(records[0].Direction, ShouldEqual, "up")
		So(records[0].ClearsFriction, ShouldBeTrue)
		So(records[0].ExitTick-records[0].AnchorTick, ShouldBeGreaterThan, 80)
		// Peak retained through plateaus — not chopped into many short ups.
		So(records[0].ExtremumPrice, ShouldBeGreaterThan, 350)
	})
}

/*
calmThenLongFOMO models a minutes-scale riser (same shape as SWEAT ~4x / ~80m+):
calm base, staircase higher with plateaus (non-extending consolidations), then a
deeper giveback to resolve C. Mids stay in a quote-valid mid±0.01 band.
*/
func calmThenLongFOMO() []float64 {
	base := 100.0
	mids := calm(50, base, 0.05)

	price := mids[len(mids)-1]
	target := 401.0

	for step := 0; step < 12; step++ {
		next := price + (target-price)/float64(12-step)
		// ramp(start, per-tick step, count) — not total delta as step.
		perTick := (next - price) / 14.0
		mids = append(mids, ramp(price, perTick, 15)...)
		price = mids[len(mids)-1]
		// Plateau under the high — must not finish into fee-failing chips.
		mids = append(mids, calm(20, price-0.15, 0.04)...)
		price = mids[len(mids)-1]
	}

	// Exhaustion: long giveback toward prior so recent.Mean sheds the peak
	// (MeanShift recent window) and retained drops through the FOMO floor.
	perTick := -(price - base) * 0.97 / 99.0
	mids = append(mids, ramp(price, perTick, 100)...)

	return mids
}
