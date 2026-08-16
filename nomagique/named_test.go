package nomagique

import "testing"

func TestFrameNamedBoundaryConversion(t *testing.T) {
	frame, err := FrameFromNamed(map[string]float64{
		"named_test/alpha": 3,
		"named_test/beta":  4,
	})

	if err != nil {
		t.Fatal(err)
	}

	values := frame.Named()

	if values["named_test/alpha"] != 3 || values["named_test/beta"] != 4 {
		t.Fatalf("named values=%v; want alpha=3 beta=4", values)
	}
}
