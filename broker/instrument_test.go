package broker

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
seedClient fails the first Read (soft disconnect), then yields a snapshot after
resubscribe — mirrors Kraken 1013 during instrument seed.
*/
type seedClient struct {
	reads atomic.Int32
	frame []byte
}

func (client *seedClient) Read(...string) ([]byte, error) {
	if client.reads.Add(1) == 1 {
		return nil, errors.New("websocket: close 1013: Market data unavailable")
	}

	return client.frame, nil
}

func (client *seedClient) Write([]byte) error { return nil }

func (client *seedClient) Post(string, any) ([]byte, error) { return nil, nil }

func (client *seedClient) Watch(string, any, json.Unmarshaler) {}

func TestSeedSnapshotRetriesSoftDisconnect(t *testing.T) {
	Convey("Soft disconnect during seed re-subscribes and still arms", t, func() {
		costMin, _ := decimal.NewFromString("1.0")
		snapshot := &kraken.Instrument{
			Channel: "instrument",
			Type:    "snapshot",
		}
		snapshot.Data.Pairs = []kraken.InstrumentPair{{
			Symbol:  "BTC/USD",
			Base:    "BTC",
			Quote:   "USD",
			Status:  "online",
			CostMin: &costMin,
		}}

		frame, err := sonic.Marshal(snapshot)
		So(err, ShouldBeNil)

		public := &seedClient{frame: frame}
		instrument := &Instrument{
			System: runtime.NewSystem(context.Background(), "instrument"),
			public: public,
			cache:  &sync.Map{},
			quote:  "USD",
		}

		So(instrument.seedSnapshot([]byte(`{"method":"subscribe"}`)), ShouldBeNil)
		So(public.reads.Load(), ShouldEqual, 2)
		So(instrument.Has("BTC/USD"), ShouldBeTrue)
	})
}

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
			QtyIncrement:       &qtyInc,
			PricePrecision:     1,
			CostPrecision:      2,
			Marginable:         true,
			HasIndex:           true,
			CostMin:            &costMin,
			MarginInitial:      &marginInit,
			PositionLimitLong:  100,
			PositionLimitShort: 50,
			TickSize:           &tickSize,
			PriceIncrement:     &priceInc,
			QtyMin:             &qtyMin,
		}

		Convey("sonic.Marshal round-trips the pair", func() {
			raw, err := sonic.Marshal(original)
			So(err, ShouldBeNil)

			var decoded kraken.InstrumentPair
			So(sonic.Unmarshal(raw, &decoded), ShouldBeNil)
			So(decoded.Symbol, ShouldEqual, "BTC/USD")
			So(decoded.Base, ShouldEqual, "BTC")
			So(decoded.Quote, ShouldEqual, "USD")
			So(decoded.Status, ShouldEqual, "online")
			So(decoded.QtyPrecision, ShouldEqual, 5)
			So(decoded.Marginable, ShouldBeTrue)
			So(decoded.HasIndex, ShouldBeTrue)
			So(decoded.PositionLimitLong, ShouldEqual, 100)
			So(decoded.CostMin.String(), ShouldEqual, costMin.String())
			So(decoded.QtyMin.String(), ShouldEqual, qtyMin.String())
			So(decoded.TickSize.String(), ShouldEqual, tickSize.String())
		})
	})
}

func TestInstrumentSnapshotWithPeers(t *testing.T) {
	Convey("Given a kraken instrument snapshot with pairs", t, func() {
		costMin, _ := decimal.NewFromString("1.0")

		pair1 := kraken.InstrumentPair{
			Symbol:  "BTC/USD",
			Base:    "BTC",
			Quote:   "USD",
			Status:  "online",
			CostMin: &costMin,
		}
		pair2 := kraken.InstrumentPair{
			Symbol:  "ETH/USD",
			Base:    "ETH",
			Quote:   "USD",
			Status:  "online",
			CostMin: &costMin,
		}

		snapshot := &kraken.Instrument{
			Channel: "instrument",
			Type:    "snapshot",
		}
		snapshot.Data.Pairs = []kraken.InstrumentPair{pair1, pair2}

		raw, err := sonic.Marshal(snapshot)
		So(err, ShouldBeNil)

		Convey("the snapshot JSON carries the pairs", func() {
			var decoded kraken.Instrument
			So(sonic.Unmarshal(raw, &decoded), ShouldBeNil)
			So(decoded.Channel, ShouldEqual, "instrument")
			So(decoded.Type, ShouldEqual, "snapshot")
			So(decoded.Data.Pairs, ShouldHaveLength, 2)
			So(decoded.Data.Pairs[0].Symbol, ShouldEqual, "BTC/USD")
			So(decoded.Data.Pairs[1].Symbol, ShouldEqual, "ETH/USD")
			So(decoded.Data.Pairs[0].CostMin.String(), ShouldEqual, costMin.String())
		})
	})
}

func TestLevel3FrameSize(t *testing.T) {
	Convey("Given Kraken's standard level3 subscription-rate budget", t, func() {
		prev := system.Cfg.Market.Level3.RateLimit
		system.Cfg.Market.Level3.RateLimit = 200
		Reset(func() { system.Cfg.Market.Level3.RateLimit = prev })

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
