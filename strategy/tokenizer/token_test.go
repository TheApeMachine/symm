package tokenizer

import "testing"

func TestTokenStringComposite(t *testing.T) {
	token := NewToken([]string{"R09", "R03", "R13", "R06"}, 0.8, 0)
	if token.String() != "[R09-R03-R13-R06]" {
		t.Fatalf("token = %q", token.String())
	}
}
