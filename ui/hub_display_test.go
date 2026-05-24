package ui

import (
	"context"
	"testing"

	"github.com/theapemachine/symm/fluid"
)

type stubFluidDisplay struct {
	snapshot fluid.DisplayParamsSnapshot
}

func (stub *stubFluidDisplay) ApplyDisplayPatch(
	patch fluid.DisplayPatch,
) (fluid.DisplayParamsSnapshot, error) {
	if patch.HeightEMAAlpha != nil {
		stub.snapshot.HeightEMAAlpha = *patch.HeightEMAAlpha
	}

	return stub.snapshot, nil
}

func (stub *stubFluidDisplay) DisplayParams() fluid.DisplayParamsSnapshot {
	return stub.snapshot
}

func TestHubSetFluidDisplayBroadcastsSnapshot(t *testing.T) {
	ctx := context.Background()
	hub, err := NewHub(ctx, nil)

	if err != nil {
		t.Fatalf("new hub: %v", err)
	}

	stub := &stubFluidDisplay{
		snapshot: fluid.DisplayParamsSnapshot{
			HeightEMAAlpha: 0.35,
			GridSize:       32,
			QuantileClip:   0.95,
		},
	}

	hub.SetFluidDisplayController(stub)

	hub.handleFluidDisplay(clientMessage{
		Op:             "set_fluid_display",
		HeightEMAAlpha: ptrFloat(0.5),
	})

	hub.replayMu.Lock()
	event := hub.lastFluidDisplay
	hub.replayMu.Unlock()

	if event == nil {
		t.Fatal("expected fluid_display replay event")
	}

	if event["event"] != "fluid_display" {
		t.Fatalf("expected fluid_display event, got %v", event["event"])
	}

	if event["height_ema_alpha"] != 0.5 {
		t.Fatalf("expected alpha 0.5, got %v", event["height_ema_alpha"])
	}
}

func ptrFloat(value float64) *float64 {
	return &value
}
