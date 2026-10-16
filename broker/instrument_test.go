package broker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestLevel3TokenRetry(t *testing.T) {
	floor, ceiling := kraken.TokenRetryFloor, kraken.TokenRetryCeiling
	kraken.TokenRetryFloor, kraken.TokenRetryCeiling = time.Millisecond, 4*time.Millisecond

	defer func() { kraken.TokenRetryFloor, kraken.TokenRetryCeiling = floor, ceiling }()

	Convey("Given a REST endpoint that answers EAPI:Invalid nonce twice, then a token", t, func() {
		instrument := &Instrument{System: runtime.NewSystem(context.Background(), "instrument")}
		calls := 0
		instrument.fetchToken = func() (string, error) {
			calls++

			if calls <= 2 {
				return "", errors.New("EAPI:Invalid nonce")
			}

			return "fresh", nil
		}

		Convey("the reconnect's token fetch retries instead of failing the resubscribe", func() {
			token, err := instrument.level3Token()

			So(err, ShouldBeNil)
			So(token, ShouldEqual, "fresh")
			So(calls, ShouldEqual, 3)

			Convey("and later batches share the fetched token", func() {
				again, err := instrument.level3Token()
				So(err, ShouldBeNil)
				So(again, ShouldEqual, "fresh")
				So(calls, ShouldEqual, 3)
			})
		})

		Convey("a closed instrument stops retrying", func() {
			instrument.fetchToken = func() (string, error) {
				instrument.System.Close()
				return "", errors.New("EAPI:Invalid nonce")
			}

			_, err := instrument.level3Token()
			So(err, ShouldNotBeNil)
		})
	})
}

func TestLevel3FrameSize(t *testing.T) {
	Convey("Given Kraken's level3 snapshot rate budget", t, func() {
		defer viper.Set("market.l3_depth", viper.Get("market.l3_depth"))
		defer viper.Set("market.l3_rate_limit", viper.Get("market.l3_rate_limit"))

		Convey("a standard budget of 200 at depth 10 (cost 5) fits 40 symbols per frame", func() {
			viper.Set("market.l3_rate_limit", 200)
			viper.Set("market.l3_depth", 10)
			size, err := level3FrameSize()
			So(err, ShouldBeNil)
			So(size, ShouldEqual, 40)
		})

		Convey("depth 100 (cost 25) fits 8", func() {
			viper.Set("market.l3_rate_limit", 200)
			viper.Set("market.l3_depth", 100)
			size, err := level3FrameSize()
			So(err, ShouldBeNil)
			So(size, ShouldEqual, 8)
		})

		Convey("an unsupported depth is a startup error", func() {
			viper.Set("market.l3_rate_limit", 200)
			viper.Set("market.l3_depth", 25)
			_, err := level3FrameSize()
			So(err, ShouldNotBeNil)
		})

		Convey("a budget below one symbol's cost is a startup error", func() {
			viper.Set("market.l3_rate_limit", 0)
			viper.Set("market.l3_depth", 10)
			_, err := level3FrameSize()
			So(err, ShouldNotBeNil)
		})
	})
}

func TestLevel3RateWindow(t *testing.T) {
	Convey("Given Kraken's level3 snapshot rate window derivation", t, func() {
		defer viper.Set("market.l3_depth", viper.Get("market.l3_depth"))
		defer viper.Set("market.l3_rate_limit", viper.Get("market.l3_rate_limit"))
		defer viper.Set("market.l3_rate_window", viper.Get("market.l3_rate_window"))

		Convey("a 200 budget at depth 10 (cost 200) derives 4 seconds for 50 units/sec drain", func() {
			viper.Set("market.l3_rate_window", 0)
			viper.Set("market.l3_rate_limit", 200)
			viper.Set("market.l3_depth", 10)
			So(level3RateWindow(), ShouldEqual, 4*time.Second)
		})

		Convey("a 100 budget at depth 10 (cost 100) derives 2 seconds", func() {
			viper.Set("market.l3_rate_window", 0)
			viper.Set("market.l3_rate_limit", 100)
			viper.Set("market.l3_depth", 10)
			So(level3RateWindow(), ShouldEqual, 2*time.Second)
		})

		Convey("an explicit market.l3_rate_window overrides the derivation", func() {
			viper.Set("market.l3_rate_window", 50*time.Millisecond)
			So(level3RateWindow(), ShouldEqual, 50*time.Millisecond)
		})
	})
}

func TestSubscribeLevel3Paced(t *testing.T) {
	Convey("Given a 90-symbol batch and a budget of 40 symbols per window", t, func() {
		defer viper.Set("market.l3_depth", viper.Get("market.l3_depth"))
		defer viper.Set("market.l3_rate_limit", viper.Get("market.l3_rate_limit"))
		defer viper.Set("market.l3_rate_window", viper.Get("market.l3_rate_window"))
		viper.Set("market.l3_rate_limit", 200)
		viper.Set("market.l3_depth", 10)
		viper.Set("market.l3_rate_window", 10*time.Millisecond)

		instrument := &Instrument{System: runtime.NewSystem(context.Background(), "instrument")}
		instrument.fetchToken = func() (string, error) { return "token", nil }

		batch := make([]string, 90)
		for index := range batch {
			batch[index] = "SYM" + string(rune('A'+index%26)) + "/USD"
		}

		var mu sync.Mutex
		var sizes []int
		var depths []int
		var at []time.Time

		write := func(msg []byte) error {
			var frame kraken.Level3Subscription
			if err := sonic.Unmarshal(msg, &frame); err != nil {
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			sizes = append(sizes, len(frame.Params.Symbol))
			depths = append(depths, frame.Params.Depth)
			at = append(at, time.Now())
			return nil
		}

		Convey("it sends frames of at most 40 with explicit depth, one per rate window", func() {
			window := level3RateWindow()
			So(instrument.subscribeLevel3(write, batch), ShouldBeNil)
			So(sizes, ShouldResemble, []int{40, 40, 10})
			So(depths, ShouldResemble, []int{10, 10, 10})
			So(at[1].Sub(at[0]), ShouldBeGreaterThanOrEqualTo, window)
			So(at[2].Sub(at[1]), ShouldBeGreaterThanOrEqualTo, window)

			Convey("and the window is shared: another socket's frame waits for it too", func() {
				start := time.Now()
				So(instrument.subscribeLevel3(write, batch[:1]), ShouldBeNil)
				So(at[3].Sub(at[2]), ShouldBeGreaterThanOrEqualTo, window)
				So(time.Since(start), ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestSubscribeRequiresLevel3Reader(t *testing.T) {
	Convey("Given an instrument without a Level3 reader", t, func() {
		instrument := &Instrument{System: runtime.NewSystem(context.Background(), "instrument")}
		instrument.SetLevel3Stale(func([]string) {})

		Convey("Subscribe refuses before opening any socket", func() {
			So(instrument.Subscribe(), ShouldNotBeNil)
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

			Convey("and NewInstrumentPairFromValue completely reconstructs the original pair", func() {
				restored, err := kraken.NewInstrumentPairFromValue(val)

				So(err, ShouldBeNil)
				So(restored, ShouldNotBeNil)
				So(restored.Symbol, ShouldEqual, original.Symbol)
				So(restored.Base, ShouldEqual, original.Base)
				So(restored.Quote, ShouldEqual, original.Quote)
				So(restored.Status, ShouldEqual, original.Status)
				So(restored.QtyPrecision, ShouldEqual, original.QtyPrecision)
				So(restored.PricePrecision, ShouldEqual, original.PricePrecision)
				So(restored.CostPrecision, ShouldEqual, original.CostPrecision)
				So(restored.Marginable, ShouldEqual, original.Marginable)
				So(restored.HasIndex, ShouldEqual, original.HasIndex)
				So(restored.PositionLimitLong, ShouldEqual, original.PositionLimitLong)
				So(restored.PositionLimitShort, ShouldEqual, original.PositionLimitShort)
				So(restored.CostMin.String(), ShouldEqual, original.CostMin.String())
				So(restored.QtyMin.String(), ShouldEqual, original.QtyMin.String())
				So(restored.QtyIncrement.String(), ShouldEqual, original.QtyIncrement.String())
				So(restored.MarginInitial.String(), ShouldEqual, original.MarginInitial.String())
				So(restored.TickSize.String(), ShouldEqual, original.TickSize.String())
				So(restored.PriceIncrement.String(), ShouldEqual, original.PriceIncrement.String())
			})
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

			Convey("and NewInstrumentFromValue restores the snapshot", func() {
				restored, err := kraken.NewInstrumentFromValue(val)
				So(err, ShouldBeNil)
				So(restored, ShouldNotBeNil)
				So(len(restored.Data.Pairs), ShouldEqual, 2)
				So(restored.Data.Pairs[0].Symbol, ShouldEqual, "BTC/USD")
				So(restored.Data.Pairs[1].Symbol, ShouldEqual, "ETH/USD")
			})
		})
	})
}
