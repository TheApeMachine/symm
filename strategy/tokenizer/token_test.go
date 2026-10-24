package tokenizer

import "testing"

func TestTokenStringComposite(t *testing.T) {
	token := NewToken([]string{"R09", "R03", "R13", "R06"}, 0.8, 0)
	if token.String() != "[R09-R03-R13-R06]" {
		t.Fatalf("token = %q", token.String())
	}
}

func TestCutHasExitBand(t *testing.T) {
	token := NewToken([]string{"R03", "R12"}, 0.8, 0)
	if !CutHasExitBand(token) {
		t.Fatal("expected exit band")
	}

	flat := NewToken([]string{"R00", "R07"}, 0.8, 0)
	if CutHasExitBand(flat) {
		t.Fatal("enter token should not be exit band")
	}
}
