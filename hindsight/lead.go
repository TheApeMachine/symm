package hindsight

import (
	"github.com/theapemachine/symm/system"
)

/*
applyExecutionLead shifts recorded B and C earlier using detector.execution_lead_fraction
times the A→B precursor span. Entry and exit indices match the shifted B and C.
*/
// ApplyExecutionLeadFields shifts B/C and sets entry/exit in a metrics map.
func ApplyExecutionLeadFields(fields map[string]float64) {
	if len(fields) == 0 || system.Cfg == nil || system.Cfg.Detector == nil {
		return
	}

	aIdx := int64(fields["a_idx"])
	aTick := int64(fields["a_tick"])
	bIdx := int64(fields["b_idx"])
	bTick := int64(fields["b_tick"])
	cIdx := int64(fields["c_idx"])
	cTick := int64(fields["c_tick"])

	outBIdx, outBTick, outCIdx, outCTick, entryIdx, entryTick, exitIdx, exitTick := system.Cfg.Detector.ApplyExecutionLead(
		aIdx, aTick, bIdx, bTick, cIdx, cTick,
	)

	fields["b_idx"] = float64(outBIdx)
	fields["b_tick"] = float64(outBTick)
	fields["c_idx"] = float64(outCIdx)
	fields["c_tick"] = float64(outCTick)
	fields["entry_idx"] = float64(entryIdx)
	fields["entry_tick"] = float64(entryTick)
	fields["exit_idx"] = float64(exitIdx)
	fields["exit_tick"] = float64(exitTick)
}
