package broker

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/kraken"
)

func TestSubscribeRequiresLevel3Reader(t *testing.T) {
	Convey("Given an instrument without a Level3 reader", t, func() {
		instrument := &Instrument{System: runtime.NewSystem(context.Background(), "instrument")}

		Convey("SubscribeBooks refuses before opening any socket", func() {
			So(instrument.SubscribeBooks(Level3{
				Stale: func([]string) {},
			}), ShouldNotBeNil)
		})
	})
}

func TestInstrumentPairMeasurementRoundTrip(t *testing.T) {
	Convey("Given a valid kraken.InstrumentPair", t, func() {
		costMin, _ := decimal.NewFromString("0.5")
		qtyMin, _ := decimal.NewFromString("0.0001")
		qtyInc, _ := decimal.NewFromString("0.00001")
		marginInit, _ := decimal.NewFromString("0.2")
		tickSize, _ := decimal.NewFromString("0.1")
		priceInc, _ := decimal.NewFromString("0.1")

		original := kraken.InstrumentPair{
			Symbol:             "BTC/USD",
			Base:               "BTC",
			Quote:              "USD",
			Status:             "online",
			QtyPrecision:       5,
			QtyIncrement:       qtyInc,
			PricePrecision:     1,
			CostPrecision:      2,
			Marginable:         true,
			HasIndex:           true,
			CostMin:            costMin,
			MarginInitial:      marginInit,
			PositionLimitLong:  100,
			PositionLimitShort: 50,
			TickSize:           tickSize,
			PriceIncrement:     priceInc,
			QtyMin:             qtyMin,
		}

		Convey("ToValue serializes all fields into a Value", func() {
			epoch := int64(123456789)
			val := original.ToValue(epoch)

			So(val, ShouldNotBeNil)
			So(val.Epoch, ShouldEqual, uint64(epoch))
			So(val.Scope, ShouldEqual, "BTC/USD")
			So(val.Role, ShouldEqual, "instrument")
		})
	})
}

func TestInstrumentSnapshotWithPeers(t *testing.T) {
	Convey("Given a kraken instrument snapshot with pairs", t, func() {
		epoch := int64(987654321)
		costMin, _ := decimal.NewFromString("1.0")

		pair1 := kraken.InstrumentPair{
			Symbol:  "BTC/USD",
			Base:    "BTC",
			Quote:   "USD",
			Status:  "online",
			CostMin: costMin,
		}
		pair2 := kraken.InstrumentPair{
			Symbol:  "ETH/USD",
			Base:    "ETH",
			Quote:   "USD",
			Status:  "online",
			CostMin: costMin,
		}

		snapshot := &kraken.Instrument{
			Channel: "instrument",
			Type:    "snapshot",
		}
		snapshot.Data.Pairs = []kraken.InstrumentPair{pair1, pair2}

		val := snapshot.ToValue(epoch)

		Convey("the snapshot value carries serialized pairs", func() {
			So(val, ShouldNotBeNil)
			So(val.Epoch, ShouldEqual, uint64(epoch))
			So(val.Role, ShouldEqual, "snapshot")
			So(val.Scope, ShouldEqual, "instrument")
		})
	})
}

func TestLevel3FrameSize(t *testing.T) {
	Convey("Given Kraken's standard level3 subscription-rate budget", t, func() {
		viper.Set("market.l3_rate_limit", 200)
		defer viper.Set("market.l3_rate_limit", nil)

		Convey("A frame carries only as many symbols as the budget pays for", func() {
			size, err := level3FrameSize(100)
			So(err, ShouldBeNil)
			So(size, ShouldEqual, 8)

			size, err = level3FrameSize(10)
			So(err, ShouldBeNil)
			So(size, ShouldEqual, 40)
		})

		Convey("An unsupported depth is an error", func() {
			_, err := level3FrameSize(25)
			So(err, ShouldNotBeNil)
		})
	})
}
