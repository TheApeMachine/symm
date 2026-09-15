package ui

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

type mockSnapshotProvider struct {
	snap *types.ManifoldState
}

func (m *mockSnapshotProvider) Snapshot() *types.ManifoldState {
	return m.snap
}

func TestWebRTCTee(t *testing.T) {
	Convey("Given a WebRTCTee off-ramp", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		hub := NewHub(ctx, nil)
		defer hub.Close()

		provider := &mockSnapshotProvider{
			snap: &types.ManifoldState{
				Version: 1,
			},
		}

		tee := NewWebRTCTee("testWebRTC", 16, hub.Fluid(), provider)
		defer tee.Close()

		Convey("Reports whether manifold is wanted", func() {
			So(tee.WantsManifold(), ShouldBeFalse)
		})

		Convey("Pushing non-manifold measurement does not trigger manifold publication", func() {
			meas := data.NewMeasurement("cvd", map[string]data.Metric[float64]{})
			tee.Push(meas)
			out := tee.Next()
			So(out, ShouldNotBeNil)
			So(out.Source, ShouldEqual, "cvd")
		})
	})
}
