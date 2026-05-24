package ui

import (
	"encoding/json"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/fluid"
)

/*
FluidCommands handles dashboard control messages forwarded from websocket clients.
*/
type FluidCommands struct {
	fluid *fluid.Fluid
	ui    *qpool.BroadcastGroup
}

/*
DashboardCommands routes websocket control payloads to dashboard subsystems.
*/
type DashboardCommands struct {
	fluidCommands *FluidCommands
	chartWatch    *ChartWatch
}

/*
HandleCommand ingests one websocket control payload from the hub read pump.
*/
func (handler *FluidCommands) HandleCommand(raw any) {
	handler.handle(raw)
}

/*
NewFluidCommands wires fluid display control handlers.
*/
func NewFluidCommands(
	fluidSignal *fluid.Fluid,
	ui *qpool.BroadcastGroup,
) *FluidCommands {
	return &FluidCommands{
		fluid: fluidSignal,
		ui:    ui,
	}
}

/*
NewDashboardCommands wires dashboard display controls and chart tick watching.
*/
func NewDashboardCommands(
	fluidSignal *fluid.Fluid,
	ui *qpool.BroadcastGroup,
	chartWatch *ChartWatch,
) *DashboardCommands {
	return &DashboardCommands{
		fluidCommands: NewFluidCommands(fluidSignal, ui),
		chartWatch:    chartWatch,
	}
}

/*
HandleCommand ingests one websocket control payload from the hub read pump.
*/
func (handler *DashboardCommands) HandleCommand(raw any) {
	payload, ok := decodeCommand(raw)

	if !ok {
		return
	}

	op, _ := payload["op"].(string)

	switch op {
	case "get_fluid_display", "set_fluid_display":
		handler.fluidCommands.HandleCommand(payload)
	case "subscribe":
		handler.chartWatch.Subscribe(commandSymbols(payload))
	case "unsubscribe":
		handler.chartWatch.Unsubscribe(commandSymbols(payload))
	case "watch":
		symbol, _ := payload["symbol"].(string)
		handler.chartWatch.Replace([]string{symbol})
	}
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

func commandSymbols(payload map[string]any) []string {
	switch typed := payload["symbols"].(type) {
	case []string:
		return typed
	case []any:
		symbols := make([]string, 0, len(typed))

		for _, symbolValue := range typed {
			symbol, ok := symbolValue.(string)

			if !ok || symbol == "" {
				continue
			}

			symbols = append(symbols, symbol)
		}

		return symbols
	default:
		symbol, _ := payload["symbol"].(string)

		if symbol == "" {
			return nil
		}

		return []string{symbol}
	}
}

func (handler *FluidCommands) publishDisplay() {
	if handler.fluid == nil || handler.ui == nil {
		return
	}

	Publish(handler.ui, "fluid_display", map[string]any{
		"height_ema_alpha": handler.fluid.DisplayParams().HeightEMAAlpha,
		"grid_size":        handler.fluid.DisplayParams().GridSize,
		"quantile_clip":    handler.fluid.DisplayParams().QuantileClip,
	})
}

func (handler *FluidCommands) applyDisplay(payload map[string]any) {
	if handler.fluid == nil || handler.ui == nil {
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

	Publish(handler.ui, "fluid_display", map[string]any{
		"height_ema_alpha": snapshot.HeightEMAAlpha,
		"grid_size":        snapshot.GridSize,
		"quantile_clip":    snapshot.QuantileClip,
	})
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
