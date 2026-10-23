package system_test

import (
	"testing"

	"github.com/theapemachine/symm/system"
)

func TestLeadSpan(t *testing.T) {
	d := &system.Detector{ExecutionLeadFraction: 0.1}

	if got := d.LeadSpan(10); got != 1 {
		t.Fatalf("LeadSpan(10) = %d, want 1", got)
	}

	if got := d.LeadSpan(1); got != 0 {
		t.Fatalf("LeadSpan(1) = %d, want 0", got)
	}

	d.ExecutionLeadFraction = 0.99
	if got := d.LeadSpan(5); got != 4 {
		t.Fatalf("LeadSpan(5) at 0.99 = %d, want 4", got)
	}
}

func TestApplyExecutionLeadShiftsBAndC(t *testing.T) {
	d := &system.Detector{ExecutionLeadFraction: 0.1}

	bIdx, bTick, cIdx, cTick, entryIdx, entryTick, exitIdx, exitTick := d.ApplyExecutionLead(
		100, 1000, 110, 1010, 130, 1030,
	)

	if bIdx != 109 || bTick != 1009 {
		t.Fatalf("shifted B = idx %d tick %d, want 109 / 1009", bIdx, bTick)
	}

	if cIdx != 129 || cTick != 1029 {
		t.Fatalf("shifted C = idx %d tick %d, want 129 / 1029", cIdx, cTick)
	}

	if entryIdx != bIdx || entryTick != bTick || exitIdx != cIdx || exitTick != cTick {
		t.Fatal("entry/exit must match shifted B and C")
	}
}

func TestApplyExecutionLeadKeepsIndexAndTickOnePrint(t *testing.T) {
	d := &system.Detector{ExecutionLeadFraction: 0.1}

	bIdx, bTick, cIdx, cTick, entryIdx, entryTick, _, _ := d.ApplyExecutionLead(
		100, 1000, 110, 1100, 130, 1200,
	)

	if bTick-bIdx != 1100-110 {
		t.Fatalf("shifted B idx %d tick %d no longer names one print", bIdx, bTick)
	}

	if cTick-cIdx != 1200-130 {
		t.Fatalf("shifted C idx %d tick %d no longer names one print", cIdx, cTick)
	}

	if entryIdx != bIdx || entryTick != bTick {
		t.Fatal("entry must be the shifted B print")
	}

	if bIdx != 100 || bTick != 1090 {
		t.Fatalf("shifted B = idx %d tick %d, want 100 / 1090", bIdx, bTick)
	}
}
