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

type scriptedClose struct {
	ret float64
	ok  bool
}

type fakeDesk struct {
	mu         sync.Mutex
	open       map[string]*broker.Position
	closed     map[string]*broker.Position
	closes     []scriptedClose
	quotes     int
	entries    int
	venue      int
	exits      int
	venueExits int
}

func newFakeDesk() *fakeDesk {
	return &fakeDesk{
		open:   make(map[string]*broker.Position),
		closed: make(map[string]*broker.Position),
	}
}

func (d *fakeDesk) Has(symbol string, virtual bool) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	position, ok := d.open[symbol]

	return ok && position.Virtual == virtual
}

func (d *fakeDesk) Enter(symbol, path string, virtual bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.open[symbol]; exists {
		return fmt.Errorf("position already exists for %s", symbol)
	}

	d.entries++
	if !virtual {
		d.venue++
	}
	d.open[symbol] = &broker.Position{
		Symbol:     symbol,
		EntryPath:  path,
		Virtual:    virtual,
		Qty:        decimal.NewFromInt64(1),
		EntryPrice: decimal.NewFromInt64(100),
		EntryFee:   decimal.NewFromInt64(1),
		Mark:       decimal.NewFromInt64(100),
		OpenedAt:   time.Now().UTC(),
	}

	return nil
}

func (d *fakeDesk) Exit(symbol, path string, _ bool) error {
	return d.ExitOccupied(symbol, path)
}

func (d *fakeDesk) Virtual(symbol string) *broker.Position {
	d.mu.Lock()
	defer d.mu.Unlock()

	position := d.open[symbol]
	if position == nil || !position.Virtual {
		return nil
	}

	return position
}

func (d *fakeDesk) Positions(open bool) []*broker.Position {
	d.mu.Lock()
	defer d.mu.Unlock()

	src := d.closed
	if open {
		src = d.open
	}

	out := make([]*broker.Position, 0, len(src))
	for _, position := range src {
		out = append(out, position)
	}

	return out
}

func (d *fakeDesk) Profit(symbol string, _ bool) (bool, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	position := d.open[symbol]
	if position == nil || position.Return == nil {
		return false, false
	}

	return *position.Return > 0, true
}

func (d *fakeDesk) ClosedVirtuals() []*broker.Position {
	d.mu.Lock()
	defer d.mu.Unlock()

	out := make([]*broker.Position, 0)
	for _, position := range d.closed {
		if position.Virtual && position.ClosedAt != nil {
			out = append(out, position)
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

func (d *fakeDesk) Quote(string) (qty, entry, fee *decimal.Decimal, err error) {
	d.mu.Lock()
	d.quotes++
	d.mu.Unlock()

	return decimal.NewFromInt64(1), decimal.NewFromInt64(100), decimal.NewFromInt64(1), nil
}

func (d *fakeDesk) Realize(string, *decimal.Decimal, *decimal.Decimal, *decimal.Decimal) (float64, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.closes) == 0 {
		return 0.01, true
	}

	next := d.closes[0]
	d.closes = d.closes[1:]

	return next.ret, next.ok
}

func (d *fakeDesk) Occupied(symbol string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.open[symbol]

	return ok
}

func (d *fakeDesk) Open(symbol string) *broker.Position {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.open[symbol]
}

func (d *fakeDesk) ExitOccupied(symbol, path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	position, ok := d.open[symbol]
	if !ok {
		return fmt.Errorf("position not found for %s", symbol)
	}

	now := time.Now().UTC()
	position.ClosedAt = &now
	position.ExitPath = path
	delete(d.open, symbol)
	d.closed[symbol] = position
	d.exits++
	if !position.Virtual {
		d.venueExits++
	}

	return nil
}

func (d *fakeDesk) queue(closes ...scriptedClose) {
	d.mu.Lock()
	d.closes = append(d.closes, closes...)
	d.mu.Unlock()
}

func regionToken(key uint8, brightness float64) tokenizer.Token {
	return tokenizer.NewTokenFromRegion(key, brightness)
}

// sampleOneExcursion feeds the entry precursor R00/R01/R02 and the exit
// region R03. After the first sample, a foreign region has to arrive
// before the same precursor can be sampled again.
func sampleOneExcursion(model *Model, symbol string, prior int) {
	if prior > 0 {
		model.Evaluate(symbol, regionToken(6, 0.2))
	}

	model.Evaluate(symbol, regionToken(0, 0.9))
	model.Evaluate(symbol, regionToken(1, 0.9))
	model.Evaluate(symbol, regionToken(2, 0.9))
	model.Evaluate(symbol, regionToken(3, 0.9))
}

func TestLedgerDoesNotPromoteOnTwoWins(t *testing.T) {
	desk := newFakeDesk()
	model := NewModel(context.Background(), desk, nil, nil, nil)
	const symbol = "BTC/USD"

	sampleOneExcursion(model, symbol, 0)
	sampleOneExcursion(model, symbol, 1)

	if _, found := model.tree.Get([]byte("R00/R01/R02")); found {
		t.Fatal("two winning excursions wrote the path into the current policy")
	}

	if desk.venue != 0 {
		t.Fatalf("venue entries = %d, want 0 while the path is still a ledger", desk.venue)
	}

	if closed := desk.closed[symbol]; closed == nil || !closed.Virtual {
		t.Fatal("ledger sample did not settle a virtual lot")
	}

	if desk.quotes != 2 {
		t.Fatalf("quotes = %d, want 2 ledger samples", desk.quotes)
	}
}

func TestConfirmedPathOpensAVenueOrder(t *testing.T) {
	desk := newFakeDesk()
	model := NewModel(context.Background(), desk, nil, nil, nil)
	const symbol = "BTC/USD"

	cfg := LoadConfig()
	for i := 0; i < cfg.FreezeAfter+cfg.ConfirmAfter; i++ {
		sampleOneExcursion(model, symbol, i)
	}

	if _, found := model.tree.Get([]byte("R00/R01/R02")); !found {
		t.Fatal("confirmed entry path missing from the current policy")
	}

	if _, found := model.tree.Get([]byte("R00/R01/R02/R03")); !found {
		t.Fatal("confirmed exit path missing from the current policy")
	}

	if desk.venue != 0 {
		t.Fatalf("venue entries during confirmation = %d, want 0", desk.venue)
	}

	sampleOneExcursion(model, symbol, cfg.FreezeAfter+cfg.ConfirmAfter)

	if desk.venue != 1 {
		t.Fatalf("venue entries = %d, want 1", desk.venue)
	}

	if desk.venueExits != 1 {
		t.Fatalf("venue exits = %d, want 1", desk.venueExits)
	}

	closed := desk.closed[symbol]
	if closed == nil {
		t.Fatal("venue round trip was not closed")
	}

	if closed.Virtual {
		t.Fatal("confirmed path opened a virtual lot")
	}
}

func TestSameTrendIsOneSample(t *testing.T) {
	desk := newFakeDesk()
	model := NewModel(context.Background(), desk, nil, nil, nil)
	const symbol = "ETH/USD"

	sampleOneExcursion(model, symbol, 0)
	model.Evaluate(symbol, regionToken(0, 0.9))
	model.Evaluate(symbol, regionToken(1, 0.9))
	model.Evaluate(symbol, regionToken(2, 0.9))

	if desk.quotes != 1 {
		t.Fatalf("quotes = %d, want 1 while the sentence has not changed", desk.quotes)
	}
}

func TestNegativeMeanRetiresThePath(t *testing.T) {
	desk := newFakeDesk()
	model := NewModel(context.Background(), desk, nil, nil, nil)
	const symbol = "SOL/USD"
	cfg := LoadConfig()

	for i := 0; i < cfg.FreezeAfter; i++ {
		desk.queue(scriptedClose{ret: -0.02, ok: true})
		sampleOneExcursion(model, symbol, i)
	}

	quotes := desk.quotes
	sampleOneExcursion(model, symbol, cfg.FreezeAfter)

	if desk.quotes != quotes {
		t.Fatalf("quotes went from %d to %d after the path was retired", quotes, desk.quotes)
	}
}

func TestBookMissDoesNotScore(t *testing.T) {
	desk := newFakeDesk()
	model := NewModel(context.Background(), desk, nil, nil, nil)
	const symbol = "ADA/USD"

	desk.queue(scriptedClose{ok: false})
	sampleOneExcursion(model, symbol, 0)

	if _, found := model.explored.Get([]byte(exploredKey("enter", "R00/R01/R02"))); found {
		t.Fatal("a close the book could not take was recorded")
	}

	model.Evaluate(symbol, regionToken(3, 0.9))

	if _, found := model.explored.Get([]byte(exploredKey("enter", "R00/R01/R02"))); !found {
		t.Fatal("retry against a book that can take the size left no score")
	}
}

func TestLongestMatchExitOverridesShorterEnter(t *testing.T) {
	desk := newFakeDesk()
	model := NewModel(context.Background(), desk, nil, nil, nil)
	const symbol = "DOGE/USD"

	model.tree, _, _ = model.tree.Insert(
		[]byte("R00/R01/R02"), newDataValue("enter", symbol, "R00/R01/R02"),
	)
	model.tree, _, _ = model.tree.Insert(
		[]byte("R00/R01/R02/R03"), newDataValue("exit", symbol, "R00/R01/R02/R03"),
	)

	model.Evaluate(symbol, regionToken(0, 0.9))
	model.Evaluate(symbol, regionToken(1, 0.9))
	entered := model.Evaluate(symbol, regionToken(2, 0.9))
	exited := model.Evaluate(symbol, regionToken(3, 0.9))

	if entered == nil || entered.Role != "enter" {
		t.Fatalf("enter result = %v, want enter", entered)
	}

	if exited == nil || exited.Role != "exit" {
		t.Fatalf("exit result = %v, want the longer exit", exited)
	}

	if desk.Occupied(symbol) {
		t.Fatal("longer exit left the venue lot open")
	}
}

func TestModelChordToken(t *testing.T) {
	desk := newFakeDesk()
	model := NewModel(context.Background(), desk, nil, nil, nil)
	const symbol = "LINK/USD"

	chord := tokenizer.NewToken([]string{"R00", "R01"}, 0.8, 0.8, 0)
	if chord.String() != "[R00-R01]" {
		t.Fatalf("chord string = %q, want [R00-R01]", chord.String())
	}

	model.Evaluate(symbol, chord)
	model.Evaluate(symbol, regionToken(2, 0.7))
	model.Evaluate(symbol, regionToken(0, 0.6))

	if desk.venue != 0 {
		t.Fatal("chord path took the venue slot before confirmation")
	}

	if !desk.Has(symbol, true) {
		t.Fatal("chord path did not open a virtual lot")
	}

	if desk.quotes != 1 {
		t.Fatalf("quotes = %d, want 1 ledger excursion", desk.quotes)
	}
}

func TestModelMaxPathLength(t *testing.T) {
	model := NewModel(context.Background(), newFakeDesk(), nil, nil, nil)
	const symbol = "DOT/USD"
	cfg := LoadConfig()

	for i := 0; i < cfg.MaxPathLength+4; i++ {
		model.Evaluate(symbol, regionToken(uint8(i%3), 0.5))
	}

	untyped, ok := model.paths.Load(symbol)
	if !ok {
		t.Fatal("path was not stored")
	}

	if got := len(untyped.(tokenizer.Tokens)); got != cfg.MaxPathLength {
		t.Fatalf("path length = %d, want %d", got, cfg.MaxPathLength)
	}
}

func TestModelDemotion(t *testing.T) {
	desk := newFakeDesk()
	model := NewModel(context.Background(), desk, nil, nil, nil)
	const symbol = "LTC/USD"
	const entry = "R00/R01/R02"
	cfg := LoadConfig()

	model.tree, _, _ = model.tree.Insert([]byte(entry), newDataValue("enter", symbol, entry))
	model.tree, _, _ = model.tree.Insert(
		[]byte(entry+"/R03"), newDataValue("exit", symbol, entry+"/R03"),
	)

	for i := 0; i < cfg.DemotionLosses; i++ {
		desk.queue(scriptedClose{ret: -0.03, ok: true})

		if i > 0 {
			model.Evaluate(symbol, regionToken(6, 0.2))
		}

		model.Evaluate(symbol, regionToken(0, 0.9))
		model.Evaluate(symbol, regionToken(1, 0.9))
		model.Evaluate(symbol, regionToken(2, 0.9))
		model.Evaluate(symbol, regionToken(3, 0.9))
	}

	if _, found := model.tree.Get([]byte(entry)); found {
		t.Fatal("path stayed in the current policy after consecutive venue losses")
	}
}

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

func TestExploredPathIsPublishedAsATrieKey(t *testing.T) {
	capture := &hubCapture{}
	model := NewModel(context.Background(), newFakeDesk(), nil, capture, nil)
	const symbol = "BTC/USD"

	sampleOneExcursion(model, symbol, 0)

	var keys []any
	for _, payload := range capture.payloads {
		value := data.New()
		if _, err := value.ReadFrom(bytes.NewReader(payload)); err != nil {
			t.Fatalf("decode hub payload: %v", err)
		}
		if value.Origin != "cognition" || value.Role != "keys" {
			continue
		}
		var body map[string]any
		value.Lookup(&body, "cognition")
		listed, _ := body["keys"].([]any)
		if len(listed) > 0 {
			keys = listed
		}
	}

	want := "policy/explored/enter/R00/R01/R02.bin"
	found := false
	for _, key := range keys {
		if key == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("published keys = %v, want %s", keys, want)
	}
}

func TestPublishGrammarFrame(t *testing.T) {
	capture := &hubCapture{}
	model := NewModel(context.Background(), newFakeDesk(), nil, capture, nil)

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

	if frame["symbol"] != "BTC/USD" {
		t.Fatalf("symbol = %v, want BTC/USD", frame["symbol"])
	}

	if _, ok := frame["policy"].(map[string]any); !ok {
		raw, _ := json.Marshal(frame)
		t.Fatalf("policy missing: %s", raw)
	}
}
