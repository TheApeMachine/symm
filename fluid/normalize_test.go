package fluid

import "testing"

func TestRobustCrossSectionActivityScalesUnlikeRawUnits(t *testing.T) {
	divActivity := robustCrossSectionActivity([]float64{80, 120})
	reActivity := robustCrossSectionActivity([]float64{0.002, 0.006})

	if divActivity <= 0 || reActivity <= 0 {
		t.Fatalf("expected positive activity, div=%v re=%v", divActivity, reActivity)
	}

	if mathAbs(divActivity-reActivity) > 4 {
		t.Fatalf(
			"expected comparable normalized activity, div=%v re=%v",
			divActivity,
			reActivity,
		)
	}
}

func TestRobustCrossSectionActivityEmpty(t *testing.T) {
	if robustCrossSectionActivity(nil) != 0 {
		t.Fatal("expected zero activity for empty slice")
	}
}

func mathAbs(value float64) float64 {
	if value < 0 {
		return -value
	}

	return value
}

func BenchmarkRobustCrossSectionActivity(b *testing.B) {
	values := make([]float64, 400)

	for index := range values {
		values[index] = float64(index%40) * 0.01
	}

	b.ReportAllocs()

	for b.Loop() {
		robustCrossSectionActivity(values)
	}
}
