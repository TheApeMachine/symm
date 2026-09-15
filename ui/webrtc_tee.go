package ui

import (
	"context"
	"unsafe"

	"github.com/theapemachine/errnie"
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
func NewWebRTCTee(label string, capacity int) *WebRTCTee {
	tee := &WebRTCTee{
		ring: wf.NewRingBuffer[*data.Measurement[float64]](capacity),
	}

	tee.System = runtime.NewSystem(context.Background(), label, tee)
	return tee
}

/*
Push receives measurements from the workspace. When a manifold measurement arrives,
and a WebRTC viewer is ready, it serializes and publishes the manifold state.
*/
func (tee *WebRTCTee) Push(measurement *data.Measurement[float64]) {
	if tee.Status() != runtime.READY {
		errnie.Warn("pushing to a non-ready system may have unintended consequences")
	}

	if !tee.ring.Put(measurement) {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"[webrtc] internal error encountered: unable to push measurement",
			nil,
		))

		tee.Transition(runtime.ERROR)
	}
}

/*
Next drains available measurements from the ring buffer.
*/
func (tee *WebRTCTee) Next() unsafe.Pointer {
	if tee.Status() != runtime.READY {
		errnie.Warn("pushing to a non-ready system may have unintended consequences")
		return nil
	}

	measurement, ok := tee.ring.Get()

	if !ok {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[webrtc] unable to process measurement",
			nil,
		))

		return nil
	}

	return unsafe.Pointer(measurement)
}
