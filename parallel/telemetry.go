package parallel

import (
	"io"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/types"
)

/*
publishVisualization sends the parallel experiment's Metal field and coders
through the same dashboard encodings as logic/resonance and logic/manifold,
so Fluid and Predictive Coding show what parallel actually runs.
*/
func (runner *Runner) publishVisualization(
	tick *data.Value,
	vizSymbol string,
	coder CoderReading,
) {
	if runner == nil || runner.hub == nil || tick == nil {
		return
	}

	runner.publishManifoldFrame(tick, vizSymbol)
	runner.publishCoderFrames(tick, vizSymbol, coder)
}

func (runner *Runner) publishManifoldFrame(tick *data.Value, scope string) {
	if runner.field == nil || !runner.field.Healthy() {
		return
	}

	frame := runner.field.ManifoldFrame()
	if frame == nil {
		return
	}

	if scope == "" {
		scope = types.Focus()
	}

	frame.Symbol = scope

	value := manifold.PublishReading(
		frame,
		scope,
		tick.Epoch,
		tick.Tick,
		tick.Timestamp,
	)
	if value == nil {
		return
	}

	value.Role = "parallel"

	io.Copy(runner.hub, value)
}

func (runner *Runner) publishCoderFrames(tick *data.Value, symbol string, coder CoderReading) {
	if symbol == "" {
		return
	}

	featureDim := len(resonance.Features)

	control, err := resonance.PublishCoderReading(
		symbol,
		tick,
		coder.controlRaw,
		resonance.CoderArch(featureDim),
	)
	if err == nil && control != nil {
		control.Role = "parallel-control"
		io.Copy(runner.hub, control)
	}

	treatDim := featureDim + physicsFeatureCount
	treatment, err := resonance.PublishCoderReading(
		symbol,
		tick,
		coder.treatmentRaw,
		resonance.CoderArch(treatDim),
	)
	if err == nil && treatment != nil {
		treatment.Role = resonance.ReadingRole
		io.Copy(runner.hub, treatment)
	}
}
