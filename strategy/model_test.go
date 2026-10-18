package strategy

import (
	"context"
	"testing"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestModelRefusesARegionBelowThePathThreshold(t *testing.T) {
	model := NewModel(context.Background(), nil, nil)
	model.Note("R01/R02")

	if got := model.Match(nil, []*Token{{Key: "R01", Brightness: 0.2}}); len(got) != 0 {
		t.Fatalf("dim region admitted: %v", got)
	}

	if got := model.Match(nil, []*Token{{Key: "R01", Brightness: 0.5}}); len(got) != 1 || got[0] != "R01" {
		t.Fatalf("region at the gate: %v", got)
	}

	dim := model.Below(nil, []*Token{{Key: "R01", Brightness: 0.2}, {Key: "R09", Brightness: 0.1}})

	if len(dim) != 1 || dim[0].Key != "R01" {
		t.Fatalf("refusals: %+v", dim)
	}
}

func TestModelLowersTheThresholdAfterAProfit(t *testing.T) {
	model := NewModel(context.Background(), nil, nil)
	model.Note("R01/R02")

	model.learn("R01/R02", true, false)
	model.learn("R01/R02", true, false)

	if got := model.Match(nil, []*Token{{Key: "R01", Brightness: 0.4}}); len(got) != 1 {
		t.Fatalf("two profits should clear 0.4, got %v (gate %v)", got, model.Threshold["R01/R02"])
	}
}

func TestModelRaisesTheThresholdAfterALoss(t *testing.T) {
	model := NewModel(context.Background(), nil, nil)
	model.Note("R01")

	model.learn("R01", false, false)

	if got := model.Match(nil, []*Token{{Key: "R01", Brightness: 0.5}}); len(got) != 0 {
		t.Fatalf("a loss should push the gate above 0.5, got %v (gate %v)", got, model.Threshold["R01"])
	}
}

func TestModelStoresAPathAfterThreeVirtualProfits(t *testing.T) {
	model := NewModel(context.Background(), nil, nil)
	path := "R01/R04"

	if model.learn(path, true, true) || model.learn(path, false, true) || model.learn(path, true, true) {
		t.Fatal("the path is stored on the third profitable virtual close")
	}

	if !model.learn(path, true, true) {
		t.Fatal("third profitable virtual close should store the path")
	}

	if !model.Promoted[path] {
		t.Fatal("path was not promoted")
	}

	if model.learn(path, true, true) {
		t.Fatal("a later profit should not store the path again")
	}
}

func TestModelRestoresALearnedThreshold(t *testing.T) {
	model := NewModel(context.Background(), nil, nil)
	model.learn("R01/R02", true, true)
	model.learn("R01/R02", true, true)
	model.Open[holdKey("BTC/USD", true)] = "R01/R02"

	value := data.NewValue("model", "state", "current")
	value.Append(model.Threshold, "threshold")
	value.Append(model.Wins, "wins")
	value.Append(model.Promoted, "promoted")
	value.Append(model.Open, "open")
	next := NewModel(context.Background(), nil, nil)
	stored := data.From(value.Bytes())

	for _, gates := range data.Layer[map[string]float64](stored, "threshold") {
		next.Threshold = gates
	}

	for _, wins := range data.Layer[map[string]int](stored, "wins") {
		next.Wins = wins
	}

	for _, promoted := range data.Layer[map[string]bool](stored, "promoted") {
		next.Promoted = promoted
	}

	for _, open := range data.Layer[map[string]string](stored, "open") {
		next.Open = open
	}

	if stored.Err != nil {
		t.Fatal(stored.Err)
	}

	if next.Threshold["R01/R02"] != 0.4 || next.Wins["R01/R02"] != 2 {
		t.Fatalf("restored gate: threshold %v wins %d", next.Threshold["R01/R02"], next.Wins["R01/R02"])
	}

	if next.Open[holdKey("BTC/USD", true)] != "R01/R02" {
		t.Fatalf("restored hold: %q", next.Open[holdKey("BTC/USD", true)])
	}

	if model.Status() != runtime.READY {
		t.Fatalf("model status %s", model.Status())
	}

	next.Note("R01/R02")

	if next.Threshold["R01/R02"] != 0.4 {
		t.Fatalf("noting a stored path reset the gate to %v", next.Threshold["R01/R02"])
	}

	if got := next.Match(nil, []*Token{{Key: "R01", Brightness: 0.4}}); len(got) != 1 {
		t.Fatalf("restored gate should admit 0.4, got %v", got)
	}

	if value.Prefix != thresholdKey {
		t.Fatalf("threshold object key: %s", value.Prefix)
	}
}

func TestModelPersistSafeWithNilClient(t *testing.T) {
	model := NewModel(context.Background(), nil, nil)
	val := data.NewValue("model", "state", "current")
	// Should not panic or error when model.client is nil
	model.persist(val)
	model.persist(nil)

	// When value has an error
	errVal := data.NewValue("too", "few")
	model.persist(errVal)
}

