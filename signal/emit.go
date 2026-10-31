package signal

import (
	"io"
	"strings"

	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Emit writes signal metrics onto the workspace tick for the thesis and S3, and
when the dashboard is focused on that symbol (or the layer is board aggregate),
sends a small frame addressed as origin=<signal>, scope=<symbol|aggregate>.

Grid / train region frames are not metrics: stage publishes those itself.
*/
type Emit struct{}

func NewEmit() *Emit {
	return &Emit{}
}

/*
Metric appends onto the tick, then fans a focus-scoped frame to the hub.
*/
func (emit *Emit) Metric(tick *data.Value, layer any, keys ...string) *data.Value {
	if emit == nil || tick == nil {
		return tick
	}

	tick.Append(layer, keys...)

	if !emit.allows(keys) {
		return tick
	}

	if ui.UI == nil || !ui.UI.Active() {
		return tick
	}

	origin, scope, rest := emit.wireAddress(keys)
	if origin == "" || scope == "" {
		return tick
	}

	frame := data.NewValue(origin, "metric", scope)
	frame.Epoch = tick.Epoch
	frame.Tick = tick.Tick
	frame.Timestamp = tick.Timestamp

	if len(rest) == 0 {
		frame.Append(layer)
	} else {
		frame.Append(layer, rest...)
	}

	io.Copy(ui.UI, frame)
	return tick
}

func (emit *Emit) allows(keys []string) bool {
	_, scope, _ := emit.wireAddress(keys)
	if scope == "" {
		return false
	}

	if scope == "aggregate" {
		return true
	}

	return types.Allows(scope)
}

/*
wireAddress maps tick layer keys to a hub origin, scope, and metric path.
*/
func (emit *Emit) wireAddress(keys []string) (origin, scope string, rest []string) {
	if len(keys) < 2 {
		return "", "", nil
	}

	origin = keys[0]

	if keys[1] == "aggregate" {
		return origin, "aggregate", keys[2:]
	}

	if strings.Contains(keys[1], "/") {
		return origin, keys[1], keys[2:]
	}

	if len(keys) >= 3 {
		return origin, keys[1] + "/" + keys[2], keys[3:]
	}

	return "", "", nil
}
