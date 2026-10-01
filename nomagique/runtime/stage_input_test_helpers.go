package runtime

import (
	"github.com/theapemachine/symm/nomagique/data"
)

/*
TestStageInput creates a StageInput suitable for testing. It takes an ingress
measurement and an optional set of prior-stage outputs.

This is a test convenience: production code uses Workspace.buildStageInput.
*/
func TestStageInput(
	ingress *data.Measurement[float64],
	priorOutputs ...[]*data.Measurement[float64],
) *StageInput {
	return &StageInput{
		seq:          1,
		ingress:      ingress,
		priorOutputs: priorOutputs,
	}
}

/*
TestStageInputFromPeers creates a StageInput from an old-style measurement
with Peers. The ingress is the measurement itself, and the Peers become
prior-stage outputs (stage 0). This bridges tests written for the old
Fork/Contribute model.
*/
func TestStageInputFromPeers(measurement *data.Measurement[float64]) *StageInput {
	if measurement == nil {
		return nil
	}

	var priorOutputs [][]*data.Measurement[float64]
	if len(measurement.Peers) > 0 {
		priorOutputs = [][]*data.Measurement[float64]{measurement.Peers}
	}

	return &StageInput{
		seq:          measurement.SeqIdx,
		ingress:      measurement,
		priorOutputs: priorOutputs,
	}
}
