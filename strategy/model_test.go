package strategy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
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

// regionToken builds a single region token for testing.
func regionToken(key uint8, brightness float64) tokenizer.Token {
	return tokenizer.NewTokenFromRegion(key, brightness)
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

// TestModelExplorePromoteAct verifies the full learning cycle with the
// new win-rate promotion: a new path is explored, enough wins at a
// sufficient win-rate promote it, and the promoted path then acts.
func TestModelExplorePromoteAct(t *testing.T) {
	ctx := context.Background()
	desk := newFakeDesk()
	model := NewModel(ctx, desk, nil, nil)

	const symbol = "BTC/USD"

	// --- Below minPathLength: no action. ---
	model.Evaluate(symbol, regionToken(1, 0.8)) // R01, len 1
	model.Evaluate(symbol, regionToken(2, 0.7)) // R02, len 2

	if desk.Has(symbol, true) {
		t.Fatal("expected no virtual after 2 tokens (below minPathLength)")
	}

	// --- Path reaches minPathLength: virtual entered (high confidence). ---
	model.Evaluate(symbol, regionToken(3, 0.9)) // R03, len 3

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

	// --- Grow path, virtual still open, no action. ---
	model.Evaluate(symbol, regionToken(4, 0.6)) // R04, len 4
	if !desk.Has(symbol, true) {
		t.Fatal("expected virtual still open")
	}

	// --- Trigger ShouldExit: mark moves 3% above entry. ---
	desk.setMark(symbol, decimal.NewFromInt64(103))

	// --- Next tick settles the virtual (win: +3%). ---
	model.Evaluate(symbol, regionToken(5, 0.5))

	if desk.Has(symbol, true) {
		t.Fatal("expected virtual to be settled")
	}

	// --- Cycle 2: same path, second win. ---
	model.Evaluate(symbol, regionToken(1, 0.8))
	model.Evaluate(symbol, regionToken(2, 0.7))
	model.Evaluate(symbol, regionToken(3, 0.9))

	if !desk.Has(symbol, true) {
		t.Fatal("expected virtual re-entered on cycle 2")
	}

	desk.setMark(symbol, decimal.NewFromInt64(102)) // +2% → win
	model.Evaluate(symbol, regionToken(4, 0.5))     // settle

	if desk.Has(symbol, true) {
		t.Fatal("expected virtual settled after cycle 2")
	}

	// --- Cycle 3: third win → promotion (3 wins, 0 losses, winRate 1.0 > 0.5). ---
	model.Evaluate(symbol, regionToken(1, 0.8))
	model.Evaluate(symbol, regionToken(2, 0.7))
	model.Evaluate(symbol, regionToken(3, 0.9))

	if !desk.Has(symbol, true) {
		t.Fatal("expected virtual re-entered on cycle 3")
	}

	desk.setMark(symbol, decimal.NewFromInt64(101)) // +1% → win
	model.Evaluate(symbol, regionToken(4, 0.5))     // settle → promote

	if desk.Has(symbol, true) {
		t.Fatal("expected virtual settled after cycle 3")
	}

	// --- Promoted path: Evaluate returns the stored action. ---
	model.Evaluate(symbol, regionToken(1, 0.8)) // R01, len 1
	model.Evaluate(symbol, regionToken(2, 0.7)) // R02, len 2

	result := model.Evaluate(symbol, regionToken(3, 0.9)) // R03, len 3

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

// TestModelLossThresholdIncrease verifies that a losing path's threshold
// increases, making it harder to re-enter.
func TestModelLossThresholdIncrease(t *testing.T) {
	ctx := context.Background()
	desk := newFakeDesk()
	model := NewModel(ctx, desk, nil, nil)

	const symbol = "ETH/USD"

	// First cycle: enter with confidence 0.5, then lose.
	model.Evaluate(symbol, regionToken(10, 0.5))
	model.Evaluate(symbol, regionToken(11, 0.5))
	model.Evaluate(symbol, regionToken(12, 0.5)) // enter (confidence 0.5 >= threshold 0)

	if !desk.Has(symbol, true) {
		t.Fatal("expected virtual entered")
	}

	desk.setMark(symbol, decimal.NewFromInt64(97)) // -3% → loss
	model.Evaluate(symbol, regionToken(13, 0.3))   // settle

	if desk.Has(symbol, true) {
		t.Fatal("expected virtual settled")
	}

	// After the loss, the threshold increased (0.5 * 1.5 = 0.75).
	// Re-entering requires confidence >= 0.75.
	// Feed a token with confidence 0.5: should NOT re-enter.
	model.Evaluate(symbol, regionToken(10, 0.5))
	model.Evaluate(symbol, regionToken(11, 0.5))
	result := model.Evaluate(symbol, regionToken(12, 0.5)) // confidence 0.5 < 0.75

	_ = result
	if desk.Has(symbol, true) {
		t.Fatal("expected no re-entry: confidence below elevated threshold")
	}

	// Now feed with higher confidence: should re-enter.
	model.Evaluate(symbol, regionToken(10, 0.9))
	model.Evaluate(symbol, regionToken(11, 0.9))
	model.Evaluate(symbol, regionToken(12, 0.9)) // confidence 0.9 >= 0.75

	if !desk.Has(symbol, true) {
		t.Fatal("expected re-entry: confidence above elevated threshold")
	}
}

// TestModelMaxThresholdRetires verifies that repeated losses push the
// threshold to max, retiring the path entirely.
func TestModelMaxThresholdRetires(t *testing.T) {
	ctx := context.Background()
	desk := newFakeDesk()
	model := NewModel(ctx, desk, nil, nil)

	const symbol = "SOL/USD"

	// Start with a high initial threshold so losses push it to max fast.
	for i := 0; i < 5; i++ {
		model.Evaluate(symbol, regionToken(20, 0.9))
		model.Evaluate(symbol, regionToken(21, 0.9))
		model.Evaluate(symbol, regionToken(22, 0.9)) // enter

		if !desk.Has(symbol, true) {
			t.Fatalf("cycle %d: expected virtual entered", i)
		}

		desk.setMark(symbol, decimal.NewFromInt64(95)) // -5% → loss
		model.Evaluate(symbol, regionToken(23, 0.5))   // settle
	}

	// After 5 losses: threshold = 0.9 * 1.5^5 ≈ 7.1, capped at 1.0.
	// Path should be retired. Even max confidence won't re-enter.
	model.Evaluate(symbol, regionToken(20, 1.0))
	model.Evaluate(symbol, regionToken(21, 1.0))
	model.Evaluate(symbol, regionToken(22, 1.0))

	if desk.Has(symbol, true) {
		t.Fatal("expected path to be retired: threshold at max")
	}
}

// TestModelNoopDampener verifies that when multiple prefixes match but
// disagree on action, the result is noop (the dampener).
func TestModelNoopDampener(t *testing.T) {
	ctx := context.Background()
	desk := newFakeDesk()
	model := NewModel(ctx, desk, nil, nil)

	const symbol = "ADA/USD"

	// Manually insert conflicting paths into the tree:
	// R01 → enter
	// R01/R02 → exit
	// These disagree: a path of R01/R02/R03 should yield noop.

	val1 := newDataValue("enter", symbol, "R01")
	model.tree, _, _ = model.tree.Insert([]byte("R01"), val1)

	val2 := newDataValue("exit", symbol, "R01/R02")
	model.tree, _, _ = model.tree.Insert([]byte("R01/R02"), val2)

	// Evaluate: path R01/R02/R03 matches both R01 (enter) and R01/R02 (exit).
	// They disagree → noop.
	model.Evaluate(symbol, regionToken(1, 0.9))
	model.Evaluate(symbol, regionToken(2, 0.9))
	result := model.Evaluate(symbol, regionToken(3, 0.9))

	if result == nil {
		t.Fatal("expected noop result from disagreement, got nil")
	}
	if result.Role != "noop" {
		t.Fatalf("role = %q, want %q (disagreement dampener)", result.Role, "noop")
	}

	// No position should be opened on a noop.
	if desk.Has(symbol, true) {
		t.Fatal("expected no position on noop")
	}
}

// TestModelAgreementYieldsAction verifies that when all matching prefixes
// agree, the action is returned.
func TestModelAgreementYieldsAction(t *testing.T) {
	ctx := context.Background()
	desk := newFakeDesk()
	model := NewModel(ctx, desk, nil, nil)

	const symbol = "DOGE/USD"

	// Insert agreeing paths:
	// R01 → enter
	// R01/R02 → enter
	// R01/R02/R03 → enter

	val1 := newDataValue("enter", symbol, "R01")
	model.tree, _, _ = model.tree.Insert([]byte("R01"), val1)

	val2 := newDataValue("enter", symbol, "R01/R02")
	model.tree, _, _ = model.tree.Insert([]byte("R01/R02"), val2)

	val3 := newDataValue("enter", symbol, "R01/R02/R03")
	model.tree, _, _ = model.tree.Insert([]byte("R01/R02/R03"), val3)

	// Evaluate: path R01/R02/R03 matches all three, all agree → enter.
	model.Evaluate(symbol, regionToken(1, 0.9))
	model.Evaluate(symbol, regionToken(2, 0.9))
	result := model.Evaluate(symbol, regionToken(3, 0.9))

	if result == nil {
		t.Fatal("expected enter result from agreement, got nil")
	}
	if result.Role != "enter" {
		t.Fatalf("role = %q, want %q", result.Role, "enter")
	}

	if !desk.Has(symbol, true) {
		t.Fatal("expected position entered")
	}
}

// TestModelPathReset verifies that the path resets after settlement and
// a new exploration starts from the next token.
func TestModelPathReset(t *testing.T) {
	ctx := context.Background()
	desk := newFakeDesk()
	model := NewModel(ctx, desk, nil, nil)

	const symbol = "AVAX/USD"

	// Build path, enter virtual.
	model.Evaluate(symbol, regionToken(1, 0.9))
	model.Evaluate(symbol, regionToken(2, 0.9))
	model.Evaluate(symbol, regionToken(3, 0.9))

	if !desk.Has(symbol, true) {
		t.Fatal("expected virtual entered")
	}

	// Grow, trigger exit, settle.
	model.Evaluate(symbol, regionToken(4, 0.5))
	desk.setMark(symbol, decimal.NewFromInt64(103))
	model.Evaluate(symbol, regionToken(5, 0.5)) // settle, path resets

	if desk.Has(symbol, true) {
		t.Fatal("expected virtual settled")
	}

	// After reset, next token starts fresh (len 1 < minPathLength).
	model.Evaluate(symbol, regionToken(6, 0.9))

	if desk.Has(symbol, true) {
		t.Fatal("expected no virtual after path reset (len 1)")
	}
}

// TestModelMaxPathLength verifies path trimming.
func TestModelMaxPathLength(t *testing.T) {
	ctx := context.Background()
	desk := newFakeDesk()
	model := NewModel(ctx, desk, nil, nil)

	const symbol = "DOT/USD"

	// Feed maxPathLength + 4 tokens without triggering exit.
	for i := uint8(1); i <= uint8(maxPathLength+4); i++ {
		model.Evaluate(symbol, regionToken(i, 0.5))
	}

	// The virtual was entered at token 3. It's still open (mark never moved).
	if !desk.Has(symbol, true) {
		t.Fatal("expected virtual to still be open")
	}

	pos := desk.Virtual(symbol)
	if pos == nil {
		t.Fatal("expected non-nil virtual")
	}

	// Entry path should be the first minPathLength tokens.
	if pos.EntryPath != "R01/R02/R03" {
		t.Fatalf("entry path = %q, want %q", pos.EntryPath, "R01/R02/R03")
	}
}

// TestModelChordToken verifies that multi-symbol tokens (chords) work
// as single trie edges.
func TestModelChordToken(t *testing.T) {
	ctx := context.Background()
	desk := newFakeDesk()
	model := NewModel(ctx, desk, nil, nil)

	const symbol = "LINK/USD"

	// A chord token: two regions as one token.
	chord := tokenizer.NewToken([]string{"R01", "R02"}, 0.8, 0.8, 0)

	if chord.String() != "[R01-R02]" {
		t.Fatalf("chord string = %q, want %q", chord.String(), "[R01-R02]")
	}

	model.Evaluate(symbol, chord)                         // [R01-R02], len 1
	model.Evaluate(symbol, tokenizer.NewTokenFromRegion(3, 0.7)) // R03, len 2
	model.Evaluate(symbol, tokenizer.NewTokenFromRegion(4, 0.6)) // R04, len 3

	if !desk.Has(symbol, true) {
		t.Fatal("expected virtual after 3 tokens")
	}

	pos := desk.Virtual(symbol)
	if pos.EntryPath != "[R01-R02]/R03/R04" {
		t.Fatalf("entry path = %q, want %q", pos.EntryPath, "[R01-R02]/R03/R04")
	}
}

// TestModelDemotion verifies that a promoted path that keeps losing is
// demoted back to explored.
func TestModelDemotion(t *testing.T) {
	ctx := context.Background()
	desk := newFakeDesk()
	model := NewModel(ctx, desk, nil, nil)

	const symbol = "LTC/USD"

	// Manually insert a promoted path.
	val := newDataValue("enter", symbol, "R10/R11/R12")
	model.tree, _, _ = model.tree.Insert([]byte("R10/R11/R12"), val)

	// Three losing cycles on the same path → demotion.
	for i := 0; i < demotionLosses; i++ {
		model.Evaluate(symbol, regionToken(10, 0.9))
		model.Evaluate(symbol, regionToken(11, 0.9))
		model.Evaluate(symbol, regionToken(12, 0.9)) // enter from tree

		if !desk.Has(symbol, true) {
			t.Fatalf("cycle %d: expected virtual entered", i)
		}

		desk.setMark(symbol, decimal.NewFromInt64(95)) // -5% → loss
		model.Evaluate(symbol, regionToken(13, 0.3))   // settle
	}

	// After 3 losses, the path should be demoted from the current tree.
	// Next cycle: it's in explored now, with elevated threshold.
	// With confidence 0.9 and threshold 0.8, it should still enter.
	model.Evaluate(symbol, regionToken(10, 0.9))
	model.Evaluate(symbol, regionToken(11, 0.9))
	model.Evaluate(symbol, regionToken(12, 0.9))

	// It enters (from explored, not from the current tree).
	if !desk.Has(symbol, true) {
		t.Fatal("expected re-entry from explored after demotion")
	}

	// But the tree should no longer have the path.
	_, found := model.tree.Get([]byte("R10/R11/R12"))
	if found {
		t.Fatal("expected path to be demoted from current tree")
	}
}

// newDataValue constructs a data.Value with the given role, scope, and path
// for direct tree insertion in tests.
func newDataValue(role, scope, path string) *data.Value {
	val := data.NewValue("training", role, scope)
	val.Append(path, "path")
	val.Append(0, "losses")
	return val
}

type hubCapture struct {
	payloads [][]byte
}

func (capture *hubCapture) Write(payload []byte) (int, error) {
	copied := make([]byte, len(payload))
	copy(copied, payload)
	capture.payloads = append(capture.payloads, copied)
	return len(payload), nil
}

// TestPublishGrammarFrame verifies Evaluate emits a sequitur/grammar frame
// the dashboard ingests as a SequiturFrame.
func TestPublishGrammarFrame(t *testing.T) {
	ctx := context.Background()
	capture := &hubCapture{}
	model := NewModel(ctx, newFakeDesk(), nil, capture)

	model.Evaluate("BTC/USD", regionToken(1, 0.8))

	var grammar *data.Value
	for _, payload := range capture.payloads {
		value := data.New()
		if _, err := value.ReadFrom(bytes.NewReader(payload)); err != nil {
			t.Fatalf("decode hub payload: %v", err)
		}
		if value.Origin == "sequitur" {
			grammar = value
		}
	}

	if grammar == nil {
		t.Fatal("Evaluate did not publish a sequitur frame")
	}

	var frame map[string]any
	grammar.Lookup(&frame, "grammar")
	if frame == nil {
		t.Fatal("sequitur frame missing grammar layer")
	}

	raw, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal grammar layer: %v", err)
	}

	if frame["symbol"] != "BTC/USD" {
		t.Fatalf("symbol = %v, want BTC/USD", frame["symbol"])
	}
	if _, ok := frame["policy"].(map[string]any); !ok {
		t.Fatalf("policy missing: %s", raw)
	}
	t.Logf("grammar layer: %s", raw)
}
