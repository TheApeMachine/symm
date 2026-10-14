package strategy

import (
	"iter"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

func newTestDetector(t *testing.T, symbol string) (*Detector, runtime.Tee) {
	ctx := t.Context()
	normalizer := spot.NewNormalizer()
	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"BTC": {AltName: "XBT", Decimals: 8},
			"USD": {AltName: "ZUSD", Decimals: 2},
		},
		NewPairs: map[string]spot.AssetPair{
			symbol: {WSName: symbol, Base: "BTC", Quote: "USD", LotDecimals: 8, LotMultiplier: 1},
		},
	})

	priceSystem := broker.NewPrice(ctx, nil, nil, nil, normalizer)
	priceSystem.SetFee(symbol, kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
	priceSystem.SetReferenceCash(decimal.NewFromFloat64(200))

	storeTee := hindsight.NewStoreTee(ctx, "detector_test")
	storeTee.Transition(runtime.READY)

	detector := NewDetector(ctx, storeTee, priceSystem)
	return detector, storeTee
}

func makeTrade(
	symbol string,
	sequenceIndex int64,
	tickIndex int64,
	atTime time.Time,
	tradePrice float64,
	bidQuote *float64,
	askQuote *float64,
) *data.Measurement {
	measurement := data.NewMeasurement(
		1, symbol, "spot:trade", atTime, atTime,
		&data.StringEntry{Key: "type", Value: "trade"},
		&data.StringEntry{Key: "side", Value: "buy"},
	)
	measurement.SeqIdx = sequenceIndex
	measurement.Tick = tickIndex

	priceUnit, priceTime := data.GetDimension("spot:trade", "price")
	qtyUnit, qtyTime := data.GetDimension("spot:trade", "qty")

	metrics := []*data.Metric{
		data.NewExactMetric("price", decimal.NewFromFloat64(tradePrice), priceUnit, priceTime),
		data.NewMetric("qty", 1.0, qtyUnit, qtyTime),
	}

	if bidQuote != nil {
		bidUnit, bidTime := data.GetDimension("spot:trade", "bid_price_0")
		metrics = append(metrics, data.NewExactMetric("bid_price_0", decimal.NewFromFloat64(*bidQuote), bidUnit, bidTime))
	}

	if askQuote != nil {
		askUnit, askTime := data.GetDimension("spot:trade", "ask_price_0")
		metrics = append(metrics, data.NewExactMetric("ask_price_0", decimal.NewFromFloat64(*askQuote), askUnit, askTime))
	}

	return measurement.Write(metrics...)
}

func makeTradeWithSnapshot(
	symbol string,
	sequenceIndex int64,
	tickIndex int64,
	atTime time.Time,
	tradePrice float64,
	bids []broker.Level,
	asks []broker.Level,
) *data.Measurement {
	measurement := data.NewMeasurement(
		1, symbol, "spot:trade", atTime, atTime,
		&data.StringEntry{Key: "type", Value: "trade"},
		&data.StringEntry{Key: "side", Value: "buy"},
	)
	measurement.SeqIdx = sequenceIndex
	measurement.Tick = tickIndex

	priceUnit, priceTime := data.GetDimension("spot:trade", "price")
	qtyUnit, qtyTime := data.GetDimension("spot:trade", "qty")

	metrics := []*data.Metric{
		data.NewExactMetric("price", decimal.NewFromFloat64(tradePrice), priceUnit, priceTime),
		data.NewMetric("qty", 1.0, qtyUnit, qtyTime),
	}

	snapshot := &broker.Snapshot{
		Depth: 10,
		Bids:  bids,
		Asks:  asks,
	}
	metrics = append(metrics, snapshot.Metrics()...)

	return measurement.Write(metrics...)
}

func measurementStream(measurements []*data.Measurement) iter.Seq2[*data.Measurement, error] {
	return func(yield func(*data.Measurement, error) bool) {
		for _, measurement := range measurements {
			if !yield(measurement, nil) {
				return
			}
		}
	}
}

func floatPtr(value float64) *float64 {
	return &value
}

func TestDetector_Scan(t *testing.T) {
	system.NewConfig()
	symbol := "BTC/USD"
	startTime := time.Unix(1_700_000_000, 0).UTC()

	Convey("Given a Detector with round-trip friction", t, func() {
		detector, storeTee := newTestDetector(t, symbol)

		Convey("Wide-spread bid-ask bounce is not mislabeled as an up excursion", func() {
			var tapeTrades []*data.Measurement

			for tradeStep := 0; tradeStep < 6; tradeStep++ {
				var printPrice float64
				if tradeStep%2 == 0 {
					printPrice = 100.0
				}
				if tradeStep%2 != 0 {
					printPrice = 110.0
				}

				tapeTrades = append(tapeTrades, makeTrade(
					symbol,
					int64(tradeStep+1),
					int64(tradeStep+1),
					startTime.Add(time.Duration(tradeStep)*time.Second),
					printPrice,
					floatPtr(100.0),
					floatPtr(110.0),
				))
			}

			err := detector.Scan(measurementStream(tapeTrades))
			So(err, ShouldBeNil)

			var emittedClasses []string
			for storeTee.Pending() > 0 {
				incoming := storeTee.Next()
				pushed := *(**data.Measurement)(incoming)
				typeEntry := data.Pull(pushed.Meta("type"))
				if typeEntry != nil {
					emittedClasses = append(emittedClasses, typeEntry.Value)
				}
			}

			for _, class := range emittedClasses {
				So(class, ShouldNotEqual, excursionUp)
			}
		})

		Convey("Executable price rise in wide spread qualifies as excursionUp after confirming retrace", func() {
			var tapeTrades []*data.Measurement

			tapeTrades = append(tapeTrades, makeTrade(
				symbol, 1, 1, startTime, 100.0, floatPtr(100.0), floatPtr(110.0),
			))

			tapeTrades = append(tapeTrades, makeTrade(
				symbol, 2, 2, startTime.Add(time.Second), 105.0, floatPtr(105.0), floatPtr(115.0),
			))

			tapeTrades = append(tapeTrades, makeTrade(
				symbol, 3, 3, startTime.Add(2*time.Second), 135.0, floatPtr(125.0), floatPtr(135.0),
			))

			tapeTrades = append(tapeTrades, makeTrade(
				symbol, 4, 4, startTime.Add(3*time.Second), 95.0, floatPtr(90.0), floatPtr(100.0),
			))

			tapeTrades = append(tapeTrades, makeTrade(
				symbol, 5, 5, startTime.Add(4*time.Second), 95.0, floatPtr(90.0), floatPtr(100.0),
			))

			err := detector.Scan(measurementStream(tapeTrades))
			So(err, ShouldBeNil)

			var emittedClasses []string
			var upDetections []*data.Measurement
			for storeTee.Pending() > 0 {
				incoming := storeTee.Next()
				pushed := *(**data.Measurement)(incoming)
				typeEntry := data.Pull(pushed.Meta("type"))
				if typeEntry != nil {
					emittedClasses = append(emittedClasses, typeEntry.Value)
					if typeEntry.Value == excursionUp {
						upDetections = append(upDetections, pushed)
					}
				}
			}

			So(emittedClasses, ShouldContain, excursionUp)
			So(len(upDetections), ShouldEqual, 1)

			upMeas := upDetections[0]
			bPriceMetric := data.Pull(upMeas.Read("b_price"))
			cPriceMetric := data.Pull(upMeas.Read("c_price"))
			So(bPriceMetric, ShouldNotBeNil)
			So(cPriceMetric, ShouldNotBeNil)
			So(bPriceMetric.Metric.Exact().Cmp(decimal.NewFromFloat64(100.0)), ShouldEqual, 0)
			So(cPriceMetric.Metric.Exact().Cmp(decimal.NewFromFloat64(135.0)), ShouldEqual, 0)
		})

		Convey("Unstamped trades without quote evidence do not fake excursions", func() {
			var tapeTrades []*data.Measurement

			tapeTrades = append(tapeTrades, makeTrade(symbol, 1, 1, startTime, 100.0, nil, nil))
			tapeTrades = append(tapeTrades, makeTrade(symbol, 2, 2, startTime.Add(time.Second), 101.0, nil, nil))
			tapeTrades = append(tapeTrades, makeTrade(symbol, 3, 3, startTime.Add(2*time.Second), 115.0, nil, nil))
			tapeTrades = append(tapeTrades, makeTrade(symbol, 4, 4, startTime.Add(3*time.Second), 95.0, nil, nil))
			tapeTrades = append(tapeTrades, makeTrade(symbol, 5, 5, startTime.Add(4*time.Second), 95.0, nil, nil))

			err := detector.Scan(measurementStream(tapeTrades))
			So(err, ShouldBeNil)

			var emittedClasses []string
			for storeTee.Pending() > 0 {
				incoming := storeTee.Next()
				pushed := *(**data.Measurement)(incoming)
				typeEntry := data.Pull(pushed.Meta("type"))
				if typeEntry != nil {
					emittedClasses = append(emittedClasses, typeEntry.Value)
				}
			}

			So(emittedClasses, ShouldNotContain, excursionUp)
		})

		Convey("Peak excursion C pins at peak safe profit rather than thin needle top", func() {
			var tapeTrades []*data.Measurement

			// Trade 1 (entry B): price 100, ample bid and ask depth. Budget is 200, so posQty is ~2.0 BTC.
			bids1 := []broker.Level{
				{Price: decimal.NewFromFloat64(100.0), Quantity: decimal.NewFromFloat64(10.0)},
			}
			asks1 := []broker.Level{
				{Price: decimal.NewFromFloat64(100.0), Quantity: decimal.NewFromFloat64(10.0)},
			}
			tapeTrades = append(tapeTrades, makeTradeWithSnapshot(symbol, 1, 1, startTime, 100.0, bids1, asks1))

			// Trade 2 (fat liquidity, higher price 110): exit bid depth is 10.0, well above posQty (2.0). Safe profit is high.
			bids2 := []broker.Level{
				{Price: decimal.NewFromFloat64(110.0), Quantity: decimal.NewFromFloat64(10.0)},
			}
			asks2 := []broker.Level{
				{Price: decimal.NewFromFloat64(111.0), Quantity: decimal.NewFromFloat64(10.0)},
			}
			tapeTrades = append(tapeTrades, makeTradeWithSnapshot(symbol, 2, 2, startTime.Add(time.Second), 110.0, bids2, asks2))

			// Trade 3 (thin needle top: price prints 120, but bid depth collapsed to 0.5 < posQty (2.0)): Safe profit is 0!
			bids3 := []broker.Level{
				{Price: decimal.NewFromFloat64(120.0), Quantity: decimal.NewFromFloat64(0.5)},
			}
			asks3 := []broker.Level{
				{Price: decimal.NewFromFloat64(121.0), Quantity: decimal.NewFromFloat64(10.0)},
			}
			tapeTrades = append(tapeTrades, makeTradeWithSnapshot(symbol, 3, 3, startTime.Add(2*time.Second), 120.0, bids3, asks3))

			// Trade 4 & 5 (confirming retrace back down to 90): confirms the up leg.
			bids4 := []broker.Level{
				{Price: decimal.NewFromFloat64(90.0), Quantity: decimal.NewFromFloat64(10.0)},
			}
			asks4 := []broker.Level{
				{Price: decimal.NewFromFloat64(91.0), Quantity: decimal.NewFromFloat64(10.0)},
			}
			tapeTrades = append(tapeTrades, makeTradeWithSnapshot(symbol, 4, 4, startTime.Add(3*time.Second), 90.0, bids4, asks4))
			tapeTrades = append(tapeTrades, makeTradeWithSnapshot(symbol, 5, 5, startTime.Add(4*time.Second), 90.0, bids4, asks4))

			err := detector.Scan(measurementStream(tapeTrades))
			So(err, ShouldBeNil)

			var upDetections []*data.Measurement
			for storeTee.Pending() > 0 {
				incoming := storeTee.Next()
				pushed := *(**data.Measurement)(incoming)
				typeEntry := data.Pull(pushed.Meta("type"))
				if typeEntry != nil && typeEntry.Value == excursionUp {
					upDetections = append(upDetections, pushed)
				}
			}

			So(len(upDetections), ShouldEqual, 1)
			cPriceMetric := data.Pull(upDetections[0].Read("c_price"))
			So(cPriceMetric, ShouldNotBeNil)
			// C price must stay pinned at Trade 2's exit bid (110.0), NOT needle top (120.0)
			So(cPriceMetric.Metric.Exact().Cmp(decimal.NewFromFloat64(110.0)), ShouldEqual, 0)
		})
	})
}
