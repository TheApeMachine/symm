package strategy

import "testing"

func TestBelowEnergyKeepsADimPolicyToken(t *testing.T) {
	training := &Training{
		prefixes: [][]*Token{
			{{Key: "R01", Threshold: 0.5, Brightness: 0.5}},
			{{Key: "R02", Threshold: 0.5, Brightness: 0.5}},
		},
	}

	dim := training.belowEnergy(nil, []*Token{
		{Key: "R01", Brightness: 0.2},
		{Key: "R09", Brightness: 0.1},
	})

	if len(dim) != 1 || dim[0].Key != "R01" {
		t.Fatalf("dim tokens = %+v", dim)
	}

	if len(training.belowEnergy(Tokens{{Key: "R01"}}, []*Token{
		{Key: "R02", Brightness: 0.9},
	})) != 0 {
		t.Fatal("a token at or above the gate is not a refusal")
	}
}
