package strategy

import (
	"io"
	"strings"

	"github.com/theapemachine/nomagique/data"
	nmstore "github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/ui"
)

/*
Regions fans lit grid rows onto the workspace tick (thesis / S3 / training
Lookup) and, when the dashboard is attached, replaces each symbol's hub
frame every tick — including an empty lit set so the Impulse Map can go
dark. signal.Emit stays focus-gated metric traffic only.
*/
type Regions struct{}

func NewRegions() *Regions {
	return &Regions{}
}

func (regions *Regions) Publish(
	tick *data.Value, symbol string, lit []*nmstore.Region,
) {
	if regions == nil || tick == nil || symbol == "" {
		return
	}

	if lit == nil {
		lit = []*nmstore.Region{}
	}

	tick.Append(lit, "grid", symbol, "regions")

	if parts := strings.Split(symbol, "/"); len(parts) == 2 {
		tick.Append(lit, "train", "regions", parts[0], parts[1])
	}

	if ui.UI == nil || !ui.UI.Active() {
		return
	}

	grid := data.NewValue("grid", "regions", symbol)
	grid.Epoch = tick.Epoch
	grid.Tick = tick.Tick
	grid.Timestamp = tick.Timestamp
	grid.Append(lit, "regions")
	io.Copy(ui.UI, grid)

	if parts := strings.Split(symbol, "/"); len(parts) == 2 {
		train := data.NewValue("train", "regions", symbol)
		train.Epoch = tick.Epoch
		train.Tick = tick.Tick
		train.Timestamp = tick.Timestamp
		train.Append(lit, "regions")
		io.Copy(ui.UI, train)
	}
}
