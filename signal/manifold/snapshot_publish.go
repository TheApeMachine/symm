package manifold

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/internal"
)

/*
publishSnapshot ships the last integrated rho projection and carrier state to ui
without advancing the GPU solver again. Timer publishes must not call integrate:
extra steps dissipate the field and drop whale carriers between trade bursts.
*/
func (system *System) publishSnapshot(eventAt time.Time) error {
	if eventAt.IsZero() {
		return fmt.Errorf("manifold: snapshot event time is zero")
	}

	if !system.field.hasPublishableSnapshot() {
		return nil
	}

	payload, err := system.field.snapshotPayload(eventAt)

	if err != nil {
		return err
	}

	if payload == nil {
		return nil
	}

	return system.base.Bus().Send(internal.ChannelUI, "manifold_snapshot", payload)
}
