package strategy

import (
	"context"
	"fmt"
	"iter"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/tests/market"
)

func metricRaw(measurement *data.Measurement, key string) float64 {
	metric, err := readMetric(measurement, key)
	if err != nil || metric == nil {
		panic(fmt.Sprintf("metricRaw: %s missing (err=%v)", key, err))
	}
	return metric.Raw
}

func tradeWithPrice(epoch int64, label string, tick int64, exact *decimal.Decimal) *data.Measurement {
	trade := data.NewMeasurement(epoch, label, "spot:trade", tick, tick)
	trade.At = tradeAt(tick)
	trade.From = trade.At
	return trade.Write(
		data.NewExactMetric("price", exact, data.UnitPrice, data.TimescaleTick),
	)
}

/*
tradeAt is the synthetic venue time of a fixture trade at tick.
*/
func tradeAt(tick int64) time.Time {
	return time.Unix(1_700_000_000+tick, 0).UTC()
}

/*
tradeTape turns synthetic market legs into stored spot:trade measurements of one
epoch, numbering ticks and sequence indices in tape order.
*/
func tradeTape(t *testing.T, epoch int64, legs ...[]*data.Measurement) []*data.Measurement {
	var tape []*data.Measurement

	for _, leg := range legs {
		for _, frame := range leg {
			raw := metricRaw(frame, "price")
			exact, err := decimal.NewFromString(strconv.FormatFloat(raw, 'f', 8, 64))

			if err != nil {
				t.Fatal(err)
			}

			trade := tradeWithPrice(epoch, frame.Label, int64(len(tape)+1), exact)
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
func drawUp(tape []*data.Measurement) (int64, int64) {
	var (
		best float64
		low  int64
		high int64
	)

	for left := range tape {
		for right := left + 1; right < len(tape); right++ {
			pLeft := metricRaw(tape[left], "price")
			pRight := metricRaw(tape[right], "price")

			if pLeft <= 0 {
				continue
			}

			gain := pRight / pLeft

			if gain <= 1 || gain <= best {
				continue
			}

			best, low, high = gain, tape[left].Tick, tape[right].Tick
		}
	}

	return low, high
}

func testPrice(ctx context.Context, symbols ...string) *broker.Price {
	normalizer := spot.NewNormalizer()
	assets := make(map[string]spot.AssetInfo)
	pairs := make(map[string]spot.AssetPair)

	for _, s := range symbols {
		parts := strings.Split(s, "/")
		base, quote := parts[0], parts[1]
		assets[base] = spot.AssetInfo{AltName: base}
		assets[quote] = spot.AssetInfo{AltName: quote}
		pairs[s] = spot.AssetPair{
			WSName:        s,
			Base:          base,
			Quote:         quote,
			LotDecimals:   8,
			LotMultiplier: 1,
		}
	}

	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: assets,
		NewPairs:  pairs,
	})

	price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
	for _, s := range symbols {
		price.SetFee(s, kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		})
	}
	price.SetReferenceCash(decimal.NewFromFloat64(200))
	return price
}

func popMeasurement(storeTee *hindsight.StoreTee) *data.Measurement {
	ptr := storeTee.Next()
	if ptr == nil {
		return nil
	}
	pub := *(*data.Publication)(ptr)
	return pub.Measurement
}

/*
scan feeds one tape through the detector and drains every published
detection, grouped by its excursion class.
*/
func scan(
	detector *Detector, storeTee *hindsight.StoreTee, tape []*data.Measurement,
) map[string][]*data.Measurement {
	So(detector.Scan(tapeSeq(tape)), ShouldBeNil)

	return drain(storeTee)
}

func tapeSeq(tape []*data.Measurement) iter.Seq2[*data.Measurement, error] {
	return func(yield func(*data.Measurement, error) bool) {
		for _, trade := range tape {
			if !yield(trade, nil) {
				return
			}
		}
	}
}

/*
failingSeq yields the tape and then a read error, the way a catalog scan
reports a storage failure part-way through a tape.
*/
func failingSeq(tape []*data.Measurement, readErr error) iter.Seq2[*data.Measurement, error] {
	return func(yield func(*data.Measurement, error) bool) {
		for _, trade := range tape {
			if !yield(trade, nil) {
				return
			}
		}

		yield(nil, readErr)
	}
}

/*
drain pops every published detection, grouped by its excursion class.
*/
func drain(storeTee *hindsight.StoreTee) map[string][]*data.Measurement {
	byClass := make(map[string][]*data.Measurement)

	for detection := popMeasurement(storeTee); detection != nil; detection = popMeasurement(storeTee) {
		byClass[detection.Meta("type")] = append(byClass[detection.Meta("type")], detection)
	}

	return byClass
}

func priceTape(t *testing.T, label string, prices ...string) []*data.Measurement {
	tape := make([]*data.Measurement, len(prices))

	for i, pStr := range prices {
		exact, err := decimal.NewFromString(pStr)
		if err != nil {
			t.Fatal(err)
		}

		tape[i] = tradeWithPrice(100, label, int64(i+1), exact)
	}

	return tape
}

func TestDetector_Scan(t *testing.T) {
	Convey("Given a detector storing into a ready store tee", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)
		detector := NewDetector(ctx, storeTee, testPrice(ctx, "BTC/USD", "ETH/USD", "SWEAT/USD"))

		Convey("When the tape's highest price comes before its lowest price (adversarial wick breakdown)", func() {
			tape := tradeTape(
				t,
				100,
				market.NewDownwardBreakdownTape("BTC/USD", 60000, 10),
				market.NewAdversarialMultiWickTape("BTC/USD", 57000, 10),
			)

			byClass := scan(detector, storeTee, tape)

			Convey("Its up excursion is the exact maximum low-before-high move confirmed by the brute-force oracle", func() {
				So(byClass[excursionUp], ShouldHaveLength, 1)

				detection := byClass[excursionUp][0]
				low, high := drawUp(tape)
				So(metricRaw(detection, "b_tick"), ShouldEqual, float64(low))
				So(metricRaw(detection, "c_tick"), ShouldEqual, float64(high))
				So(metricRaw(detection, "c_price"), ShouldBeGreaterThan, metricRaw(detection, "b_price"))
			})

			Convey("Its down excursion falls from an earlier top to a later bottom", func() {
				So(byClass[excursionDown], ShouldHaveLength, 1)

				detection := byClass[excursionDown][0]
				So(metricRaw(detection, "b_tick"), ShouldBeLessThan, metricRaw(detection, "c_tick"))
				So(metricRaw(detection, "c_price"), ShouldBeLessThan, metricRaw(detection, "b_price"))
			})
		})

		Convey("When a stop-loss hunter wicks down to a new low after an earlier macro run", func() {
			// Early macro run: 100 -> 250 (+150% gain) across ticks 1 to 6.
			// Then at tick 15, a stoploss hunter plunges to 40 (new lower trough).
			// From 40, it only weakly recovers to 50 (+25% gain).
			byClass := scan(detector, storeTee, priceTape(t, "BTC/USD",
				"100", "110", "130", "160", "200", "250", "240", "220", "200", "180",
				"150", "120", "80", "50", "40", "42", "45", "48", "50", "49",
			))

			Convey("The earlier +150% macro excursion is preserved and NOT destroyed by the stop-loss hunt", func() {
				So(byClass[excursionUp], ShouldHaveLength, 1)

				detection := byClass[excursionUp][0]
				// Must choose the 100 -> 250 run, NOT the 40 -> 50 bounce!
				So(metricRaw(detection, "b_price"), ShouldEqual, 100)
				So(metricRaw(detection, "c_price"), ShouldEqual, 250)
				So(metricRaw(detection, "b_tick"), ShouldEqual, 1)
				So(metricRaw(detection, "c_tick"), ShouldEqual, 6)
			})

			Convey("The stop-loss hunt itself is the down excursion, from the 250 top to the 40 wick", func() {
				So(byClass[excursionDown], ShouldHaveLength, 1)

				detection := byClass[excursionDown][0]
				So(metricRaw(detection, "b_price"), ShouldEqual, 250)
				So(metricRaw(detection, "c_price"), ShouldEqual, 40)
				So(metricRaw(detection, "b_tick"), ShouldEqual, 6)
				So(metricRaw(detection, "c_tick"), ShouldEqual, 15)
				So(metricRaw(detection, "start_tick"), ShouldEqual, 1)
			})
		})

		Convey("When a larger macro excursion follows a stop-loss wick", func() {
			// Small early run: 100 -> 120 (+20%).
			// Then stop-loss wick drops to 50 (new trough).
			// Then massive macro run from that wick: 50 -> 250 (+400%)!
			byClass := scan(detector, storeTee, priceTape(t, "BTC/USD",
				"100", "110", "120", "90", "70", "50", "80", "120", "180", "250",
			))

			Convey("The +400% run starting from the trough is correctly detected", func() {
				So(byClass[excursionUp], ShouldHaveLength, 1)

				detection := byClass[excursionUp][0]
				So(metricRaw(detection, "b_price"), ShouldEqual, 50)
				So(metricRaw(detection, "c_price"), ShouldEqual, 250)
				So(metricRaw(detection, "b_tick"), ShouldEqual, 6)
				So(metricRaw(detection, "c_tick"), ShouldEqual, 10)
				So(metricRaw(detection, "start_tick"), ShouldBeLessThan, metricRaw(detection, "b_tick"))
				So(metricRaw(detection, "end_tick"), ShouldBeGreaterThanOrEqualTo, metricRaw(detection, "c_tick"))
			})
		})

		Convey("When a multi-candle macro excursion has intermediate pullbacks (SWEAT/USD style)", func() {
			// Trough at 0.00042 -> pullbacks at 0.00063 and 0.00075 -> blowoff peak at 0.00105 (+150%)
			// -> crash to 0.00065
			byClass := scan(detector, storeTee, priceTape(t, "SWEAT/USD",
				"0.00042", "0.00050", "0.00065", "0.00085", "0.00070", "0.00063",
				"0.00078", "0.00095", "0.00082", "0.00075", "0.00090", "0.00105",
				"0.00092", "0.00078", "0.00065",
			))

			Convey("The full multi-candle excursion from 0.00042 to 0.00105 is captured", func() {
				So(byClass[excursionUp], ShouldHaveLength, 1)

				detection := byClass[excursionUp][0]
				So(detection.Label, ShouldEqual, "SWEAT/USD")
				So(metricRaw(detection, "b_price"), ShouldEqual, 0.00042)
				So(metricRaw(detection, "c_price"), ShouldEqual, 0.00105)
				So(metricRaw(detection, "b_tick"), ShouldEqual, 1)
				So(metricRaw(detection, "c_tick"), ShouldEqual, 12)
			})
		})

		Convey("When multiple symbols and epochs stream through sequentially", func() {
			symbols := []string{"BTC/USD", "ETH/USD"}
			epochs := []int64{100, 101}

			var multiTape []*data.Measurement
			tick := int64(1)

			for _, ep := range epochs {
				for _, sym := range symbols {
					// 50 -> 100 run for each symbol and epoch, then 100 -> 80.
					for _, pStr := range []string{"60", "50", "70", "90", "100", "80"} {
						exact, _ := decimal.NewFromString(pStr)
						multiTape = append(multiTape, tradeWithPrice(ep, sym, tick, exact))
						tick++
					}
				}
			}

			byClass := scan(detector, storeTee, multiTape)

			Convey("All 4 independent symbol-epoch up and down excursions are flushed", func() {
				So(byClass[excursionUp], ShouldHaveLength, 4)
				So(byClass[excursionDown], ShouldHaveLength, 4)

				for _, det := range byClass[excursionUp] {
					So(metricRaw(det, "b_price"), ShouldEqual, 50)
					So(metricRaw(det, "c_price"), ShouldEqual, 100)
				}

				for _, det := range byClass[excursionDown] {
					So(metricRaw(det, "b_price"), ShouldEqual, 100)
					So(metricRaw(det, "c_price"), ShouldEqual, 80)
				}
			})
		})

		Convey("When a tape is monotonically downward with no rallies", func() {
			byClass := scan(detector, storeTee, priceTape(t, "BTC/USD",
				"100", "95", "90", "85", "80", "70", "60",
			))

			Convey("Zero false up excursions are flushed", func() {
				So(byClass[excursionUp], ShouldBeEmpty)
				So(byClass[excursionUpShort], ShouldBeEmpty)
			})

			Convey("The whole fall is the down excursion", func() {
				So(byClass[excursionDown], ShouldHaveLength, 1)

				detection := byClass[excursionDown][0]
				So(metricRaw(detection, "b_tick"), ShouldEqual, 1)
				So(metricRaw(detection, "c_tick"), ShouldEqual, 7)
			})
		})

		Convey("When a trade carries no exact price", func() {
			tape := priceTape(t, "BTC/USD", "100", "250")
			trade := data.NewMeasurement(100, "BTC/USD", "spot:trade", 3, 3)
			trade.At = tradeAt(3)
			trade.From = trade.At
			trade.Write(data.NewMetric("price", 60000, data.UnitPrice, data.TimescaleTick))
			tape = append(tape, trade)

			err := detector.Scan(tapeSeq(tape))

			Convey("The scan halts with an error and publishes nothing", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "exact price")
				So(drain(storeTee), ShouldBeEmpty)
				So(detector.Status(), ShouldEqual, runtime.ERROR)
			})
		})

		Convey("When a trade carries no venue time", func() {
			tape := priceTape(t, "BTC/USD", "100", "250")
			exact, err := decimal.NewFromString("60000")
			So(err, ShouldBeNil)
			trade := data.NewMeasurement(100, "BTC/USD", "spot:trade", 3, 3)
			trade.Write(data.NewExactMetric("price", exact, data.UnitPrice, data.TimescaleTick))
			tape = append(tape, trade)

			err = detector.Scan(tapeSeq(tape))

			Convey("The scan halts with an error and publishes nothing", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "venue time")
				So(drain(storeTee), ShouldBeEmpty)
				So(detector.Status(), ShouldEqual, runtime.ERROR)
			})
		})

		Convey("When the trade tape read fails part-way", func() {
			// Read to completion, this tape holds a full 100 -> 250 up and
			// a 250 -> 80 down excursion.
			readErr := errnie.Err(errnie.BadGateway, "[iceberg] batch decode failure for measurements", nil)
			err := detector.Scan(failingSeq(priceTape(t, "BTC/USD", "100", "250", "80", "300"), readErr))

			Convey("The scan halts with the read error and flushes no partial tape", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "trade tape read failed")
				So(err.Error(), ShouldContainSubstring, "batch decode failure")
				So(drain(storeTee), ShouldBeEmpty)
				So(detector.Status(), ShouldEqual, runtime.ERROR)
			})
		})
	})
}

func TestDetector_RequiresFriction(t *testing.T) {
	Convey("Given a detector constructed without a price system", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)
		detector := NewDetector(ctx, storeTee, nil)

		Convey("It is rejected at construction", func() {
			So(detector.Status(), ShouldEqual, runtime.ERROR)
			So(detector.Error(), ShouldNotBeNil)
			So(detector.Error().Error(), ShouldContainSubstring, "price is required")
		})

		Convey("Scan refuses to classify any tape and publishes nothing", func() {
			err := detector.Scan(tapeSeq(priceTape(t, "BTC/USD", "100", "250", "80")))

			So(err, ShouldNotBeNil)
			So(drain(storeTee), ShouldBeEmpty)
		})
	})

	Convey("Given a detector whose price system has no fee for the scanned symbol", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)
		detector := NewDetector(ctx, storeTee, testPrice(ctx, "BTC/USD"))

		So(detector.Status(), ShouldNotEqual, runtime.ERROR)

		Convey("Scan halts on the unpriceable round trip instead of treating friction as zero", func() {
			err := detector.Scan(tapeSeq(priceTape(t, "ETH/USD", "100", "250", "80")))

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "ETH/USD")
			So(drain(storeTee), ShouldBeEmpty)
			So(detector.Status(), ShouldEqual, runtime.ERROR)
		})
	})
}

func TestDetector_FrictionGating(t *testing.T) {
	Convey("Given a detector wired to a Price manager with realistic taker fees", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		priceMgr := testPrice(ctx, "BTC/USD")
		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)
		detector := NewDetector(ctx, storeTee, priceMgr)

		roundTrip := func(detection *data.Measurement) *decimal.Decimal {
			b, c, err := tables.DetectionPrices(detection)
			So(err, ShouldBeNil)

			pnl, _, err := priceMgr.RoundTrip("BTC/USD", b, c)
			So(err, ShouldBeNil)
			return pnl
		}

		Convey("When a tape contains only sub-friction noise (+0.15% gain against 0.52% roundtrip fees)", func() {
			byClass := scan(detector, storeTee, tradeTape(
				t,
				100,
				market.NewUnprofitableUpperTape("BTC/USD", 60000, 10),
			))

			Convey("No up excursion is flushed", func() {
				So(byClass[excursionUp], ShouldBeEmpty)
			})

			Convey("The weak ignition is stored as the up_friction near miss, which loses after fees", func() {
				So(byClass[excursionUpShort], ShouldHaveLength, 1)

				detection := byClass[excursionUpShort][0]
				So(metricRaw(detection, "c_price"), ShouldBeGreaterThan, metricRaw(detection, "b_price"))
				So(metricRaw(detection, "c_tick"), ShouldBeGreaterThan, metricRaw(detection, "b_tick"))
				So(roundTrip(detection).Sign(), ShouldBeLessThanOrEqualTo, 0)
			})

			Convey("The sub-friction reversal is never a down excursion", func() {
				So(byClass[excursionDown], ShouldBeEmpty)
			})
		})

		Convey("When a tape contains a true profitable macro breakout (+3.5% gain clearing friction)", func() {
			byClass := scan(detector, storeTee, tradeTape(
				t,
				100,
				market.NewProfitableUpperTape("BTC/USD", 60000, 10),
			))

			Convey("The excursion comfortably clears roundtrip friction and is flushed as up", func() {
				So(byClass[excursionUp], ShouldHaveLength, 1)

				detection := byClass[excursionUp][0]
				So(detection.Label, ShouldEqual, "BTC/USD")
				So(detection.Epoch, ShouldEqual, 100)
				So(metricRaw(detection, "c_price"), ShouldBeGreaterThan, metricRaw(detection, "b_price"))
				So(roundTrip(detection).Sign(), ShouldBeGreaterThan, 0)
			})

			Convey("Every up_friction near miss it also stores loses after fees", func() {
				for _, detection := range byClass[excursionUpShort] {
					So(roundTrip(detection).Sign(), ShouldBeLessThanOrEqualTo, 0)
				}
			})
		})

		Convey("When a tape contains oscillating chop whipsaws within the spread", func() {
			tape := tradeTape(
				t,
				100,
				market.NewChopWhipsawTape("BTC/USD", 60000, 10, 50),
			)
			byClass := scan(detector, storeTee, tape)

			Convey("Chop within the friction deadband is never flushed as an up or down excursion", func() {
				So(byClass[excursionUp], ShouldBeEmpty)
				So(byClass[excursionDown], ShouldBeEmpty)
			})

			Convey("The whole deadband tape is stored as one chop stretch", func() {
				So(byClass[excursionChop], ShouldHaveLength, 1)

				detection := byClass[excursionChop][0]
				So(metricRaw(detection, "b_tick"), ShouldEqual, float64(tape[0].Tick))
				So(metricRaw(detection, "c_tick"), ShouldEqual, float64(tape[len(tape)-1].Tick))
				So(metricRaw(detection, "start_tick"), ShouldBeLessThanOrEqualTo, metricRaw(detection, "b_tick"))
				So(metricRaw(detection, "end_tick"), ShouldBeGreaterThanOrEqualTo, metricRaw(detection, "c_tick"))
			})
		})

		Convey("When a tape is a motionless flat line", func() {
			tape := tradeTape(
				t,
				100,
				market.NewFlatQuiescentTape("BTC/USD", 60000, 10, 20),
			)
			byClass := scan(detector, storeTee, tape)

			Convey("It is stored as one flat run and nothing else", func() {
				So(byClass, ShouldHaveLength, 1)
				So(byClass[excursionFlat], ShouldHaveLength, 1)

				detection := byClass[excursionFlat][0]
				So(metricRaw(detection, "b_tick"), ShouldEqual, float64(tape[0].Tick))
				So(metricRaw(detection, "c_tick"), ShouldEqual, float64(tape[len(tape)-1].Tick))
				So(metricRaw(detection, "b_price"), ShouldEqual, metricRaw(detection, "c_price"))
			})
		})

		Convey("When chop breaks out into a profitable rally that then collapses", func() {
			byClass := scan(detector, storeTee, priceTape(t, "BTC/USD",
				"60000", "60010", "59995", "60005", "60000", "60008",
				"60500", "61200", "62000", "62500",
				"61500", "60500", "59800",
			))

			Convey("It stores the chop stretch, the rally as up, and the collapse as down", func() {
				So(byClass[excursionChop], ShouldHaveLength, 1)
				So(metricRaw(byClass[excursionChop][0], "b_tick"), ShouldEqual, 1)
				So(metricRaw(byClass[excursionChop][0], "c_tick"), ShouldEqual, 6)

				So(byClass[excursionUp], ShouldHaveLength, 1)
				So(metricRaw(byClass[excursionUp][0], "b_tick"), ShouldEqual, 3)
				So(metricRaw(byClass[excursionUp][0], "c_tick"), ShouldEqual, 10)

				So(byClass[excursionDown], ShouldHaveLength, 1)
				So(metricRaw(byClass[excursionDown][0], "b_tick"), ShouldEqual, 10)
				So(metricRaw(byClass[excursionDown][0], "c_tick"), ShouldEqual, 13)
			})
		})
	})
}

func TestDetector_ClearGainCacheIsFeeKeyed(t *testing.T) {
	Convey("Given a tape whose trough-to-102 move clears a 0.26% taker fee", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		priceMgr := testPrice(ctx, "BTC/USD")
		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)
		detector := NewDetector(ctx, storeTee, priceMgr)

		point := func(tick int64, p float64) tapePoint {
			return tapePoint{idx: tick, tick: tick, price: decimal.NewFromFloat64(p)}
		}

		tp := &tape{detector: detector, symbol: "BTC/USD"}
		tp.trough, tp.peak = point(1, 100), point(1, 100)

		So(tp.rise(point(2, 102)), ShouldBeNil)
		So(tp.clearGain, ShouldNotBeNil)
		So(tp.near.c.price, ShouldBeNil)

		Convey("With the fee unchanged, a larger gain is not re-priced and is not a near miss", func() {
			So(tp.rise(point(3, 103)), ShouldBeNil)
			So(tp.clearGain, ShouldNotBeNil)
			So(tp.near.c.price, ShouldBeNil)
		})

		Convey("After a fee rise to 1.5%, the cached threshold is dropped and 103 is re-priced", func() {
			priceMgr.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(1.5)})

			So(tp.rise(point(3, 103)), ShouldBeNil)
			So(tp.clearGain, ShouldBeNil)
			So(tp.near.c.price, ShouldNotBeNil)
			So(tp.near.c.price.Cmp(decimal.NewFromFloat64(103)), ShouldEqual, 0)
		})
	})
}
