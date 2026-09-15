package ui

import (
	"context"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"golang.design/x/lockfree/wf"
)

type SnapshotProvider interface {
	Snapshot() *types.ManifoldState
}

/*
WebRTCTee is a concrete off-ramp that accepts *data.Measurement[float64]
and fans manifold state out to connected WebRTC peers.
It satisfies runtime.Tee[*data.Measurement[float64]].
*/
type WebRTCTee struct {
	*runtime.System
	ring     *wf.RingBuffer[*data.Measurement[float64]]
	fluid    *FluidRTC
	provider SnapshotProvider
}

/*
NewWebRTCTee creates a new wait-free WebRTCTee off-ramp.
*/
func NewWebRTCTee(label string, capacity int, fluid *FluidRTC, provider SnapshotProvider) *WebRTCTee {
	tee := &WebRTCTee{
		ring:     wf.NewRingBuffer[*data.Measurement[float64]](capacity),
		fluid:    fluid,
		provider: provider,
	}

	tee.System = runtime.NewSystem(context.Background(), label, tee)
	tee.Transition(runtime.READY)

	return tee
}

/*
Push receives measurements from the workspace. When a manifold measurement arrives,
and a WebRTC viewer is ready, it serializes and publishes the manifold state.
*/
func (tee *WebRTCTee) Push(measurement *data.Measurement[float64]) {
	if tee == nil || measurement == nil || tee.fluid == nil {
		return
	}

	if measurement.Source == "manifold" && tee.fluid.WantsManifold() && tee.provider != nil {
		snapshot := tee.provider.Snapshot()
		if snapshot != nil {
			tee.fluid.PublishManifold(snapshot)
		}
	}

	if !tee.ring.Put(measurement) {
		tee.Transition(runtime.ERROR)
	}
}

/*
Next drains available measurements from the ring buffer.
*/
func (tee *WebRTCTee) Next() *data.Measurement[float64] {
	if tee == nil || tee.Status() != runtime.READY {
		return nil
	}

	measurement, ok := tee.ring.Get()
	if !ok {
		return nil
	}

	return measurement
}

/*
WantsManifold satisfies manifold.Viewer.
*/
func (tee *WebRTCTee) WantsManifold() bool {
	if tee == nil || tee.fluid == nil {
		return false
	}

	return tee.fluid.WantsManifold()
}

/*
PublishManifold satisfies manifold.Viewer.
*/
func (tee *WebRTCTee) PublishManifold(state *types.ManifoldState) {
	if tee == nil || tee.fluid == nil || state == nil {
		return
	}

	tee.fluid.PublishManifold(state)
}
