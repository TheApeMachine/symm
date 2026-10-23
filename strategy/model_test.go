package strategy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	nmstore "github.com/theapemachine/symm/nomagique/store"
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

func (d *fakeDesk) Enter(symbol, path string, virtual bool, tick uint64) error {
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
		EntryTick:  tick,
	}

	return nil
}

func (d *fakeDesk) Exit(symbol, path string, _ bool, tick uint64, _ ...string) error {
	return d.ExitOccupied(symbol, path, tick)
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

	return position.Return.Sign() > 0, true
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

func (d *fakeDesk) Realize(symbol string, qty, entry, fee *decimal.Decimal) (float64, bool) {
	return d.SaleReturn(symbol, qty, entry, fee)
}

func (d *fakeDesk) SaleReturn(string, *decimal.Decimal, *decimal.Decimal, *decimal.Decimal) (float64, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.closes) == 0 {
		return 0, true
	}

	next := d.closes[0]
	d.closes = d.closes[1:]

	return next.ret, next.ok
}

func (d *fakeDesk) Friction(string) (float64, bool) {
	return 0.01, true
}

func (d *fakeDesk) OpenMark(string) (float64, bool) {
	return 0, true
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

func (d *fakeDesk) ExitOccupied(symbol, path string, tick uint64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	position, ok := d.open[symbol]
	if !ok {
		return fmt.Errorf("position not found for %s", symbol)
	}

	now := time.Now().UTC()
	position.ClosedAt = &now
	position.ExitPath = path
	position.ExitTick = tick
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

func newTestModel(desk Desk, hub io.Writer) *Model {
	model := NewModel(context.Background(), desk, nil, hub, nil)
	model.noteTape(1)
	return model
}

func regionToken(key uint8, brightness float64) tokenizer.Token {
	return tokenizer.NewTokenFromRegion(key, brightness)
}

// sampleOneExcursion opens the precursor, then settles it from the book.
// The default settlement is a fee-net return one hurdle above the open
// print, which is a cleared-friction profit. Pass a return to settle at
// that print instead.
func sampleOneExcursion(model *Model, symbol string, prior int) {
	sampleExcursionAt(model, symbol, prior, 0.01, true)
}

func sampleExcursionAt(model *Model, symbol string, prior int, ret float64, ok bool) {
	if prior > 0 {
		// The token after a close ends the cool-down. It is not a step.
		model.Evaluate(symbol, regionToken(6, 0.2))
	}

	model.Evaluate(symbol, regionToken(0, 0.9))

	if desk, isFake := model.desk.(*fakeDesk); isFake {
		desk.queue(scriptedClose{ret: ret, ok: ok})
	}

	model.reviewExcursions()
}

func TestLedgerDoesNotPromoteOnTwoWins(t *testing.T) {
	desk := newFakeDesk()
	model := newTestModel(desk, nil)
	const symbol = "BTC/USD"

	sampleOneExcursion(model, symbol, 0)
	sampleOneExcursion(model, symbol, 1)

	if _, found := model.tree.Get([]byte("R00")); found {
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
	model := newTestModel(desk, nil)
	const symbol = "BTC/USD"

	cfg := LoadConfig()
	for i := 0; i < cfg.FreezeAfter+cfg.ConfirmAfter; i++ {
		sampleOneExcursion(model, symbol, i)
	}

	if _, found := model.tree.Get([]byte("R00")); !found {
		t.Fatal("confirmed entry path missing from the current policy")
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
	model := newTestModel(desk, nil)
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
	model := newTestModel(desk, nil)
	const symbol = "SOL/USD"
	cfg := LoadConfig()

	for i := 0; i < cfg.FreezeAfter; i++ {
		sampleExcursionAt(model, symbol, i, -0.02, true)
	}

	quotes := desk.quotes
	sampleExcursionAt(model, symbol, cfg.FreezeAfter, -0.02, true)

	if desk.quotes != quotes {
		t.Fatalf("quotes went from %d to %d after the path was retired", quotes, desk.quotes)
	}
}

func TestBookMissDoesNotScore(t *testing.T) {
	desk := newFakeDesk()
	model := newTestModel(desk, nil)
	const symbol = "ADA/USD"

	sampleExcursionAt(model, symbol, 0, 0, false)

	if _, found := model.explored.Get([]byte(exploredKey("enter", "R00"))); found {
		t.Fatal("a close the book could not take was recorded")
	}

	desk.queue(scriptedClose{ret: 0.01, ok: true})
	model.reviewExcursions()

	if _, found := model.explored.Get([]byte(exploredKey("enter", "R00"))); !found {
		t.Fatal("retry against a book that can take the size left no score")
	}
}

func TestLongestMatchExitOverridesShorterEnter(t *testing.T) {
	desk := newFakeDesk()
	model := newTestModel(desk, nil)
	const symbol = "DOGE/USD"

	model.tree, _, _ = model.tree.Insert(
		[]byte("R00"), newDataValue("enter", symbol, "R00"),
	)
	model.tree, _, _ = model.tree.Insert(
		[]byte("R03"), newDataValue("exit", symbol, "R03"),
	)

	entered := model.Evaluate(symbol, regionToken(0, 0.9))
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

func TestMatchedExitClearsThePathWhenFlat(t *testing.T) {
	desk := newFakeDesk()
	model := newTestModel(desk, nil)
	const symbol = "ATOM/USD"

	model.tree, _, _ = model.tree.Insert(
		[]byte("R03"), newDataValue("exit", symbol, "R03"),
	)

	model.Evaluate(symbol, regionToken(3, 0.9))
	model.Evaluate(symbol, regionToken(0, 0.9))
	model.Evaluate(symbol, regionToken(1, 0.9))
	model.Evaluate(symbol, regionToken(2, 0.9))

	if !desk.Has(symbol, true) {
		t.Fatal("matched exit left its prefix in place and the next enter did not open")
	}
}

func TestModelChordToken(t *testing.T) {
	desk := newFakeDesk()
	model := newTestModel(desk, nil)
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

func TestBookReturnInsideFrictionStaysOpen(t *testing.T) {
	desk := newFakeDesk()
	model := newTestModel(desk, nil)
	const symbol = "XRP/USD"

	model.Evaluate(symbol, regionToken(0, 0.9))
	model.Evaluate(symbol, regionToken(1, 0.9))
	model.Evaluate(symbol, regionToken(2, 0.9))

	if !desk.Has(symbol, true) {
		t.Fatal("enter path did not open a virtual lot")
	}

	desk.queue(scriptedClose{ret: 0.004, ok: true})
	model.reviewExcursions()

	if !desk.Has(symbol, true) {
		t.Fatal("a return inside the friction hurdle closed the virtual")
	}

	desk.queue(scriptedClose{ret: -0.004, ok: true})
	model.reviewExcursions()

	if !desk.Has(symbol, true) {
		t.Fatal("a dip smaller than the friction hurdle closed the virtual")
	}
}

func TestClearedFrictionClosesVirtual(t *testing.T) {
	desk := newFakeDesk()
	model := newTestModel(desk, nil)
	const symbol = "XRP/USD"

	model.Evaluate(symbol, regionToken(0, 0.9))
	model.Evaluate(symbol, regionToken(1, 0.9))
	model.Evaluate(symbol, regionToken(2, 0.9))

	desk.queue(scriptedClose{ret: 0.01, ok: true})
	model.reviewExcursions()

	if desk.Has(symbol, true) {
		t.Fatal("clearing the friction hurdle left the virtual open")
	}

	if _, found := model.explored.Get([]byte(exploredKey("enter", "R00"))); !found {
		t.Fatal("cleared friction did not score the enter path")
	}
}

func TestFrictionDipClosesVirtual(t *testing.T) {
	desk := newFakeDesk()
	model := newTestModel(desk, nil)
	const symbol = "AVAX/USD"

	model.Evaluate(symbol, regionToken(0, 0.9))
	model.Evaluate(symbol, regionToken(1, 0.9))
	model.Evaluate(symbol, regionToken(2, 0.9))

	desk.queue(scriptedClose{ret: -0.01, ok: true})
	model.reviewExcursions()

	if desk.Has(symbol, true) {
		t.Fatal("a one-hurdle dip left the virtual open")
	}
}

func TestCloseWritesPrecursorAndHoldingAsSeparatePaths(t *testing.T) {
	desk := newFakeDesk()
	model := newTestModel(desk, nil)
	const symbol = "XNY/USD"

	model.Evaluate(symbol, regionToken(0, 0.9))
	model.Evaluate(symbol, regionToken(1, 0.9))
	model.Evaluate(symbol, regionToken(2, 0.9))
	model.Evaluate(symbol, regionToken(3, 0.9))
	model.Evaluate(symbol, regionToken(4, 0.9))

	desk.queue(scriptedClose{ret: 0.01, ok: true})
	model.reviewExcursions()

	const precursor = "R00"
	const holding = "R00/R03/R04"

	if _, found := model.explored.Get([]byte(exploredKey("enter", precursor))); !found {
		t.Fatal("precursor was not stored as its own enter path")
	}

	if _, found := model.explored.Get([]byte(exploredKey("exit", holding))); !found {
		t.Fatal("holding fragment was not stored as its own exit path")
	}

	if _, found := model.explored.Get([]byte(exploredKey("exit", precursor))); found {
		t.Fatal("exit terminal was written on the precursor path")
	}

	if _, found := model.explored.Get([]byte(exploredKey("enter", holding))); found {
		t.Fatal("enter terminal was written on the holding path")
	}
}

func TestAppendPathSkipsConsecutiveDuplicate(t *testing.T) {
	model := newTestModel(newFakeDesk(), nil)
	const symbol = "ACU/USD"

	chord := tokenizer.ChordToken([]tokenizer.Token{
		tokenizer.NewTokenFromRegion(0, 0.8),
		tokenizer.NewTokenFromRegion(9, 0.8),
	})

	model.mu.Lock()
	first := model.appendPath(symbol, chord)
	second := model.appendPath(symbol, chord)
	other := model.appendPath(
		symbol,
		tokenizer.ChordToken([]tokenizer.Token{
			tokenizer.NewTokenFromRegion(1, 0.8),
		}),
	)
	model.mu.Unlock()

	if len(first) != 1 {
		t.Fatalf("first append len = %d, want 1", len(first))
	}
	if len(second) != 1 {
		t.Fatalf("duplicate append len = %d, want 1", len(second))
	}
	if len(other) != 2 {
		t.Fatalf("after transition len = %d, want 2", len(other))
	}
}

func TestModelMaxPathLength(t *testing.T) {
	model := newTestModel(newFakeDesk(), nil)
	const symbol = "DOT/USD"
	cfg := LoadConfig()

	model.Evaluate(symbol, regionToken(0, 0.9))

	for i := 0; i < cfg.MaxPathLength+4; i++ {
		key := uint8(3)
		if i%2 == 1 {
			key = 4
		}
		model.Evaluate(symbol, regionToken(key, 0.5))
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
	model := newTestModel(desk, nil)
	const symbol = "LTC/USD"
	const entry = "R00"
	cfg := LoadConfig()

	model.tree, _, _ = model.tree.Insert([]byte(entry), newDataValue("enter", symbol, entry))
	model.tree, _, _ = model.tree.Insert(
		[]byte("R03"), newDataValue("exit", symbol, "R03"),
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
	model := newTestModel(newFakeDesk(), capture)
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

	want := "policy/explored/enter/R00.bin"
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
	model := newTestModel(newFakeDesk(), capture)

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

func TestNoteTickSurpriseIsTheEnergyError(t *testing.T) {
	model := newTestModel(newFakeDesk(), nil)
	grid := nmstore.NewGrid(context.Background())
	grid.Priors["book"] = map[string]*nmstore.Prior{
		"BTC/USD": {Samples: 2, Prediction: 1, Energy: 0.25},
	}

	for range 3 {
		model.NoteTick("BTC/USD", nil, grid)
	}

	untyped, ok := model.tickReadings.Load("BTC/USD")
	if !ok {
		t.Fatal("no tick reading")
	}

	reading := untyped.(tickReading)
	if !reading.hasSurprise || reading.surprise != 0.75 {
		t.Fatalf("surprise = %v (%v), want |0.25 - 1|", reading.surprise, reading.hasSurprise)
	}

	if reading.surprise < 0 {
		t.Fatal("surprise went negative")
	}

	if !reading.hasZ || reading.z != 0 {
		t.Fatalf("z = %v (%v), want 0 once the same error has a spread of zero", reading.z, reading.hasZ)
	}
}

func TestRegimeDriftUsesTheChordThatClosed(t *testing.T) {
	model := newTestModel(newFakeDesk(), nil)
	model.TrackSequitur("BTC/USD", tokenizer.NewToken([]string{"R01", "R02"}, 1, 1, 0))

	drift, compared := model.regimeDrift("BTC/USD", tokenizer.NewToken([]string{"R00", "R01", "R02"}, 1, 1, 0))
	if drift || !compared {
		t.Fatalf("drift=%v compared=%v, want the regime inside the chord", drift, compared)
	}

	drift, compared = model.regimeDrift("BTC/USD", tokenizer.NewToken([]string{"R09"}, 1, 1, 0))
	if !drift || !compared {
		t.Fatalf("drift=%v compared=%v, want a regime outside the chord", drift, compared)
	}

	model.TrackSequitur("BTC/USD", tokenizer.Token{})
	if _, compared = model.regimeDrift("BTC/USD", tokenizer.NewToken([]string{"R01"}, 1, 1, 0)); compared {
		t.Fatal("a tick that did not close a regime was compared")
	}
}

func TestStampStockIsNotScaledBySymbols(t *testing.T) {
	model := newTestModel(newFakeDesk(), nil)
	model.BindMetrics(newMetricsAccumulator())
	model.explored, _, _ = model.explored.Insert(
		[]byte(exploredKey("enter", "R00/R01")),
		data.NewValue("policy", "explored", "enter"),
	)

	model.recordFrameMetrics("BTC/USD", false, true, true, false, false, false)
	model.recordFrameMetrics("ETH/USD", false, true, true, false, false, false)
	model.StampStock()

	snap, _ := model.metrics.snapshot.Load().(TickMetrics)
	if snap.ExploredEnter != 1 {
		t.Fatalf("explored = %d, want 1 path, not one per symbol", snap.ExploredEnter)
	}

	if snap.GatePass != 0 || snap.FrictionPass != 2 {
		t.Fatalf("gate %d friction %d, want the friction check counted on its own", snap.GatePass, snap.FrictionPass)
	}

	if snap.PathCompared != 0 {
		t.Fatalf("path comparisons = %d, want 0 when no regime closed", snap.PathCompared)
	}
}

func TestPrecursorAnchorUsesTheTapeTick(t *testing.T) {
	desk := newFakeDesk()
	model := newTestModel(desk, nil)
	const symbol = "BTC/USD"

	model.noteTape(8550)
	model.Evaluate(symbol, regionToken(0, 0.9))
	model.noteTape(8555)
	model.Evaluate(symbol, regionToken(1, 0.9))
	model.noteTape(8560)
	model.Evaluate(symbol, regionToken(2, 0.9))

	virtual := desk.Virtual(symbol)
	if virtual == nil {
		t.Fatal("the enter regime did not open a virtual lot")
	}

	if virtual.MarkATick != 8550 {
		t.Fatalf("mark A = %d, want the regime's tape tick 8550", virtual.MarkATick)
	}

	if virtual.EntryTick != 8550 {
		t.Fatalf("entry tick = %d, want the regime's tape tick 8550", virtual.EntryTick)
	}
}

func TestOppositionAndOraclesDoNotOpenOrWipe(t *testing.T) {
	desk := newFakeDesk()
	model := newTestModel(desk, nil)
	const symbol = "BTC/USD"

	model.Evaluate(symbol, regionToken(2, 0.9))
	model.Evaluate(symbol, regionToken(7, 0.9))
	model.Evaluate(symbol, tokenizer.NewToken([]string{"R00", "R02"}, 0.9, 0.9, 0))

	if desk.Has(symbol, true) {
		t.Fatal("opposition or an oracle opened a sample")
	}

	model.Evaluate(symbol, regionToken(0, 0.1))

	if desk.Has(symbol, true) {
		t.Fatal("brightness under the starting threshold opened a sample")
	}

	model.Evaluate(symbol, regionToken(0, 0.9))

	if !desk.Has(symbol, true) {
		t.Fatal("brightness above the starting threshold did not open a sample")
	}
}

func TestRestoredVirtualIsNotTheLearningSlot(t *testing.T) {
	desk := newFakeDesk()
	model := newTestModel(desk, nil)
	const symbol = "BTC/USD"

	desk.open[symbol] = &broker.Position{
		Symbol:     symbol,
		Virtual:    true,
		EntryPath:  "restored",
		Qty:        decimal.NewFromInt64(1),
		EntryPrice: decimal.NewFromInt64(100),
		EntryFee:   decimal.NewFromInt64(1),
	}

	model.Evaluate(symbol, regionToken(0, 0.9))

	if desk.entries != 0 {
		t.Fatal("the sample took the restored lot's slot")
	}

	if desk.quotes != 1 {
		t.Fatalf("quotes = %d, want the sample priced beside the restored lot", desk.quotes)
	}

	if _, ok := model.excursions[symbol]; !ok {
		t.Fatal("the restored lot blocked the learning sample")
	}

	desk.queue(scriptedClose{ret: 0.01, ok: true})
	model.reviewExcursions()

	if _, found := model.explored.Get([]byte(exploredKey("enter", "R00"))); !found {
		t.Fatal("the sample beside the restored lot was not scored")
	}

	if desk.open[symbol] == nil || desk.open[symbol].EntryPath != "restored" {
		t.Fatal("scoring the sample closed the restored lot")
	}
}

func scoreFor(t *testing.T, model *Model, path string) pathScore {
	t.Helper()
	val, found := model.explored.Get([]byte(exploredKey("enter", path)))
	if !found || val == nil {
		t.Fatalf("no score for %s", path)
	}
	var score pathScore
	val.Lookup(&score, "score")
	return score
}

func TestThresholdRisesAfterALossAndFallsAfterAWin(t *testing.T) {
	desk := newFakeDesk()
	model := newTestModel(desk, nil)
	const symbol = "BTC/USD"

	model.Evaluate(symbol, regionToken(0, 0.9))
	desk.queue(scriptedClose{ret: -0.02, ok: true})
	model.reviewExcursions()

	score := scoreFor(t, model, "R00")
	if score.Threshold < 0.54 || score.Threshold > 0.56 {
		t.Fatalf("threshold = %v, want about 0.55 after a loss", score.Threshold)
	}

	model.Evaluate(symbol, regionToken(6, 0.2))
	model.Evaluate(symbol, regionToken(0, 0.5))
	if desk.quotes != 1 {
		t.Fatalf("quotes = %d, want the dimmer print refused", desk.quotes)
	}

	model.Evaluate(symbol, regionToken(0, 0.6))
	desk.queue(scriptedClose{ret: 0.02, ok: true})
	model.reviewExcursions()

	score = scoreFor(t, model, "R00")
	if score.Threshold < 0.49 || score.Threshold > 0.51 {
		t.Fatalf("threshold = %v, want about 0.50 after the win", score.Threshold)
	}
}

func TestThresholdAtTheCeilingPrunesThePath(t *testing.T) {
	desk := newFakeDesk()
	model := newTestModel(desk, nil)
	const symbol = "BTC/USD"
	cfg := model.cfg
	// Keep the mean rule from retiring the path before the ceiling does.
	cfg.FreezeAfter = 100

	steps := int((cfg.ThresholdCeil-cfg.ThresholdStart)/cfg.ThresholdStep + 0.5)
	for i := 0; i < steps; i++ {
		if i > 0 {
			model.Evaluate(symbol, regionToken(6, 0.2))
		}
		model.Evaluate(symbol, regionToken(0, 1))
		desk.queue(scriptedClose{ret: -0.02, ok: true})
		model.reviewExcursions()
	}

	score := scoreFor(t, model, "R00")
	if !score.Retired || score.Threshold < cfg.ThresholdCeil-1e-9 {
		t.Fatalf("threshold = %v retired = %v, want the ceiling to prune the path", score.Threshold, score.Retired)
	}

	quotes := desk.quotes
	model.Evaluate(symbol, regionToken(6, 0.2))
	model.Evaluate(symbol, regionToken(0, 1))
	if desk.quotes != quotes {
		t.Fatal("a pruned path was entered again")
	}
}
