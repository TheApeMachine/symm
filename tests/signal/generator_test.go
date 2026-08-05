package signal

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestGeneratorNewGenerator(t *testing.T) {
	Convey("Given a symbol name and start price", t, func() {
		generator := NewGenerator("SIM1/USD", 100.0, 42)

		Convey("It should initialize with default Baseline state", func() {
			So(generator, ShouldNotBeNil)
			So(generator.symbol, ShouldEqual, "SIM1/USD")
			So(generator.midPrice, ShouldEqual, 100.0)
			So(generator.currentState, ShouldEqual, testtypes.Baseline)
		})
	})
}

func TestGeneratorSetState(t *testing.T) {
	Convey("Given a generator", t, func() {
		generator := NewGenerator("SIM1/USD", 100.0, 42)

		Convey("When SetState is called", func() {
			generator.SetState(testtypes.FastPump)

			So(generator.targetState, ShouldEqual, testtypes.FastPump)
			So(generator.PrecursorPending(), ShouldBeTrue)
			So(generator.IgnitionArmed(), ShouldBeFalse)
		})
	})
}

func TestGeneratorStep(t *testing.T) {
	Convey("Given a generator", t, func() {
		generator := NewGenerator("SIM1/USD", 100.0, 42)

		Convey("When Step is called", func() {
			sample := generator.Step()

			So(sample.Symbol, ShouldEqual, "SIM1/USD")
			So(sample.Bid, ShouldBeGreaterThan, 0)
			So(sample.Ask, ShouldBeGreaterThan, sample.Bid)
			So(sample.Last, ShouldBeGreaterThan, 0)
			So(sample.Volume, ShouldBeGreaterThan, 0)
			So(sample.VWAP, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a FastPump transition", t, func() {
		generator := NewGenerator("SIM1/USD", 100.0, 42)
		profile := testtypes.DefaultProfiles[testtypes.FastPump]
		generator.SetState(testtypes.FastPump, testtypes.MomentumMap[testtypes.FastPump])

		for generator.PrecursorPending() {
			sample := generator.Step()
			So(sample.ChangePct,
				ShouldBeLessThan, profile.IgnitionMove*100.0)
			So(sample.AggressorSide, ShouldEqual, profile.AggressorSide)
			So(sample.Last, ShouldEqual, sample.Ask)
		}

		Convey("It should leave the full ignition armed for the next sample", func() {
			So(generator.IgnitionArmed(), ShouldBeTrue)

			ignition := generator.Step()

			So(ignition.ChangePct,
				ShouldBeGreaterThanOrEqualTo, profile.IgnitionMove*100.0)
			So(generator.IgnitionArmed(), ShouldBeFalse)
		})
	})
}

func TestGeneratorGenerate(t *testing.T) {
	Convey("Given a generator and JSON template", t, func() {
		generator := NewGenerator("SIM1/USD", 100.0, 42)
		template := []byte(`{"channel":"ticker","type":"snapshot","data":[{"symbol":"SIM1/USD","bid":100.0}]}`)

		Convey("When Generate is called", func() {
			count := 0

			for frame := range generator.Generate(template) {
				So(len(frame), ShouldBeGreaterThan, 0)
				count++
			}

			So(count, ShouldEqual, 1)
		})
	})
}

func TestGeneratorRender(t *testing.T) {
	Convey("Given one sampled market state and each venue channel template", t, func() {
		generator := NewGenerator("SIM1/USD", 100.0, 42)
		sample := generator.Step()
		marketTime := generator.currTime
		marketPrice := generator.midPrice
		stamp := sample.Timestamp.Format("2006-01-02T15:04:05.999999999Z07:00")

		tickerFrame := struct {
			Data []struct {
				Symbol    string  `json:"symbol"`
				Bid       float64 `json:"bid"`
				BidQty    float64 `json:"bid_qty"`
				Ask       float64 `json:"ask"`
				AskQty    float64 `json:"ask_qty"`
				Last      float64 `json:"last"`
				Volume    float64 `json:"volume"`
				Timestamp string  `json:"timestamp"`
			} `json:"data"`
		}{}
		bookFrame := struct {
			Data []struct {
				Symbol    string `json:"symbol"`
				Timestamp string `json:"timestamp"`
				Bids      []struct {
					Price float64 `json:"price"`
					Qty   float64 `json:"qty"`
				} `json:"bids"`
				Asks []struct {
					Price float64 `json:"price"`
					Qty   float64 `json:"qty"`
				} `json:"asks"`
			} `json:"data"`
		}{}
		tradeFrame := struct {
			Data []struct {
				Symbol    string  `json:"symbol"`
				Side      string  `json:"side"`
				Price     float64 `json:"price"`
				Qty       float64 `json:"qty"`
				Timestamp string  `json:"timestamp"`
			} `json:"data"`
		}{}
		level3Frame := struct {
			Data []struct {
				Symbol    string `json:"symbol"`
				Timestamp string `json:"timestamp"`
				Bids      []struct {
					Event     string  `json:"event"`
					OrderID   string  `json:"order_id"`
					Price     float64 `json:"limit_price"`
					Qty       float64 `json:"order_qty"`
					Timestamp string  `json:"timestamp"`
				} `json:"bids"`
				Asks []struct {
					Event     string  `json:"event"`
					OrderID   string  `json:"order_id"`
					Price     float64 `json:"limit_price"`
					Qty       float64 `json:"order_qty"`
					Timestamp string  `json:"timestamp"`
				} `json:"asks"`
			} `json:"data"`
		}{}

		err := json.Unmarshal(generator.Render(
			[]byte(`{"channel":"ticker","data":[{}]}`), sample,
		), &tickerFrame)
		So(err, ShouldBeNil)
		err = json.Unmarshal(generator.Render(
			[]byte(`{"channel":"book","data":[{}]}`), sample,
		), &bookFrame)
		So(err, ShouldBeNil)
		err = json.Unmarshal(generator.Render(
			[]byte(`{"channel":"trade","data":[{}]}`), sample,
		), &tradeFrame)
		So(err, ShouldBeNil)
		err = json.Unmarshal(generator.Render(
			[]byte(`{"channel":"level3","data":[{}]}`), sample,
		), &level3Frame)
		So(err, ShouldBeNil)

		So(tickerFrame.Data, ShouldHaveLength, 1)
		So(bookFrame.Data, ShouldHaveLength, 1)
		So(tradeFrame.Data, ShouldHaveLength, 1)
		So(level3Frame.Data, ShouldHaveLength, 1)
		So(bookFrame.Data[0].Bids, ShouldHaveLength, 1)
		So(bookFrame.Data[0].Asks, ShouldHaveLength, 1)
		So(level3Frame.Data[0].Bids, ShouldHaveLength, 1)
		So(level3Frame.Data[0].Asks, ShouldHaveLength, 1)

		So(tickerFrame.Data[0].Symbol, ShouldEqual, sample.Symbol)
		So(bookFrame.Data[0].Symbol, ShouldEqual, sample.Symbol)
		So(tradeFrame.Data[0].Symbol, ShouldEqual, sample.Symbol)
		So(level3Frame.Data[0].Symbol, ShouldEqual, sample.Symbol)
		So(tickerFrame.Data[0].Timestamp, ShouldEqual, stamp)
		So(bookFrame.Data[0].Timestamp, ShouldEqual, stamp)
		So(tradeFrame.Data[0].Timestamp, ShouldEqual, stamp)
		So(level3Frame.Data[0].Timestamp, ShouldEqual, stamp)
		So(level3Frame.Data[0].Bids[0].Timestamp, ShouldEqual, stamp)
		So(level3Frame.Data[0].Asks[0].Timestamp, ShouldEqual, stamp)

		So(tickerFrame.Data[0].Bid, ShouldEqual, sample.Bid)
		So(tickerFrame.Data[0].BidQty, ShouldEqual, sample.BidQty)
		So(tickerFrame.Data[0].Ask, ShouldEqual, sample.Ask)
		So(tickerFrame.Data[0].AskQty, ShouldEqual, sample.AskQty)
		So(bookFrame.Data[0].Bids[0].Price, ShouldEqual, sample.Bid)
		So(bookFrame.Data[0].Bids[0].Qty, ShouldEqual, sample.BidQty)
		So(bookFrame.Data[0].Asks[0].Price, ShouldEqual, sample.Ask)
		So(bookFrame.Data[0].Asks[0].Qty, ShouldEqual, sample.AskQty)
		So(level3Frame.Data[0].Bids[0].Price, ShouldEqual, sample.Bid)
		So(level3Frame.Data[0].Bids[0].Qty, ShouldEqual, sample.BidQty)
		So(level3Frame.Data[0].Asks[0].Price, ShouldEqual, sample.Ask)
		So(level3Frame.Data[0].Asks[0].Qty, ShouldEqual, sample.AskQty)
		So(tradeFrame.Data[0].Price, ShouldEqual, sample.Last)
		So(tradeFrame.Data[0].Qty, ShouldEqual, sample.StepVolume)
		So(tradeFrame.Data[0].Side, ShouldEqual, sample.AggressorSide)
		So(tradeFrame.Data[0].Price, ShouldBeGreaterThanOrEqualTo, sample.Bid)
		So(tradeFrame.Data[0].Price, ShouldBeLessThanOrEqualTo, sample.Ask)
		So(tickerFrame.Data[0].Last, ShouldEqual, tradeFrame.Data[0].Price)
		So(tickerFrame.Data[0].Volume, ShouldEqual, sample.Volume)

		So(generator.currTime, ShouldEqual, marketTime)
		So(generator.midPrice, ShouldEqual, marketPrice)

		nextFrame := level3Frame
		next := generator.Step()
		err = json.Unmarshal(generator.Render(
			[]byte(`{"channel":"level3","data":[{}]}`), next,
		), &nextFrame)
		So(err, ShouldBeNil)
		nextBid := nextFrame.Data[0].Bids[len(nextFrame.Data[0].Bids)-1]
		nextAsk := nextFrame.Data[0].Asks[len(nextFrame.Data[0].Asks)-1]
		So(nextBid.Event, ShouldBeIn, "add", "modify")
		So(nextAsk.Event, ShouldBeIn, "add", "modify")
		So(nextBid.OrderID,
			ShouldEqual, level3Frame.Data[0].Bids[0].OrderID)
		So(nextAsk.OrderID,
			ShouldEqual, level3Frame.Data[0].Asks[0].OrderID)
	})
}

func BenchmarkGeneratorStep(b *testing.B) {
	generator := NewGenerator("SIM1/USD", 100.0, 42)

	for b.Loop() {
		_ = generator.Step()
	}
}
