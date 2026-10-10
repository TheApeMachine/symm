package cvd

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
TestSignalStepVolumeBarWindows drives the real Signal.Step boundary. Flow
totals are windowed on the volume clock pumpdump uses (bar target = median
prior trade quantity, fixed at open): they appear once per closed bar, hold
only that bar's trades, and never accumulate since an epoch. Epochs and
symbols keep separate clocks.
*/
func TestSignalStepVolumeBarWindows(t *testing.T) {
	Convey("Given a tape across two symbols and two epochs", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		signal := NewSignal(ctx)
		signal.Transition(runtime.READY)

		observations := []struct {
			symbol          string
			epoch           int64
			at              int64
			side            string
			price, quantity float64
		}{
			// BTC@1 seeds Q on 10; the bar opens at 120 on Q* = 10.
			{"BTC/USD", 1, 100, "buy", 100, 10},
			{"ETH/USD", 1, 110, "buy", 50, 3},
			{"BTC/USD", 1, 120, "sell", 110, 3},
			// Another epoch's trade never joins epoch 1's bar.
			{"BTC/USD", 2, 130, "buy", 200, 2},
			{"BTC/USD", 1, 140, "buy", 90, 5},
			// ETH opens its own bar on Q* = 3 and does not fill it.
			{"ETH/USD", 1, 150, "sell", 75, 2},
			// 3 + 5 + 14 >= 10: the BTC@1 bar [120, 160] closes.
			{"BTC/USD", 1, 160, "sell", 80, 14},
			// The next bar opens at 160 on Q* = median{10, 3, 5, 14} = 7.5
			// and closes on this trade alone.
			{"BTC/USD", 1, 170, "buy", 100, 8},
		}

		results := make([]*data.Measurement, 0, len(observations))

		for index, observation := range observations {
			prior := data.NewMeasurement(
				observation.epoch, observation.symbol, "spot:trade",
				int64(index+1), int64(index+1),
				&data.StringEntry{Key: "side", Value: observation.side},
			)
			prior.At = time.Unix(observation.at, 0).UTC()
			prior.From = prior.At
			prior.Write(
				data.NewMetric("price", observation.price, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("qty", observation.quantity, data.UnitQuantity, data.TimescaleInstantaneous),
			)

			result := signal.Step(prior)
			So(result, ShouldNotBeNil)
			results = append(results, result)
		}

		read := func(result *data.Measurement, label string) (float64, bool) {
			entry := data.Pull(result.Read(label))

			if entry == nil || entry.Err != nil || entry.Metric == nil {
				return 0, false
			}

			return entry.Metric.Raw, true
		}

		Convey("No frame before a bar closes carries flow totals", func() {
			for index := range 6 {
				for _, label := range []string{"trade_count", "cumulative_volume_delta", "signed_net_fraction", "cvd_epoch_from"} {
					_, held := read(results[index], label)
					So(held, ShouldBeFalse)
				}
			}
		})

		Convey("A closed bar holds exactly its own trades", func() {
			closed := results[6]
			want := map[string]float64{
				"trade_count":               3,
				"trade_count:buy":           1,
				"trade_count:sell":          2,
				"executed_quantity:buy":     5,
				"executed_quantity:sell":    17,
				"gross_executed_quantity":   22,
				"net_executed_quantity":     -12,
				"cumulative_volume_delta":   -12,
				"aggressive_notional:buy":   450,
				"aggressive_notional:sell":  1450,
				"gross_notional":            1900,
				"net_notional":              -1000,
				"cumulative_notional_delta": -1000,
				"signed_net_fraction":       -1000.0 / 1900.0,
				"signed_count_fraction":     -1.0 / 3.0,
				"mean_trade_notional":       1900.0 / 3.0,
			}

			for label, value := range want {
				got, held := read(closed, label)
				So(held, ShouldBeTrue)
				So(got, ShouldAlmostEqual, value, 1e-12)
			}

			So(closed.From, ShouldEqual, time.Unix(120, 0).UTC())
		})

		Convey("The next bar starts empty instead of continuing a running total", func() {
			next := results[7]
			count, _ := read(next, "trade_count")
			delta, _ := read(next, "cumulative_volume_delta")
			fraction, _ := read(next, "signed_net_fraction")
			So(count, ShouldEqual, 1)
			So(delta, ShouldEqual, 8)
			So(fraction, ShouldEqual, 1)
			So(next.From, ShouldEqual, time.Unix(160, 0).UTC())
		})
	})
}
