package types

import (
	"errors"
	"testing"
)

func TestMapCloneIterationAndDelete(t *testing.T) {
	mapping := NewMap[string, float64]()
	mapping.Put("a", 1)
	mapping.Put("b", 2)

	seen := 0
	for range mapping.All() {
		seen++
	}
	if seen != 2 || mapping.Len() != 2 {
		t.Fatalf("seen=%d len=%d; want 2", seen, mapping.Len())
	}

	clone := mapping.Clone()
	clone.Put("a", 3)
	if value, _ := mapping.Get("a"); value != 1 {
		t.Fatalf("clone mutation changed source to %v", value)
	}
	clone.Delete("b")
	if clone.Len() != 1 || mapping.Len() != 2 {
		t.Fatalf("clone len=%d source len=%d", clone.Len(), mapping.Len())
	}
}

func TestErrorInputRetainsCurrentPayload(t *testing.T) {
	failure := NewErrorInput(7.0, errors.New("failed"))
	if failure.Error() == "" {
		t.Fatal("failure should report an error")
	}
	if got := failure.Project().Read(); got != 7 {
		t.Fatalf("retained payload=%v; want 7", got)
	}
}
