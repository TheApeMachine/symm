package impulse

import (
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

// tape supplies repeated rises, reversals, silence and renewed movement across owners.
type tape struct {
	frame   *data.Measurement[float64]
	trade   *data.Measurement[float64]
	signals []*data.Measurement[float64]
}

func newTape(count int) *tape {
	sample := &tape{frame: data.NewMeasurement[float64]("frame", nil),
		trade: data.NewMeasurement[float64]("spot", nil)}
	sample.trade.Label = "BTC/USD"
	sample.trade.Provenance["owner"] = "public"
	sample.trade.Provenance["channel"] = "trade"
	sample.trade.Metadata["venue"] = "true"
	sample.trade.Metadata["volume-unit"] = "base"
	sample.trade.Maturity = 1
	sample.trade.Metrics["qty"] = data.Metric[float64]{Label: "qty", Raw: 1, Exact: decimal.NewFromInt64(1)}
	sample.frame.Peers = append(sample.frame.Peers, sample.trade)

	for index := 0; index < count; index++ {
		name := string(rune('a' + index))
		measurement := data.NewMeasurement[float64](name, nil)
		measurement.Label = "BTC/USD"
		measurement.Maturity = 1
		measurement.Provenance["owner"] = name
		measurement.Metrics["value"] = data.Metric[float64]{Label: "value"}
		sample.signals = append(sample.signals, measurement)
		sample.frame.Peers = append(sample.frame.Peers, measurement)
	}
	return sample
}

func (sample *tape) step(sequence int64) *data.Measurement[float64] {
	sample.frame.SeqIdx, sample.trade.SeqIdx = sequence, sequence

	for index, measurement := range sample.signals {
		value := math.Sin(float64(sequence))

		if index%2 != 0 {
			value = -value
		}

		measurement.SeqIdx = sequence
		measurement.Metrics["value"] = data.Metric[float64]{Label: "value", Raw: value}
	}
	return sample.frame
}

func TestMapStep(t *testing.T) {
	Convey("A map addresses independently owned metrics on the recorded volume clock", t, func() {
		sample := newTape(3)
		live, replay := NewMap(), NewMap()

		for sequence := int64(1); sequence <= 24; sequence++ {
			frame := sample.step(sequence)
			So(live.Step(frame), ShouldBeNil)
			slices.Reverse(frame.Peers)
			So(replay.Step(frame), ShouldBeNil)
			So(replay.Markets["BTC/USD"].Impulse, ShouldResemble, live.Markets["BTC/USD"].Impulse)

			for index, cell := range live.Markets["BTC/USD"].Cells {
				So(replay.Markets["BTC/USD"].Cells[index].Position, ShouldResemble, cell.Position)
			}
		}

		market := live.Markets["BTC/USD"]
		So(market.Volume.String(), ShouldEqual, "24.000000000000")
		So(len(market.Impulse.Regions), ShouldBeGreaterThan, 0)

		Convey("Changing coordinates does not move or rewrite a producer value", func() {
			cell := market.addresses[[2]string{"a", "value"}]
			original, present := cell.Value()
			So(present, ShouldBeTrue)
			cell.Position.X += 7
			value, present := cell.Value()
			So(present, ShouldBeTrue)
			So(value, ShouldEqual, original)
			sample.signals[0].Metrics["value"] = data.Metric[float64]{Raw: 91}
			value, present = cell.Value()
			So(present, ShouldBeTrue)
			So(value, ShouldEqual, 91)
		})

		Convey("An out-of-order boundary is rejected", func() {
			So(live.Step(sample.frame), ShouldNotBeNil)
		})

		Convey("A rejected producer is explicitly absent and disables decisions only for its market", func() {
			frame := sample.step(25)
			sample.signals[0].Err = errors.New("rejected quote")
			So(live.Step(frame), ShouldBeNil)
			So(live.Invalid, ShouldEqual, 1)
			So(market.Impulse.Ready, ShouldBeFalse)
			So(market.addresses[[2]string{"a", "value"}].Present, ShouldBeFalse)
			sample.signals[0].Err = nil
			So(live.Step(sample.step(26)), ShouldBeNil)
			So(live.Invalid, ShouldEqual, 0)
			So(market.Impulse.Ready, ShouldBeTrue)
		})

		Convey("A quote advances order but not traded volume", func() {
			frame := sample.step(25)
			sample.trade.Provenance["channel"] = "ticker"
			before := market.Volume
			So(live.Step(frame), ShouldBeNil)
			So(market.Volume, ShouldEqual, before)
			So(market.Sequence, ShouldEqual, 25)
		})
	})
}

func BenchmarkMapStep(b *testing.B) {
	sample := newTape(249)
	impulseMap := NewMap()

	if err := impulseMap.Step(sample.step(1)); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for index := 0; b.Loop(); index++ {
		if err := impulseMap.Step(sample.step(int64(index + 2))); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMapStepQuote(b *testing.B) {
	sample := newTape(249)
	impulseMap := NewMap()

	if err := impulseMap.Step(sample.step(1)); err != nil {
		b.Fatal(err)
	}

	sample.trade.Provenance["channel"] = "ticker"
	b.ReportAllocs()

	for index := 0; b.Loop(); index++ {
		if err := impulseMap.Step(sample.step(int64(index + 2))); err != nil {
			b.Fatal(err)
		}
	}
}
