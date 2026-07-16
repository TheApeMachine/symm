package trader

import (
	"context"
	"strings"
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
)

func TestNewCrypto(t *testing.T) {
	previousDepth := viper.Get("market.l3_depth")
	previousInterval := viper.Get("signals.fluid.integration_interval")
	previousCapacity := viper.Get("market.manifold_max_symbols")
	t.Cleanup(func() { viper.Set("market.l3_depth", previousDepth) })
	t.Cleanup(func() { viper.Set("signals.fluid.integration_interval", previousInterval) })
	t.Cleanup(func() { viper.Set("market.manifold_max_symbols", previousCapacity) })
	viper.Set("market.l3_depth", 10)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	viper.Set("market.manifold_max_symbols", 8)

	Convey("Given NewCrypto wiring", t, func() {
		ctx := context.Background()
		booter := system.NewBooter(ctx, nil)
		analyzer, err := logic.NewAnalyzer(ctx, booter, nil, nil, nil)
		So(err, ShouldBeNil)
		planner := strategy.NewPlanner(ctx, nil, nil, analyzer)
		tree := dmt.NewTree(t.TempDir())
		t.Cleanup(func() {
			if err := tree.Close(); err != nil {
				t.Error(err)
			}
		})
		thesis := types.NewThesis(nil)
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
			)

			Convey("Then it is ready to start", func() {
				So(err, ShouldBeNil)
				So(crypto, ShouldNotBeNil)
				So(crypto.tickBudget, ShouldEqual, 10*time.Millisecond)
				So(crypto.tick.Load(), ShouldEqual, 46)
			})
		})
	})
}

func TestCryptoRun(t *testing.T) {
	previousDepth := viper.Get("market.l3_depth")
	previousInterval := viper.Get("signals.fluid.integration_interval")
	previousCapacity := viper.Get("market.manifold_max_symbols")
	t.Cleanup(func() { viper.Set("market.l3_depth", previousDepth) })
	t.Cleanup(func() { viper.Set("signals.fluid.integration_interval", previousInterval) })
	t.Cleanup(func() { viper.Set("market.manifold_max_symbols", previousCapacity) })
	viper.Set("market.l3_depth", 10)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	viper.Set("market.manifold_max_symbols", 8)

	Convey("Given a started crypto runtime", t, func() {
		ctx := context.Background()
		channel := make(chan []byte, 8)
		booter := system.NewBooter(ctx, channel)
		analyzer, err := logic.NewAnalyzer(ctx, booter, nil, nil, nil)
		So(err, ShouldBeNil)
		planner := strategy.NewPlanner(ctx, channel, nil, analyzer)
		tree := dmt.NewTree(t.TempDir())
		t.Cleanup(func() {
			if err := tree.Close(); err != nil {
				t.Error(err)
			}
		})
		thesis := types.NewThesis(channel)
		desk := broker.NewDesk(nil, nil, nil, nil, thesis, channel)

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
		)
		So(err, ShouldBeNil)
		t.Cleanup(func() {
			crypto.cancel()
			<-crypto.done
		})

		booter.AddStages(
			system.NewStage(system.StagePreflight),
			system.NewStage(system.StageWarmup, crypto),
		)

		So(booter.Start(), ShouldBeNil)
		So(crypto.Run(), ShouldBeNil)

		Convey("When one tick elapses", func() {
			time.Sleep(15 * time.Millisecond)

			Convey("Then the runtime reports ready and publishes a tick", func() {
				So(crypto.Status(), ShouldEqual, types.READY)
				So(crypto.tick.Load(), ShouldBeGreaterThan, 0)
				_, persisted := tree.Get([]byte(types.ThesisKey))
				So(persisted, ShouldBeTrue)

				// booter.Start published its own boot-progress frames onto
				// this same channel before crypto.Run started ticking, so
				// a tick frame is somewhere in the backlog, not necessarily
				// first.
				foundTick := false

				for {
					select {
					case frame := <-channel:
						if strings.Contains(string(frame), `"tick"`) {
							foundTick = true
						}
					default:
						So(foundTick, ShouldBeTrue)
						return
					}
				}
			})
		})
	})
}
