package strategy

import (
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
)

func TestVirtualProfitOpensThePathForEverySymbol(t *testing.T) {
	training := &Training{}
	path := "R01/R05"

	if !training.beginVirtual(virtualLot{Path: path, Symbol: "TEL/USD", Brightness: []float64{0.6, 0.5}}) {
		t.Fatal("virtual did not open")
	}

	training.applyVirtualReturn(path, 0.50)

	if training.virtualOpen(path) {
		t.Fatal("a 50 percent gain left the virtual open")
	}

	if training.pathAction(path) != "enter" {
		t.Fatalf("action = %q, want enter", training.pathAction(path))
	}

	if training.admitToken([]string{"R01"}, "R05", 0.5, 0.9) == false {
		t.Fatal("the brightness that opened the path cannot walk the next step")
	}
}

func TestVirtualLossClearsThePath(t *testing.T) {
	training := &Training{}
	path := "R01/R05"
	training.setPathAction(path, "enter")
	training.beginVirtual(virtualLot{Path: path, Symbol: "BTC/USD"})

	training.applyVirtualReturn(path, -0.25)

	if training.pathAction(path) != "" {
		t.Fatalf("action = %q, want no action", training.pathAction(path))
	}

	if training.virtualOpen(path) {
		t.Fatal("a 25 percent loss left the virtual open")
	}
}

func TestVirtualInsideTheBandStaysOpen(t *testing.T) {
	training := &Training{}
	path := "R02/R10"
	training.beginVirtual(virtualLot{Path: path, Symbol: "TEL/USD"})
	training.setPathAction(path, "exit")

	training.applyVirtualReturn(path, 0.20)

	if !training.virtualOpen(path) {
		t.Fatal("a gain inside the band closed the virtual")
	}

	if training.pathAction(path) != "exit" {
		t.Fatalf("action = %q, want exit", training.pathAction(path))
	}
}

func TestAPathWithNoActionStillOpensAVirtual(t *testing.T) {
	training := &Training{}
	path := "R04/R09"

	if !training.beginVirtual(virtualLot{Path: path, Symbol: "TEL/USD"}) {
		t.Fatal("a path with no action refused a virtual")
	}

	if training.beginVirtual(virtualLot{Path: path, Symbol: "BTC/USD"}) {
		t.Fatal("a second symbol opened another virtual on the same path")
	}
}

func TestProfitLowersTheNextStep(t *testing.T) {
	training := &Training{}
	path := "R01/R05"
	training.setPathAction(path, "enter")
	training.seedThreshold(path, []float64{0.5, 0.5})

	if training.admitToken([]string{"R01"}, "R05", 0.45, 0.2) {
		t.Fatal("a token under the step threshold was appended")
	}

	training.strengthenPath(path, nil)

	if !training.admitToken([]string{"R01"}, "R05", 0.45, 0.2) {
		t.Fatal("profit did not make the next step easier")
	}

	if training.admitToken([]string{"R01"}, "R09", 0.1, 0.2) {
		t.Fatal("a token that does not extend the path passed on the lowered bar")
	}
}

func TestALosingCloseLeavesThePath(t *testing.T) {
	training := &Training{}
	training.setPathAction("R13/R04", "enter")

	if err := training.noteClosed(roundTrip("TEL/USD", "R13/R04", 8)); err != nil {
		t.Fatalf("note closed: %v", err)
	}

	if training.pathAction("R13/R04") != "enter" {
		t.Fatalf("action = %q", training.pathAction("R13/R04"))
	}
}

func TestAWinningCloseLowersTheStep(t *testing.T) {
	training := &Training{}
	training.setPathAction("R13/R04", "enter")
	training.seedThreshold("R13/R04", []float64{0.5, 0.5})

	if err := training.noteClosed(roundTrip("TEL/USD", "R13/R04", 13)); err != nil {
		t.Fatalf("win: %v", err)
	}

	if !training.admitToken([]string{"R13"}, "R04", 0.45, 0.2) {
		t.Fatal("a profitable enter did not lower the next step")
	}
}

func TestStoredVirtualKeepsTokensAndOpened(t *testing.T) {
	opened := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	lot := virtualLot{
		Path:       "R01/R05",
		Symbol:     "TEL/USD",
		Brightness: []float64{0.4, 0.2},
		Tokens:     []string{"R01", "R05"},
		Opened:     opened,
		Quantity:   decimal.NewFromFloat64(1),
		Entry:      decimal.NewFromFloat64(2),
		Fee:        decimal.NewFromFloat64(0.01),
	}
	record := storedVirtual{
		Path:       lot.Path,
		Symbol:     lot.Symbol,
		Brightness: lot.Brightness,
		Tokens:     append([]string{}, lot.Tokens...),
		Opened:     lot.Opened.UTC().Format(time.RFC3339Nano),
		Quantity:   lot.Quantity.String(),
		Entry:      lot.Entry.String(),
		Fee:        lot.Fee.String(),
	}
	raw, err := sonic.Marshal(record)

	if err != nil {
		t.Fatal(err)
	}

	value := data.NewValue("policy", "virtual", lot.Symbol)
	value.FormatPrefix()
	value.AppendJSON(raw, value.Prefix+"/"+data.LayerLedger)

	got, ok := virtualFrom(value)

	if !ok {
		t.Fatal("stored virtual did not load")
	}

	if got.Opened.UTC() != opened || len(got.Tokens) != 2 || got.Tokens[0] != "R01" || got.Tokens[1] != "R05" {
		t.Fatalf("restored opened %s tokens %v", got.Opened, got.Tokens)
	}
}

func roundTrip(symbol, path string, sell float64) *broker.Position {
	position := broker.NewPosition()
	position.Enter(symbol, decimal.NewFromFloat64(2), path)
	position.Apply(kraken.ExecutionData{
		Symbol:      symbol,
		Side:        "buy",
		ExecType:    "trade",
		CumQty:      decimal.NewFromFloat64(2),
		LastQty:     decimal.NewFromFloat64(2),
		LastPrice:   decimal.NewFromFloat64(10),
		FeeUsdEquiv: decimal.NewFromFloat64(0.25),
	})
	position.Exit(symbol)
	position.Apply(kraken.ExecutionData{
		Symbol:      symbol,
		Side:        "sell",
		ExecType:    "trade",
		LastQty:     decimal.NewFromFloat64(2),
		LastPrice:   decimal.NewFromFloat64(sell),
		FeeUsdEquiv: decimal.NewFromFloat64(0.25),
	})

	return position
}
