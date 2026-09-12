package manifold

import (
	"context"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/types"
)

type testViewer struct {
	wants    bool
	manifold *types.Envelope
}

func (v *testViewer) WantsManifold() bool { return v.wants }
func (v *testViewer) PublishManifold(env *types.Envelope) {
	v.manifold = env
}

func TestDatasetAndSolverAdvance(t *testing.T) {
	Convey("Given a dataset and a spot book with orders", t, func() {
		ds := NewDataset()
		bk := spotbook.New()

		// Insert a bid and ask
		bk.Update(&spotbook.UpdateOptions{
			Direction: spotbook.Bid,
			ID:        "bid-1",
			Price:     decimal.NewFromFloat64(50000.0),
			Quantity:  decimal.NewFromFloat64(1.5),
			Timestamp: time.Now(),
		})
		bk.Update(&spotbook.UpdateOptions{
			Direction: spotbook.Bid,
			ID:        "bid-2",
			Price:     decimal.NewFromFloat64(49990.0),
			Quantity:  decimal.NewFromFloat64(2.0),
			Timestamp: time.Now(),
		})
		bk.Update(&spotbook.UpdateOptions{
			Direction: spotbook.Ask,
			ID:        "ask-1",
			Price:     decimal.NewFromFloat64(50010.0),
			Quantity:  decimal.NewFromFloat64(1.0),
			Timestamp: time.Now(),
		})

		var states []*sensorium.State
		for state := range ds.Step("BTC/USD", bk.Bids, bk.Asks, forcingState{}) {
			states = append(states, state)
		}

		So(ds.Error(), ShouldBeNil)
		So(len(states), ShouldEqual, 3)

		Convey("And a solver stepping this state", func() {
			physics := sensorium.NewManifold(8, 8, 8)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			solver := &Solver{
				ctx:     ctx,
				cancel:  cancel,
				physics: physics,
				dataset: ds,
				loaded:  make(map[int64]struct{}),
				dirty:   make(map[string]struct{}),
				wake:    make(chan struct{}, 1),
			}
			defer solver.Close()

			viewer := &testViewer{wants: true}
			solver.SetViewer(viewer)

			batch := collectStates(states)
			So(batch, ShouldNotBeNil)
			So(batch.N, ShouldEqual, 3)

			solver.advanceMu.Lock()
			stepped, err := solver.physics.Step(batch)
			So(err, ShouldBeNil)
			So(stepped, ShouldNotBeNil)
			reading := solver.publishReading(stepped)
			solver.advanceMu.Unlock()

			So(reading, ShouldNotBeNil)
			So(reading.Reading.CoherenceMag2, ShouldBeGreaterThanOrEqualTo, 0)

			solver.publish()
			So(viewer.manifold, ShouldNotBeNil)
			So(viewer.manifold.Manifold, ShouldNotBeNil)
			So(viewer.manifold.Manifold.GridX, ShouldEqual, 8)
		})
	})
}
