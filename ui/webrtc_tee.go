package ui

import (
	"context"
	"sync/atomic"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"golang.design/x/lockfree/lf"
)

/*
WebRTCTee accepts completed immutable results directly from concurrent workspace
consumers. Route exclusions are dropped before entering its lock-free queue.
*/
type WebRTCTee struct {
	*runtime.System
	queue    *lf.Queue[any]
	pending  atomic.Int64
	capacity int64
}

func NewWebRTCTee(ctx context.Context, label string, capacity int) *WebRTCTee {
	return &WebRTCTee{
		System:   runtime.NewSystem(ctx, label),
		queue:    lf.NewQueue[any](),
		capacity: int64(capacity),
	}
}

func (tee *WebRTCTee) Push(measurement *data.Measurement[float64]) {
	if tee.Status() != runtime.READY {
		errnie.Warn(tee.Name() + ": Push called before READY; dropping event")
		return
	}

	if measurement == nil || !types.AllowsWebRTC(measurement.Source, measurement.Label) {
		return
	}

	var artifact any

	switch result := measurement.Result.(type) {
	case *types.ManifoldState:
		if result != nil {
			artifact = result
		}
	case *types.ResonanceArtifact:
		if result != nil {
			artifact = result
		}
	}

	if artifact == nil {
		return
	}

	if tee.pending.Add(1) > tee.capacity {
		tee.pending.Add(-1)
		errnie.Warn(tee.Name() + ": result queue is full; dropping visualization update")
		return
	}

	tee.queue.Enqueue(artifact)
}

// Next also drops pending results excluded by a subsequent route/focus change.
func (tee *WebRTCTee) Next() unsafe.Pointer {
	if tee.Status() != runtime.READY {
		errnie.Warn(tee.Name() + ": Next called before READY")
		return nil
	}

	for artifact, ok := tee.queue.Dequeue(); ok; artifact, ok = tee.queue.Dequeue() {
		tee.pending.Add(-1)
		source, label := "manifold", ""

		if result, ok := artifact.(*types.ResonanceArtifact); ok {
			source, label = "resonance", result.Symbol
		}

		if types.AllowsWebRTC(source, label) {
			return unsafe.Pointer(&artifact)
		}
	}

	return nil
}
