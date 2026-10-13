package runtime

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/system"
)

type dummyNode struct {
	lastPrior *data.Measurement
}

func (node *dummyNode) Step(prior *data.Measurement) *data.Measurement {
	node.lastPrior = prior
	if prior == nil {
		return nil
	}

	return prior.Next(prior, map[string]float64{"dummy": 1.0})
}

func TestWorkspaceTickAdvancement(t *testing.T) {
	Convey("Given a configured Workspace", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		node := &dummyNode{}
		workspace := NewWorkspace(
			ctx,
			1,
			"test_workspace",
			[][]Node{{node}},
		)
		workspace.Transition(READY)

		Convey("the workspace defaults to an 8192 capacity ring buffer", func() {
			So(workspace.capacity, ShouldEqual, 8192)
			So(workspace.mask, ShouldEqual, 8191)
		})

		Convey("measurements stamped with Tick at source are preserved and track Progress", func() {
			initialTick := system.Tick.Load()
			now := time.Now().UTC()

			m1 := data.NewMeasurement(1, "BTC/USD", "spot:trade", now, now)
			m1.Write(data.NewMetric("price", 50000, data.UnitPrice, data.TimescaleInstantaneous))

			workspace.Step(m1)

			So(m1.Tick, ShouldEqual, initialTick+1)
			So(system.Tick.Load(), ShouldEqual, initialTick+1)

			tickProgress, atProgress := workspace.Progress()
			So(tickProgress, ShouldEqual, initialTick+1)
			So(atProgress, ShouldEqual, now.UnixNano())

			now2 := now.Add(time.Second)
			m2 := data.NewMeasurement(1, "BTC/USD", "spot:trade", now2, now2)
			m2.Write(data.NewMetric("price", 50010, data.UnitPrice, data.TimescaleInstantaneous))

			workspace.Step(m2)

			So(m2.Tick, ShouldEqual, initialTick+2)
			So(system.Tick.Load(), ShouldEqual, initialTick+2)

			tickProgress2, atProgress2 := workspace.Progress()
			So(tickProgress2, ShouldEqual, initialTick+2)
			So(atProgress2, ShouldEqual, now2.UnixNano())
		})
	})
}
