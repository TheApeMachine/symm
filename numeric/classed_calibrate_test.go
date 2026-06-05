package numeric

import (
	"testing"

	"github.com/theapemachine/symm/numeric/adaptive"
)

// TestSelfCalibrationAdaptsBands reproduces the depthflow failure in miniature: a
// stream whose values all fall under the seed band edge collapses onto one
// category. Online self-calibration must move the edges into the occupied range
// and spread the categories.
func TestSelfCalibrationAdaptsBands(t *testing.T) {
	classifier := adaptive.NewClassifier(
		[]float64{0.5, 2.0, 4.0}, // seed: everything below 0.5 lands in band 0
		[]float64{0, 1, 2, 3},
		[]string{"a", "b", "c", "d"},
	)

	calibrator := NewBandCalibrator([]float64{0.25, 0.25, 0.25, 0.25}, 2000, 500, 1000, 0.0)

	// Feed observations uniformly in [0, 0.4): degenerate under the seed edges. The
	// pooled calibrator is driven directly against the shared classifier, exactly as
	// a signal drives it once per emit.
	for i := 0; i < 4000; i++ {
		value := 0.4 * float64(i%1000) / 1000.0
		calibrator.Observe(value, classifier)
	}

	edges := classifier.Upper()

	if len(edges) != 3 {
		t.Fatalf("edges = %v, want 3", edges)
	}

	if edges[0] >= 0.5 {
		t.Errorf("band edges did not adapt — still near the seed: %v", edges)
	}

	// The categorised distribution should now spread across all four bands.
	counts := make([]int, 4)

	for i := 0; i < 1000; i++ {
		value := 0.4 * float64(i) / 1000.0
		code, _ := classifier.Code(value)
		counts[int(code)]++
	}

	for band, count := range counts {
		if count == 0 {
			t.Errorf("band %d is empty after calibration: counts=%v edges=%v", band, counts, edges)
		}
	}
}

// TestSelfCalibrationOffByDefault: without enabling it, the bands never move.
func TestSelfCalibrationOffByDefault(t *testing.T) {
	classifier := adaptive.NewClassifier(
		[]float64{0.5, 2.0, 4.0},
		[]float64{0, 1, 2, 3},
		[]string{"a", "b", "c", "d"},
	)

	classify := NewClassify(classifier)

	for i := 0; i < 4000; i++ {
		if _, err := classify.Next(0.4 * float64(i%1000) / 1000.0); err != nil {
			t.Fatalf("Next: %v", err)
		}
	}

	edges := classifier.Upper()

	if edges[0] != 0.5 || edges[1] != 2.0 || edges[2] != 4.0 {
		t.Errorf("bands moved without calibration enabled: %v", edges)
	}
}
