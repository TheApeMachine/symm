package types

import "testing"

/*
TestSkillSeedUsesSignalShare proves Seed prefers |ER|/(|ER|+σ) over uncertainty
share so trail distance is not identically |ER|+σ.
*/
func TestSkillSeedUsesSignalShare(t *testing.T) {
	t.Parallel()

	skill := NewSkill()
	skill.Seed(StopEvidence{ExpectedReturn: 0.05, Uncertainty: 0.02})
	want := 0.05 / 0.07

	if skill.Weight < want-1e-9 || skill.Weight > want+1e-9 {
		t.Fatalf("signal-share weight: want %v, got %v", want, skill.Weight)
	}
}

/*
TestSkillReweightOncePerEpoch ensures skill updates apply once per forecast
epoch and do not double-apply when the same epoch is replayed.
*/
func TestSkillReweightOncePerEpoch(t *testing.T) {
	t.Parallel()

	skill := NewSkill()
	skill.Reweight(StopEvidence{
		ReturnReady: true, ForecastEpoch: 1, NormalizedResidual: 4,
	})
	first := skill.Weight
	skill.Reweight(StopEvidence{
		ReturnReady: true, ForecastEpoch: 1, NormalizedResidual: 0.1,
	})

	if skill.Weight != first {
		t.Fatalf("same epoch must not reweight: first=%v duplicate=%v", first, skill.Weight)
	}

	skill.Reweight(StopEvidence{
		ReturnReady: true, ForecastEpoch: 2, NormalizedResidual: 0.1,
	})

	if skill.Weight <= first {
		t.Fatalf("new epoch should reweight upward: first=%v next=%v", first, skill.Weight)
	}
}

/*
BenchmarkSkillReweight measures one epoch reweight.
*/
func BenchmarkSkillReweight(b *testing.B) {
	skill := NewSkill()
	evidence := StopEvidence{
		ReturnReady: true, ForecastEpoch: 1, NormalizedResidual: 0.5,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; b.Loop(); index++ {
		evidence.ForecastEpoch = uint64(index + 1)
		skill.Reweight(evidence)
	}
}
