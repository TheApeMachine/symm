package trader

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

func TestNewCrypto(t *testing.T) {
	Convey("Given NewCrypto wiring", t, func() {
		ctx := context.Background()
		booter := system.NewBooter(ctx, nil)
		analyzer := logic.NewAnalyzer(booter, nil)
		planner := strategy.NewPlanner(ctx, nil, nil, analyzer)

		Convey("When the runtime is constructed", func() {
			crypto, err := NewCrypto(
				ctx,
				booter,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				analyzer,
				planner,
			)

			Convey("Then it is ready to start", func() {
				So(err, ShouldBeNil)
				So(crypto, ShouldNotBeNil)
				So(crypto.tickBudget, ShouldEqual, 10*time.Millisecond)
			})
		})
	})
}

func TestCryptoRun(t *testing.T) {
	Convey("Given a started crypto runtime", t, func() {
		ctx := context.Background()
		channel := make(chan []byte, 8)
		booter := system.NewBooter(ctx, channel)
		analyzer := logic.NewAnalyzer(booter, channel)
		planner := strategy.NewPlanner(ctx, channel, nil, analyzer)

		crypto, err := NewCrypto(
			ctx,
			booter,
			nil,
			nil,
			nil,
			nil,
			channel,
			nil,
			analyzer,
			planner,
		)
		So(err, ShouldBeNil)

		booter.AddStages(
			system.NewStage(system.StagePreflight),
			system.NewStage(system.StageWarmup, crypto),
		)

		So(crypto.Run(), ShouldBeNil)

		Convey("When one tick elapses", func() {
			time.Sleep(15 * time.Millisecond)

			Convey("Then the runtime reports ready and publishes a tick", func() {
				So(crypto.Status(), ShouldEqual, types.READY)
				So(crypto.tick.Load(), ShouldBeGreaterThan, 0)

				select {
				case frame := <-channel:
					So(string(frame), ShouldContainSubstring, `"tick"`)
				default:
					So("tick frame", ShouldEqual, "published")
				}
			})
		})
	})
}
