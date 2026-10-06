package strategy

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/tests/market"
)

func metricRaw(measurement *data.Measurement, key string) float64 {
	return data.Pull(measurement.Read(key)).Metric.Raw
}

func tradeWithPrice(epoch int64, label string, tick int64, exact *decimal.Decimal) *data.Measurement {
	trade := data.NewMeasurement(epoch, label, "spot:trade", tick, tick)
	return trade.Write(
		data.NewExactMetric("price", exact, data.UnitPrice, data.TimescaleTick),
	)
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

func TestDetector_Scan(t *testing.T) {
	Convey("Given a detector storing into a ready store tee", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)
		detector := NewDetector(ctx, storeTee, nil)

		Convey("When the tape's highest price comes before its lowest price (adversarial wick breakdown)", func() {
			tape := tradeTape(
				t,
				100,
				market.NewDownwardBreakdownTape("BTC/USD", 60000, 10),
				market.NewAdversarialMultiWickTape("BTC/USD", 57000, 10),
			)

			detector.Scan(func(yield func(*data.Measurement) bool) {
				for _, trade := range tape {
					if !yield(trade) {
						return
					}
				}
			})

			Convey("It yields the exact maximum low-before-high excursion confirmed by the brute-force oracle", func() {
				So(storeTee.Pending(), ShouldEqual, 1)

				detection := popMeasurement(storeTee)
				So(detection, ShouldNotBeNil)

				low, high := drawUp(tape)
				So(metricRaw(detection, "low_tick"), ShouldEqual, float64(low))
				So(metricRaw(detection, "high_tick"), ShouldEqual, float64(high))
				So(metricRaw(detection, "high_price"), ShouldBeGreaterThan, metricRaw(detection, "low_price"))
			})
		})

		Convey("When a stop-loss hunter wicks down to a new low after an earlier macro run", func() {
			// Early macro run: 100 -> 250 (+150% gain) across ticks 1 to 6.
			// Then at tick 15, a stoploss hunter plunges to 40 (new lower trough).
			// From 40, it only weakly recovers to 50 (+25% gain).
			prices := []string{
				"100", "110", "130", "160", "200", "250", "240", "220", "200", "180",
				"150", "120", "80", "50", "40", "42", "45", "48", "50", "49",
			}

			tape := make([]*data.Measurement, len(prices))
			for i, pStr := range prices {
				exact, pErr := decimal.NewFromString(pStr)
				So(pErr, ShouldBeNil)

				tape[i] = tradeWithPrice(100, "BTC/USD", int64(i+1), exact)
			}

			detector.Scan(func(yield func(*data.Measurement) bool) {
				for _, trade := range tape {
					if !yield(trade) {
						return
					}
				}
			})

			Convey("The earlier +150% macro excursion is preserved and NOT destroyed by the stop-loss hunt", func() {
				So(storeTee.Pending(), ShouldEqual, 1)

				detection := popMeasurement(storeTee)
				So(detection, ShouldNotBeNil)

				// Must choose the 100 -> 250 run, NOT the 40 -> 50 bounce!
				So(metricRaw(detection, "low_price"), ShouldEqual, 100)
				So(metricRaw(detection, "high_price"), ShouldEqual, 250)
				So(metricRaw(detection, "low_tick"), ShouldEqual, 1)
				So(metricRaw(detection, "high_tick"), ShouldEqual, 6)
			})
		})

		Convey("When a larger macro excursion follows a stop-loss wick", func() {
			// Small early run: 100 -> 120 (+20%).
			// Then stop-loss wick drops to 50 (new trough).
			// Then massive macro run from that wick: 50 -> 250 (+400%)!
			prices := []string{
				"100", "110", "120", "90", "70", "50", "80", "120", "180", "250",
			}

			tape := make([]*data.Measurement, len(prices))
			for i, pStr := range prices {
				exact, pErr := decimal.NewFromString(pStr)
				So(pErr, ShouldBeNil)

				tape[i] = tradeWithPrice(100, "BTC/USD", int64(i+1), exact)
			}

			detector.Scan(func(yield func(*data.Measurement) bool) {
				for _, trade := range tape {
					if !yield(trade) {
						return
					}
				}
			})

			Convey("The +400% run starting from the trough is correctly detected", func() {
				So(storeTee.Pending(), ShouldEqual, 1)

				detection := popMeasurement(storeTee)
				So(detection, ShouldNotBeNil)

				So(metricRaw(detection, "low_price"), ShouldEqual, 50)
				So(metricRaw(detection, "high_price"), ShouldEqual, 250)
				So(metricRaw(detection, "low_tick"), ShouldEqual, 6)
				So(metricRaw(detection, "high_tick"), ShouldEqual, 10)
			})
		})

		Convey("When a multi-candle macro excursion has intermediate pullbacks (SWEAT/USD style)", func() {
			// Trough at 0.00042 -> pullbacks at 0.00063 and 0.00075 -> blowoff peak at 0.00105 (+150%)
			// -> crash to 0.00065
			prices := []string{
				"0.00042", "0.00050", "0.00065", "0.00085", "0.00070", "0.00063",
				"0.00078", "0.00095", "0.00082", "0.00075", "0.00090", "0.00105",
				"0.00092", "0.00078", "0.00065",
			}

			tape := make([]*data.Measurement, len(prices))
			for i, pStr := range prices {
				exact, pErr := decimal.NewFromString(pStr)
				So(pErr, ShouldBeNil)

				tape[i] = tradeWithPrice(100, "SWEAT/USD", int64(i+1), exact)
			}

			detector.Scan(func(yield func(*data.Measurement) bool) {
				for _, trade := range tape {
					if !yield(trade) {
						return
					}
				}
			})

			Convey("The full multi-candle excursion from 0.00042 to 0.00105 is captured", func() {
				So(storeTee.Pending(), ShouldEqual, 1)

				detection := popMeasurement(storeTee)
				So(detection, ShouldNotBeNil)

				So(detection.Label, ShouldEqual, "SWEAT/USD")
				So(metricRaw(detection, "low_price"), ShouldEqual, 0.00042)
				So(metricRaw(detection, "high_price"), ShouldEqual, 0.00105)
				So(metricRaw(detection, "low_tick"), ShouldEqual, 1)
				So(metricRaw(detection, "high_tick"), ShouldEqual, 12)
			})
		})

		Convey("When multiple symbols and epochs stream through sequentially", func() {
			symbols := []string{"BTC/USD", "ETH/USD"}
			epochs := []int64{100, 101}

			var multiTape []*data.Measurement
			tick := int64(1)

			for _, ep := range epochs {
				for _, sym := range symbols {
					// 50 -> 100 run for each symbol and epoch
					for _, pStr := range []string{"60", "50", "70", "90", "100", "80"} {
						exact, _ := decimal.NewFromString(pStr)
						multiTape = append(multiTape, tradeWithPrice(ep, sym, tick, exact))
						tick++
					}
				}
			}

			detector.Scan(func(yield func(*data.Measurement) bool) {
				for _, trade := range multiTape {
					if !yield(trade) {
						return
					}
				}
			})

			Convey("All 4 independent symbol-epoch macro excursions are flushed", func() {
				So(storeTee.Pending(), ShouldEqual, 4)

				for i := 0; i < 4; i++ {
					det := popMeasurement(storeTee)
					So(det, ShouldNotBeNil)
					So(metricRaw(det, "low_price"), ShouldEqual, 50)
					So(metricRaw(det, "high_price"), ShouldEqual, 100)
				}
			})
		})

		Convey("When a tape is monotonically downward with no rallies", func() {
			prices := []string{"100", "95", "90", "85", "80", "70", "60"}
			tape := make([]*data.Measurement, len(prices))
			for i, pStr := range prices {
				exact, _ := decimal.NewFromString(pStr)
				tape[i] = tradeWithPrice(100, "BTC/USD", int64(i+1), exact)
			}

			detector.Scan(func(yield func(*data.Measurement) bool) {
				for _, trade := range tape {
					if !yield(trade) {
						return
					}
				}
			})

			Convey("Zero false excursions are flushed", func() {
				So(storeTee.Pending(), ShouldEqual, 0)
			})
		})

		Convey("When a trade carries no exact price", func() {
			trade := data.NewMeasurement(100, "BTC/USD", "spot:trade", 1, 1)
			trade.Write(data.NewMetric("price", 60000, data.UnitPrice, data.TimescaleTick))

			detector.Scan(func(yield func(*data.Measurement) bool) {
				yield(trade)
			})

			Convey("It is skipped instead of producing an invalid excursion", func() {
				So(storeTee.Pending(), ShouldEqual, 0)
			})
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

		Convey("When a tape contains only sub-friction noise (+0.15% gain against 0.52% roundtrip fees)", func() {
			tape := tradeTape(
				t,
				100,
				market.NewUnprofitableUpperTape("BTC/USD", 60000, 10),
			)

			detector.Scan(func(yield func(*data.Measurement) bool) {
				for _, trade := range tape {
					if !yield(trade) {
						return
					}
				}
			})

			Convey("The excursion is rejected by clearFriction and nothing is flushed", func() {
				So(storeTee.Pending(), ShouldEqual, 0)
			})
		})

		Convey("When a tape contains a true profitable macro breakout (+3.5% gain clearing friction)", func() {
			tape := tradeTape(
				t,
				100,
				market.NewProfitableUpperTape("BTC/USD", 60000, 10),
			)

			detector.Scan(func(yield func(*data.Measurement) bool) {
				for _, trade := range tape {
					if !yield(trade) {
						return
					}
				}
			})

			Convey("The excursion comfortably clears roundtrip friction and is flushed to storage", func() {
				So(storeTee.Pending(), ShouldEqual, 1)

				detection := popMeasurement(storeTee)
				So(detection, ShouldNotBeNil)
				So(detection.Label, ShouldEqual, "BTC/USD")
				So(detection.Epoch, ShouldEqual, 100)
				So(metricRaw(detection, "high_price"), ShouldBeGreaterThan, metricRaw(detection, "low_price"))
			})
		})

		Convey("When a tape contains oscillating chop whipsaws within the spread", func() {
			tape := tradeTape(
				t,
				100,
				market.NewChopWhipsawTape("BTC/USD", 60000, 10, 50),
			)

			detector.Scan(func(yield func(*data.Measurement) bool) {
				for _, trade := range tape {
					if !yield(trade) {
						return
					}
				}
			})

			Convey("Chop within the friction deadband is never flushed as an excursion", func() {
				So(storeTee.Pending(), ShouldEqual, 0)
			})
		})
	})
}
