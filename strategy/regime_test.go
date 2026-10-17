package strategy

import "testing"

func TestRegimeHoldsAStableEnergy(t *testing.T) {
	var regime regime

	for range 8 {
		if regime.advance(0.2) {
			t.Fatal("a constant energy broke the sentence")
		}
	}
}

func TestRegimeBreaksWhenTwoRegimesAreShorter(t *testing.T) {
	var regime regime

	for range 6 {
		if regime.advance(0) {
			t.Fatal("a constant energy broke the sentence")
		}
	}

	if !regime.advance(1) {
		t.Fatal("a second regime was not shorter than one")
	}

	if regime.advance(1) {
		t.Fatal("the new regime broke on its next sample")
	}
}

func TestRegimeHoldsAlternatingEnergy(t *testing.T) {
	var regime regime

	for _, energy := range []float64{1, 2, 1, 2, 1, 2, 1, 2} {
		if regime.advance(energy) {
			t.Fatal("one alternating regime broke")
		}
	}
}

func TestRegimeBreaksOnALevelShift(t *testing.T) {
	var regime regime
	broke := false

	for _, energy := range []float64{1, 2, 1, 2, 10, 11, 10, 11} {
		if !regime.advance(energy) {
			continue
		}

		if energy < 10 {
			t.Fatal("the first level broke")
		}

		broke = true
		break
	}

	if !broke {
		t.Fatal("the second level was not a shorter description")
	}

	if regime.advance(11) {
		t.Fatal("the new regime broke on its next sample")
	}
}

func TestPhraseMustSitInsideTheSentence(t *testing.T) {
	sentence := []string{"R01", "R05"}

	if !phraseInSentence([]string{"R01", "R05"}, sentence) {
		t.Fatal("the sentence itself was rejected")
	}

	if !phraseInSentence([]string{"R05"}, sentence) {
		t.Fatal("a rule inside the sentence was rejected")
	}

	if phraseInSentence([]string{"R09", "R01"}, sentence) {
		t.Fatal("a rule that crosses the break was accepted")
	}
}
