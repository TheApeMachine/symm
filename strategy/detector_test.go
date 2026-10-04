package strategy

import (
	"context"
	"strconv"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/tests/market"
)

/*
tradeTape turns synthetic market legs into stored spot:trade measurements of one
epoch, numbering ticks and sequence indices in tape order.
*/
func tradeTape(t *testing.T, epoch int64, legs ...[]*data.Measurement[float64]) []*data.Measurement[float64] {
	var tape []*data.Measurement[float64]

	for _, leg := range legs {
		for _, frame := range leg {
			raw := frame.GetMetric("price").Raw
			exact, err := decimal.NewFromString(strconv.FormatFloat(raw, 'f', 2, 64))

			if err != nil {
				t.Fatal(err)
			}

			trade := data.NewMeasurement("spot:trade", map[string]data.Metric[float64]{
				"price": {Raw: exact.Float64(), Exact: exact},
			})
			trade.Epoch = epoch
			trade.Label = frame.Label
			trade.Tick = int64(len(tape) + 1)
			trade.SeqIdx = trade.Tick
			trade.At = frame.At
			tape = append(tape, trade)
		}
	}

	return tape
}

/*
drawUp is the brute-force oracle: the low/high tick pair with the largest
relative gain where the low strictly precedes the high.
*/
func drawUp(tape []*data.Measurement[float64]) (int64, int64) {
	var (
		best float64
		low  int64
		high int64
	)

	for left := range tape {
		for right := left + 1; right < len(tape); right++ {
			gain := tape[right].GetMetric("price").Raw / tape[left].GetMetric("price").Raw

			if gain <= 1 || gain <= best {
				continue
			}

			best, low, high = gain, tape[left].Tick, tape[right].Tick
		}
	}

	return low, high
}

func TestDetector_Scan(t *testing.T) {
	Convey("Given a detector storing into a ready store tee", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)
		detector := NewDetector(ctx, storeTee)

		Convey("When the tape's highest price comes before its lowest price", func() {
			tape := tradeTape(
				t,
				100,
				market.NewDownwardBreakdownTape("BTC/USD", 60000, 10),
				market.NewAdversarialMultiWickTape("BTC/USD", 57000, 10),
			)

			detections, err := detector.Scan(func(yield func(*data.Measurement[float64]) bool) {
				for _, trade := range tape {
					if !yield(trade) {
						return
					}
				}
			})

			Convey("It still yields the largest low-before-high excursion", func() {
				So(err, ShouldBeNil)
				So(detections, ShouldHaveLength, 1)

				low, high := drawUp(tape)
				So(detections[0].GetMetric("LowTick").Raw, ShouldEqual, float64(low))
				So(detections[0].GetMetric("HighTick").Raw, ShouldEqual, float64(high))
				So(detections[0].GetMetric("HighPrice").Exact.Cmp(
					detections[0].GetMetric("LowPrice").Exact,
				), ShouldBeGreaterThan, 0)
				So(storeTee.Pending(), ShouldEqual, 1)
			})
		})

		Convey("When a trade carries no exact price", func() {
			trade := data.NewMeasurement("spot:trade", map[string]data.Metric[float64]{
				"price": {Raw: 60000},
			})
			trade.Epoch = 100
			trade.Label = "BTC/USD"
			trade.Tick = 1

			_, err := detector.Scan(func(yield func(*data.Measurement[float64]) bool) {
				yield(trade)
			})

			Convey("It fails instead of skipping the trade", func() {
				So(err, ShouldNotBeNil)
				So(storeTee.Pending(), ShouldEqual, 0)
			})
		})
	})
}
