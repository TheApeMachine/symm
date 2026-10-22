package strategy

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
)

// ---------------------------------------------------------------------------
// fakeDesk — in-memory Desk for tests
// ---------------------------------------------------------------------------

type fakeDesk struct {
	mu     sync.Mutex
	open   map[string]*broker.Position
	closed map[string]*broker.Position
}

func newFakeDesk() *fakeDesk {
	return &fakeDesk{
		open:   make(map[string]*broker.Position),
		closed: make(map[string]*broker.Position),
	}
}

func (d *fakeDesk) Has(symbol string, _ bool) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.open[symbol]
	return ok
}

func (d *fakeDesk) Enter(symbol, path string, _ bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.open[symbol]; exists {
		return fmt.Errorf("position already exists for %s", symbol)
	}

	pos := &broker.Position{
		Symbol:     symbol,
		EntryPath:  path,
		Virtual:    true,
		EntryPrice: decimal.NewFromInt64(100),
		Mark:       decimal.NewFromInt64(100),
		OpenedAt:   time.Now().UTC(),
	}
	d.open[symbol] = pos
	return nil
}

func (d *fakeDesk) Exit(symbol, path string, _ bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	pos, ok := d.open[symbol]
	if !ok {
		return fmt.Errorf("position not found for %s", symbol)
	}

	now := time.Now().UTC()
	pos.ClosedAt = &now
	pos.ExitPath = path

	if pos.Mark != nil && pos.EntryPrice != nil && pos.EntryPrice.Sign() > 0 {
		ret := (pos.Mark.Float64() / pos.EntryPrice.Float64() - 1) * 100
		pos.Return = &ret
	}

	delete(d.open, symbol)
	d.closed[symbol] = pos
	return nil
}

func (d *fakeDesk) Virtual(symbol string) *broker.Position {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.open[symbol]
}

func (d *fakeDesk) Positions(open bool) []*broker.Position {
	d.mu.Lock()
	defer d.mu.Unlock()

	src := d.closed
	if open {
		src = d.open
	}

	out := make([]*broker.Position, 0, len(src))
	for _, p := range src {
		out = append(out, p)
	}
	return out
}

func (d *fakeDesk) Profit(symbol string, _ bool) (bool, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	pos := d.open[symbol]
	if pos == nil || pos.Return == nil {
		return false, false
	}
	return *pos.Return > 0, true
}

func (d *fakeDesk) ClosedVirtuals() []*broker.Position {
	d.mu.Lock()
	defer d.mu.Unlock()

	out := make([]*broker.Position, 0)
	for _, p := range d.closed {
		if p.ClosedAt != nil {
			out = append(out, p)
		}
	}
	return out
}

func (d *fakeDesk) Drop(symbol string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.open, symbol)
	delete(d.closed, symbol)
}

// setMark updates the mark on an open virtual (test helper).
func (d *fakeDesk) setMark(symbol string, mark *decimal.Decimal) {
	d.mu.Lock()
	pos, ok := d.open[symbol]
	d.mu.Unlock()

	if !ok {
		return
	}
	pos.Update(mark)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// tokenValue builds a data.Value carrying one region token for the symbol.
func tokenValue(symbol string, regionKey uint8) *data.Value {
	region := &nmstore.Region{
		Symbol:     symbol,
		Key:        regionKey,
		Brightness: 0.5,
	}
	token := tokenizer.NewToken(region)
	return data.NewValue("training", "update", symbol).Append(token, "token")
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

// TestModelExplorePromoteAct verifies the full learning cycle:
// a new path is explored, three profitable closes promote it into the
// policy tree, and the promoted path then acts without re-exploring.
func TestModelExplorePromoteAct(t *testing.T) {
	ctx := context.Background()
	desk := newFakeDesk()
	model := NewModel(ctx, desk, nil, nil)

	const symbol = "BTC/USD"

	// --- Path accumulates below minPathLength: no action. ---
	model.Evaluate(tokenValue(symbol, 1)) // R01, len 1
	model.Evaluate(tokenValue(symbol, 2)) // R02, len 2

	if desk.Has(symbol, true) {
		t.Fatal("expected no virtual after 2 tokens (below minPathLength)")
	}

	// --- Path reaches minPathLength: virtual entered, path explored. ---
	model.Evaluate(tokenValue(symbol, 3)) // R03, len 3

	if !desk.Has(symbol, true) {
		t.Fatal("expected virtual after 3 tokens")
	}

	pos := desk.Virtual(symbol)
	if pos == nil {
		t.Fatal("expected non-nil virtual")
	}
	if pos.EntryPath != "R01/R02/R03" {
		t.Fatalf("entry path = %q, want %q", pos.EntryPath, "R01/R02/R03")
	}

	// --- Path grows, virtual still open, no action. ---
	model.Evaluate(tokenValue(symbol, 4)) // R04, len 4
	if !desk.Has(symbol, true) {
		t.Fatal("expected virtual still open")
	}

	// --- Trigger ShouldExit: mark moves 3% above entry. ---
	desk.setMark(symbol, decimal.NewFromInt64(103))

	// --- Next tick settles the virtual (win: +3%). ---
	model.Evaluate(tokenValue(symbol, 5))

	if desk.Has(symbol, true) {
		t.Fatal("expected virtual to be settled and dropped")
	}

	// --- Cycle 2: same path, second win. ---
	model.Evaluate(tokenValue(symbol, 1)) // R01, len 1 (reset)
	model.Evaluate(tokenValue(symbol, 2)) // R02, len 2
	model.Evaluate(tokenValue(symbol, 3)) // R03, len 3 → re-enter (explored)

	if !desk.Has(symbol, true) {
		t.Fatal("expected virtual re-entered on cycle 2")
	}

	desk.setMark(symbol, decimal.NewFromInt64(102)) // +2% → win
	model.Evaluate(tokenValue(symbol, 4))          // settle

	if desk.Has(symbol, true) {
		t.Fatal("expected virtual settled after cycle 2")
	}

	// --- Cycle 3: same path, third win → promotion. ---
	model.Evaluate(tokenValue(symbol, 1))
	model.Evaluate(tokenValue(symbol, 2))
	model.Evaluate(tokenValue(symbol, 3)) // R03 → re-enter

	if !desk.Has(symbol, true) {
		t.Fatal("expected virtual re-entered on cycle 3")
	}

	desk.setMark(symbol, decimal.NewFromInt64(101)) // +1% → win
	model.Evaluate(tokenValue(symbol, 4))          // settle → promote

	if desk.Has(symbol, true) {
		t.Fatal("expected virtual settled after cycle 3")
	}

	// --- Promoted path: Evaluate returns the stored action. ---
	model.Evaluate(tokenValue(symbol, 1)) // R01, len 1
	model.Evaluate(tokenValue(symbol, 2)) // R02, len 2

	result := model.Evaluate(tokenValue(symbol, 3)) // R03, len 3

	if result == nil {
		t.Fatal("expected Evaluate to return the promoted action, got nil")
	}
	if result.Role != "enter" {
		t.Fatalf("role = %q, want %q", result.Role, "enter")
	}

	if !desk.Has(symbol, true) {
		t.Fatal("expected virtual entered from promoted path")
	}
}

// TestModelLossNotPromoted verifies that a losing path is not promoted
// into the policy tree. The path stays in explored and re-enters on
// recurrence, but Evaluate returns nil (no stored action).
func TestModelLossNotPromoted(t *testing.T) {
	ctx := context.Background()
	desk := newFakeDesk()
	model := NewModel(ctx, desk, nil, nil)

	const symbol = "ETH/USD"

	// One losing cycle on path R10/R11/R12.
	model.Evaluate(tokenValue(symbol, 10))
	model.Evaluate(tokenValue(symbol, 11))
	model.Evaluate(tokenValue(symbol, 12)) // enter

	if !desk.Has(symbol, true) {
		t.Fatal("expected virtual entered")
	}

	desk.setMark(symbol, decimal.NewFromInt64(97)) // -3% → loss
	model.Evaluate(tokenValue(symbol, 13))       // settle

	if desk.Has(symbol, true) {
		t.Fatal("expected virtual settled")
	}

	// Next cycle: same path re-enters (explored), Evaluate returns nil.
	model.Evaluate(tokenValue(symbol, 10))
	model.Evaluate(tokenValue(symbol, 11))
	result := model.Evaluate(tokenValue(symbol, 12))

	if result != nil {
		t.Fatalf("expected nil (explored, not promoted), got role=%q", result.Role)
	}

	if !desk.Has(symbol, true) {
		t.Fatal("expected virtual re-entered for continued exploration")
	}
}

// TestModelMaxLossesRetires verifies that a path which loses maxLosses
// times in a row is retired: it is no longer re-entered.
func TestModelMaxLossesRetires(t *testing.T) {
	ctx := context.Background()
	desk := newFakeDesk()
	model := NewModel(ctx, desk, nil, nil)

	const symbol = "SOL/USD"

	// Three losing cycles on the same path.
	for i := 0; i < maxLosses; i++ {
		model.Evaluate(tokenValue(symbol, 20))
		model.Evaluate(tokenValue(symbol, 21))
		model.Evaluate(tokenValue(symbol, 22)) // enter / re-enter

		if !desk.Has(symbol, true) {
			t.Fatalf("cycle %d: expected virtual entered", i)
		}

		desk.setMark(symbol, decimal.NewFromInt64(95)) // -5% → loss
		model.Evaluate(tokenValue(symbol, 23))       // settle
	}

	// Fourth cycle: path is retired, no entry.
	model.Evaluate(tokenValue(symbol, 20))
	model.Evaluate(tokenValue(symbol, 21))
	model.Evaluate(tokenValue(symbol, 22))

	if desk.Has(symbol, true) {
		t.Fatal("expected path to be retired after maxLosses consecutive losses")
	}
}

// TestModelPathReset verifies that the path resets after settlement and
// a new exploration starts from the next token.
func TestModelPathReset(t *testing.T) {
	ctx := context.Background()
	desk := newFakeDesk()
	model := NewModel(ctx, desk, nil, nil)

	const symbol = "ADA/USD"

	// Build path R01/R02/R03, enter virtual.
	model.Evaluate(tokenValue(symbol, 1))
	model.Evaluate(tokenValue(symbol, 2))
	model.Evaluate(tokenValue(symbol, 3))

	if !desk.Has(symbol, true) {
		t.Fatal("expected virtual entered")
	}

	// Grow the path to R01/R02/R03/R04/R05.
	model.Evaluate(tokenValue(symbol, 4))
	model.Evaluate(tokenValue(symbol, 5))

	// Trigger exit and settle.
	desk.setMark(symbol, decimal.NewFromInt64(103))
	model.Evaluate(tokenValue(symbol, 6)) // settle, path resets

	if desk.Has(symbol, true) {
		t.Fatal("expected virtual settled")
	}

	// After reset, the next token starts a fresh path of length 1.
	model.Evaluate(tokenValue(symbol, 7)) // R07, len 1

	// Still below minPathLength: no virtual.
	if desk.Has(symbol, true) {
		t.Fatal("expected no virtual after path reset (len 1 < minPathLength)")
	}
}

// TestModelMaxPathLength verifies that the path is trimmed to
// maxPathLength when it grows beyond the bound.
func TestModelMaxPathLength(t *testing.T) {
	ctx := context.Background()
	desk := newFakeDesk()
	model := NewModel(ctx, desk, nil, nil)

	const symbol = "DOGE/USD"

	// Feed maxPathLength + 4 distinct tokens. The path should be
	// trimmed to the last maxPathLength tokens.
	for i := uint8(1); i <= uint8(maxPathLength+4); i++ {
		model.Evaluate(tokenValue(symbol, i))
	}

	// The virtual was entered when the path first reached minPathLength
	// (token 3, path "R01/R02/R03"). It's still open (mark never moved).
	if !desk.Has(symbol, true) {
		t.Fatal("expected virtual to still be open")
	}

	pos := desk.Virtual(symbol)
	if pos == nil {
		t.Fatal("expected non-nil virtual")
	}

	// The entry path should be the first minPathLength tokens.
	if pos.EntryPath != "R01/R02/R03" {
		t.Fatalf("entry path = %q, want %q", pos.EntryPath, "R01/R02/R03")
	}
}
