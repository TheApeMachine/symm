package ui

import (
	"context"
	"encoding/json"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/fluid"
)

/*
FluidCommands handles dashboard control messages forwarded from websocket clients.
*/
type FluidCommands struct {
	ui     *qpool.BroadcastGroup
	fluid  *fluid.Fluid
	stream *MarketStream
}

/*
NewFluidCommands subscribes to the ui group and responds to fluid control ops.
*/
func NewFluidCommands(
	ctx context.Context,
	ui *qpool.BroadcastGroup,
	fluidSignal *fluid.Fluid,
	stream *MarketStream,
) *FluidCommands {
	handler := &FluidCommands{
		ui:     ui,
		fluid:  fluidSignal,
		stream: stream,
	}

	subscription := ui.Subscribe("fluid_commands", 256)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case value := <-subscription.Incoming:
				if value == nil {
					continue
				}

				handler.handle(value.Value)
			}
		}
	}()

	return handler
}

func (handler *FluidCommands) handle(raw any) {
	payload, ok := decodeCommand(raw)

	if !ok {
		return
	}

	op, _ := payload["op"].(string)

	switch op {
	case "get_fluid_display":
		handler.publishDisplay()
	case "set_fluid_display":
		handler.applyDisplay(payload)
	}
}

func (handler *FluidCommands) publishDisplay() {
	if handler.fluid == nil || handler.stream == nil {
		return
	}

	handler.stream.FluidDisplay(handler.fluid.DisplayParams())
}

func (handler *FluidCommands) applyDisplay(payload map[string]any) {
	if handler.fluid == nil || handler.stream == nil {
		return
	}

	patch := fluid.DisplayPatch{}

	if alpha, ok := optionalFloat(payload["height_ema_alpha"]); ok {
		patch.HeightEMAAlpha = &alpha
	}

	if size, ok := optionalInt(payload["grid_size"]); ok {
		patch.GridSize = &size
	}

	if clip, ok := optionalFloat(payload["quantile_clip"]); ok {
		patch.QuantileClip = &clip
	}

	if reset, ok := payload["reset_smoothing"].(bool); ok && reset {
		patch.ResetSmoothing = &reset
	}

	snapshot, err := handler.fluid.ApplyDisplayPatch(patch)

	if err != nil {
		return
	}

	handler.stream.FluidDisplay(snapshot)
}

func decodeCommand(raw any) (map[string]any, bool) {
	switch typed := raw.(type) {
	case map[string]any:
		return typed, true
	case []byte:
		payload := map[string]any{}

		if err := json.Unmarshal(typed, &payload); err != nil {
			return nil, false
		}

		return payload, true
	default:
		return nil, false
	}
}

func optionalFloat(raw any) (float64, bool) {
	switch typed := raw.(type) {
	case float64:
		return typed, true
	case json.Number:
		value, err := typed.Float64()

		return value, err == nil
	default:
		return 0, false
	}
}

func optionalInt(raw any) (int, bool) {
	switch typed := raw.(type) {
	case float64:
		return int(typed), true
	case json.Number:
		value, err := typed.Int64()

		return int(value), err == nil
	default:
		return 0, false
	}
}
