package trader

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests/mockapi"
)

/*
TestMarketWaitDirtyCadenceUnderBurst proves WaitDirty + Cut under Simulator-fed
ticker bursts stay well above the ~1Hz freeze regime that followed the Level3
index thrash — many dirty wakes must still produce Cuts faster than 1 per 200ms.
*/
func TestMarketWaitDirtyCadenceUnderBurst(t *testing.T) {
	Convey("Given a Market on Paper Simulator with a short tick budget", t, func() {
		previousBudget := viper.Get("cognitive.tick_budget")
		previousWindow := viper.Get("signals.coalesce_window")
		previousTimeline := viper.Get("signals.feed_timeline_capacity")
		previousTrack := viper.Get("signals.feed_track_capacity")
		t.Cleanup(func() {
			viper.Set("cognitive.tick_budget", previousBudget)
			viper.Set("signals.coalesce_window", previousWindow)
			viper.Set("signals.feed_timeline_capacity", previousTimeline)
			viper.Set("signals.feed_track_capacity", previousTrack)
		})
		viper.Set("cognitive.tick_budget", 10*time.Millisecond)
		viper.Set("signals.coalesce_window", 10*time.Millisecond)
		viper.Set("signals.feed_timeline_capacity", 128)
		viper.Set("signals.feed_track_capacity", 128)

		ctx := context.Background()
		mock := mockapi.NewMockAPI()
		paper := websocket.NewPaper(
			ctx, websocket.NewLatencySimulator(system.NewBooter(ctx, nil)),
		)
		api := websocket.NewAPI(ctx, mock.Public(), mock.Private(), paper)
		market, err := NewMarket(ctx, api, nil)
		So(err, ShouldBeNil)

		stop := make(chan struct{})
		go func() {
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-stop:
					return
				case now := <-ticker.C:
					market.OnTicker([]byte(
						`{"channel":"ticker","type":"update","data":[{` +
							`"symbol":"BTC/USD","bid":"99.5","bid_qty":"1","ask":"100.5",` +
							`"ask_qty":"1","last":"100","volume":"1","vwap":"100",` +
							`"low":"99","high":"101","change":"0","change_pct":"0",` +
							`"timestamp":"` + now.UTC().Format(time.RFC3339Nano) + `"}]}`,
					))
				}
			}
		}()

		started := time.Now()
		cuts := 0

		for time.Since(started) < 200*time.Millisecond {
			market.WaitDirty(10 * time.Millisecond)
			frame, cutErr := market.Cut(time.Now().UTC())
			So(cutErr, ShouldBeNil)

			if frame != nil && !frame.IsEmpty() {
				cuts++
			}
		}

		close(stop)

		Convey("Then Cut cadence stays far above 1Hz", func() {
			So(cuts, ShouldBeGreaterThan, 5)
		})
	})
}
