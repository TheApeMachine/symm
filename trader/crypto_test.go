package trader

import (
	"context"
	"runtime"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

func TestNewCrypto(t *testing.T) {
	previousDepth := viper.Get("market.l3_depth")
	previousInterval := viper.Get("signals.fluid.integration_interval")
	previousCapacity := viper.Get("market.manifold_max_symbols")
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	t.Cleanup(func() { viper.Set("market.l3_depth", previousDepth) })
	t.Cleanup(func() { viper.Set("signals.fluid.integration_interval", previousInterval) })
	t.Cleanup(func() { viper.Set("market.manifold_max_symbols", previousCapacity) })
	t.Cleanup(func() { viper.Set("signals.feed_timeline_capacity", previousTimeline) })
	t.Cleanup(func() { viper.Set("signals.feed_track_capacity", previousTrack) })
	viper.Set("market.l3_depth", 10)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	viper.Set("market.manifold_max_symbols", 8)
	viper.Set("signals.feed_timeline_capacity", 128)
	viper.Set("signals.feed_track_capacity", 128)

	Convey("Given NewCrypto wiring", t, func() {
		ctx := context.Background()
		booter := system.NewBooter(ctx, nil)
		analyzer, err := logic.NewAnalyzer(ctx, booter, nil, nil, nil, nil)
		So(err, ShouldBeNil)
		planner := strategy.NewPlanner(ctx, nil, nil, analyzer)
		tree := dmt.NewTree(t.TempDir())
		t.Cleanup(func() {
			if err := tree.Close(); err != nil {
				t.Error(err)
			}
		})
		thesis := types.NewThesis(nil, nil)
		thesis.Tick = 46

		Convey("When the runtime is constructed", func() {
			crypto, err := NewCrypto(
				ctx,
				booter,
				nil,
				nil,
				nil,
				nil,
				nil,
				analyzer,
				planner,
				tree,
				thesis,
				nil,
			)

			Convey("Then it is ready to start", func() {
				So(err, ShouldBeNil)
				So(crypto, ShouldNotBeNil)
				So(crypto.tick.Load(), ShouldEqual, 46)
			})
		})
	})
}

func TestCryptoRun(t *testing.T) {
	previousDepth := viper.Get("market.l3_depth")
	previousInterval := viper.Get("signals.fluid.integration_interval")
	previousCapacity := viper.Get("market.manifold_max_symbols")
	previousDataPath := viper.Get("system.data_path")
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	t.Cleanup(func() { viper.Set("market.l3_depth", previousDepth) })
	t.Cleanup(func() { viper.Set("signals.fluid.integration_interval", previousInterval) })
	t.Cleanup(func() { viper.Set("market.manifold_max_symbols", previousCapacity) })
	t.Cleanup(func() { viper.Set("system.data_path", previousDataPath) })
	t.Cleanup(func() { viper.Set("signals.feed_timeline_capacity", previousTimeline) })
	t.Cleanup(func() { viper.Set("signals.feed_track_capacity", previousTrack) })
	viper.Set("market.l3_depth", 10)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	viper.Set("market.manifold_max_symbols", 8)
	viper.Set("signals.feed_timeline_capacity", 128)
	viper.Set("signals.feed_track_capacity", 128)
	dataPath := t.TempDir()
	viper.Set("system.data_path", dataPath)

	Convey("Given a started crypto runtime", t, func() {
		ctx := context.Background()
		channel := make(chan []byte, 8)
		booter := system.NewBooter(ctx, channel)
		analyzer, err := logic.NewAnalyzer(ctx, booter, nil, nil, nil, nil)
		So(err, ShouldBeNil)
		planner := strategy.NewPlanner(ctx, channel, nil, analyzer)
		tree := dmt.NewTree(t.TempDir())
		t.Cleanup(func() {
			if err := tree.Close(); err != nil {
				t.Error(err)
			}
		})
		thesis := types.NewThesis(channel, nil)
		desk := broker.NewDesk(nil, nil, nil, nil)
		hub, err := ui.NewHub(ctx, nil, nil, thesis, channel)
		So(err, ShouldBeNil)

		crypto, err := NewCrypto(
			ctx,
			booter,
			nil,
			nil,
			nil,
			desk,
			nil,
			analyzer,
			planner,
			tree,
			thesis,
			hub,
		)

		So(err, ShouldBeNil)

		t.Cleanup(func() {
			crypto.cancel()
			<-crypto.ctx.Done()
		})

		booter.AddStages(
			system.NewStage(system.StagePreflight),
			system.NewStage(system.StageWarmup, crypto),
		)

		So(booter.Start(), ShouldBeNil)
		So(crypto.Run(), ShouldBeNil)

		Convey("When no market frame has arrived", func() {
			Convey("Then the runtime should wait without advancing the tick", func() {
				So(crypto.Status(), ShouldEqual, types.READY)
				So(crypto.tick.Load(), ShouldEqual, 0)
			})
		})

		Convey("When one market frame arrives", func() {
			crypto.market.OnTicker([]byte(`{
				"channel":"ticker",
				"type":"update",
				"data":[{
					"symbol":"BTC/USD",
					"bid":"100",
					"ask":"101",
					"last":"100.5",
					"volume":10,
					"timestamp":"2026-07-16T20:00:00Z"
				}]
			}`))
			deadline := time.Now().Add(time.Second)

			for crypto.tick.Load() == 0 && time.Now().Before(deadline) {
				runtime.Gosched()
			}

			Convey("Then the runtime reports ready and advances the tick", func() {
				So(crypto.Status(), ShouldEqual, types.READY)
				So(crypto.tick.Load(), ShouldEqual, 1)
			})
		})
	})
}
