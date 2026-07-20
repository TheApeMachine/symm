package trader

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests/mockapi"
)

/*
TestMarketWaitDirtyCadenceUnderBurst proves WaitDirty + Cut keep pace with a
Simulator-fed ticker burst. Atomic Capture may merge concurrent observations,
but the consumer must not impose an additional quiet period on every Cut.
ponytail: the fixed producer interval, observation window, and ratio are
scheduling-sensitive; a deterministic clock and event-count driver is the
upgrade path.
*/
func TestMarketWaitDirtyCadenceUnderBurst(t *testing.T) {
	Convey("Given a Market on Paper Simulator with a short tick budget", t, func() {
		previousBudget := viper.Get("cognitive.tick_budget")
		previousTimeline := viper.Get("signals.feed_timeline_capacity")
		previousTrack := viper.Get("signals.feed_track_capacity")
		t.Cleanup(func() {
			viper.Set("cognitive.tick_budget", previousBudget)
			viper.Set("signals.feed_timeline_capacity", previousTimeline)
			viper.Set("signals.feed_track_capacity", previousTrack)
		})
		viper.Set("cognitive.tick_budget", 10*time.Millisecond)
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
		var produced atomic.Int64
		var producer sync.WaitGroup
		var stopOnce sync.Once
		producer.Add(1)
		stopProducer := func() {
			stopOnce.Do(func() { close(stop) })
			producer.Wait()
		}
		t.Cleanup(stopProducer)

		go func() {
			defer producer.Done()
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
					produced.Add(1)
				}
			}
		}()

		started := time.Now()
		cuts := 0

		for time.Since(started) < 200*time.Millisecond {
			market.WaitDirty(10 * time.Millisecond)
			cutAt := time.Now().UTC()
			frame, cutErr := market.Cut(cutAt)
			So(cutErr, ShouldBeNil)
			So(frame.At, ShouldEqual, cutAt)

			if frame != nil && !frame.IsEmpty() {
				cuts++
			}
		}

		stopProducer()
		_, drainErr := market.Cut(time.Now().UTC())
		So(drainErr, ShouldBeNil)
		noProgressAt := time.Now().UTC()
		noProgress, noProgressErr := market.Cut(noProgressAt)
		So(noProgressErr, ShouldBeNil)

		Convey("Then Cuts keep pace without a per-observation delay", func() {
			So(cuts, ShouldBeGreaterThan, int(produced.Load()/2))
			So(noProgress.IsEmpty(), ShouldBeTrue)
			So(noProgress.At, ShouldEqual, noProgressAt)
		})
	})
}
