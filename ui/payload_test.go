package ui

import "testing"

func TestOmitEmptyCollections(t *testing.T) {
	payload := map[string]any{
		"signals":       []map[string]any{},
		"evaluations":   []map[string]any{},
		"source_scores": map[string]float64{},
		"seq":           3,
	}

	omitEmptyCollections(payload)

	if _, ok := payload["signals"]; ok {
		t.Fatal("expected empty signals to be omitted")
	}

	if _, ok := payload["evaluations"]; ok {
		t.Fatal("expected empty evaluations to be omitted")
	}

	if _, ok := payload["source_scores"]; ok {
		t.Fatal("expected empty source_scores to be omitted")
	}

	if payload["seq"] != 3 {
		t.Fatal("expected scalar fields to remain")
	}
}
