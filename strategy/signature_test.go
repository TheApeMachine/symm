package strategy

import (
	"bytes"
	"testing"
)

func TestAppendLitFrameAAACollapsesToA(t *testing.T) {
	a := []byte{4, 5, 215}
	var sig []byte
	sig = appendLitFrame(sig, a)
	sig = appendLitFrame(sig, a)
	sig = appendLitFrame(sig, a)

	want := append(append([]byte{}, a...), 0)
	if !bytes.Equal(sig, want) {
		t.Fatalf("AAA→A: got %v want %v", sig, want)
	}
}

func TestAppendLitFrameABBACollapsesToABA(t *testing.T) {
	a := []byte{1, 2, 3}
	b := []byte{7, 8, 9}
	var sig []byte
	sig = appendLitFrame(sig, a)
	sig = appendLitFrame(sig, b)
	sig = appendLitFrame(sig, b)
	sig = appendLitFrame(sig, a)

	want := append([]byte{}, a...)
	want = append(want, 0)
	want = append(want, b...)
	want = append(want, 0)
	want = append(want, a...)
	want = append(want, 0)

	if !bytes.Equal(sig, want) {
		t.Fatalf("ABBA→ABA: got %v want %v", sig, want)
	}
}

func TestAppendLitFrameEmptyTokenNoop(t *testing.T) {
	a := []byte{1, 2}
	sig := appendLitFrame(nil, a)
	same := appendLitFrame(sig, nil)
	if !bytes.Equal(sig, same) {
		t.Fatalf("empty token mutated signature: %v vs %v", sig, same)
	}
}
