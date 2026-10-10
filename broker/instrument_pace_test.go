package broker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

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

func TestSubscribeLevel3Paced(t *testing.T) {
	Convey("Given a 90-symbol batch and a budget of 40 symbols per window", t, func() {
		defer viper.Set("market.l3_depth", viper.Get("market.l3_depth"))
		defer viper.Set("market.l3_rate_limit", viper.Get("market.l3_rate_limit"))
		viper.Set("market.l3_rate_limit", 200)
		viper.Set("market.l3_depth", 10)

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
			So(instrument.subscribeLevel3(write, batch), ShouldBeNil)
			So(sizes, ShouldResemble, []int{40, 40, 10})
			So(depths, ShouldResemble, []int{10, 10, 10})
			So(at[1].Sub(at[0]), ShouldBeGreaterThanOrEqualTo, level3RateWindow)
			So(at[2].Sub(at[1]), ShouldBeGreaterThanOrEqualTo, level3RateWindow)

			Convey("and the window is shared: another socket's frame waits for it too", func() {
				start := time.Now()
				So(instrument.subscribeLevel3(write, batch[:1]), ShouldBeNil)
				So(at[3].Sub(at[2]), ShouldBeGreaterThanOrEqualTo, level3RateWindow)
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
