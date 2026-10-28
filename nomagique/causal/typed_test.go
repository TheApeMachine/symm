package causal

import "testing"

func causalRows(rowCount int) [][]float64 {
	rows := make([][]float64, 0, rowCount)

	for rowIndex := range rowCount {
		controlPrimary := float64(rowIndex % 3)
		controlSecondary := float64((rowIndex*rowIndex + rowIndex) % 5)
		treatment := float64(rowIndex)*0.5 + controlPrimary*0.2 - controlSecondary*0.1
		target := controlPrimary*0.3 - controlSecondary*0.2 + treatment*0.8
		rows = append(rows, []float64{
			controlPrimary,
			controlSecondary,
			treatment,
			target,
		})
	}

	return rows
}

func TestBackdoorMeasure(t *testing.T) {
	backdoor := NewBackdoor(BackdoorConfig{
		Target:    3,
		Treatment: 2,
		Controls:  []int{0, 1},
	})

	output, err := backdoor.Measure(BackdoorInput{
		Rows: causalRows(16),
	})
	if err != nil {
		t.Fatal(err)
	}

	if output.Effect == 0 {
		t.Fatal("expected non-zero backdoor effect")
	}
}

func TestRegimeMeasure(t *testing.T) {
	regime := NewRegime(RegimeConfig{
		Target:         3,
		ContagionBreak: 0.8,
		ContagionSkip:  []int{0, 3},
	})

	output, err := regime.Measure(RegimeInput{
		Rows:      causalRows(16),
		Contagion: 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	if output.RawInverted != 0 {
		t.Fatalf("raw inverted = %f, want 0", output.RawInverted)
	}

	output, err = regime.Measure(RegimeInput{
		Rows:      causalRows(16),
		Contagion: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	if output.RawInverted != 1 {
		t.Fatalf("raw inverted = %f, want 1", output.RawInverted)
	}
}
