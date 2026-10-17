package hindsight

import "testing"

func TestKeysFingerprintStable(t *testing.T) {
	t.Parallel()

	left := KeysFingerprint([]string{"b", "a", "c"})
	right := KeysFingerprint([]string{"a", "b", "c"})

	if left != right {
		t.Fatalf("fingerprint order sensitive: %d vs %d", left, right)
	}

	if KeysFingerprint([]string{"a", "b"}) == KeysFingerprint([]string{"a", "b", "c"}) {
		t.Fatal("different listings should not match")
	}
}
