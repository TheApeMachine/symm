package strategy

import (
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/strategy/tokenizer"
)

/*
appendEmitReadout records gate/blocked fields from the same Step outcome the
path uses. Telemetry and longtail rollups read these layers; nothing invents
surprise, friction, or lit fallback tokens here.
*/
func appendEmitReadout(out *data.Value, step tokenizer.StepResult, hadLit bool) {
	passed := step.Token.String() != ""

	var gate stepGate
	switch {
	case passed:
		gate = stepGate{Passed: true, Warm: false}
	case step.Silence == tokenizer.StepSilenceBrightness:
		gate = stepGate{Passed: false, Warm: false, Cold: "brightness"}
	case step.Silence == tokenizer.StepSilenceDuplicate:
		gate = stepGate{Passed: false, Warm: false, Cold: "duplicate"}
	case step.Silence == tokenizer.StepSilenceVeto:
		gate = stepGate{Passed: false, Warm: false, Cold: "veto"}
	default:
		gate = stepGate{Passed: false, Warm: false, Cold: "empty"}
	}

	out.Append(gate, "gate")

	blocked := hadLit && !passed
	out.Append(blocked, "blocked")

	if blocked && step.Silence != "" {
		out.Append(string(step.Silence), "blockedReason")
	}
}
