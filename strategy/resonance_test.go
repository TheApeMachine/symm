package strategy

import "testing"

func TestHorizonCollapsed(t *testing.T) {
	track := &cliffSample{peakHorizon: 5, seeded: true}

	if !horizonCollapsed(track, 2) {
		t.Fatal("expected collapse when supported drops from peak 5 to 2")
	}

	if horizonCollapsed(track, 4) {
		t.Fatal("supported 4 should not collapse against peak 5 with default drop 2")
	}

	flat := &cliffSample{peakHorizon: 1, seeded: true}
	if !horizonCollapsed(flat, 0) {
		t.Fatal("expected collapse below 1 supported horizon")
	}
}

func TestSkillCollapsed(t *testing.T) {
	if !skillCollapsed(0.85) {
		t.Fatal("skill below 1 should count as collapsed")
	}

	if skillCollapsed(1.05) {
		t.Fatal("skill above 1 should not collapse")
	}

	if skillCollapsed(0) {
		t.Fatal("unready skill zero should not count as collapsed")
	}
}
