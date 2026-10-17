package strategy

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
)

const (
	virtualProfit = 0.50
	virtualLoss   = -0.25
	thresholdEase = 0.8
	// virtualStake is the paper account's reset balance, in quote currency.
	// Each virtual entry is sized from this stake, scaled by the risk
	// fraction, so the stake is reset rather than left at an unbounded budget.
	virtualStake = 200
)

/*
virtualLot is one global experiment on a finished rule. The book of Symbol
marks it. The path's action, once settled, is the same for every symbol.
*/
type virtualLot struct {
	Path       string
	Symbol     string
	Quantity   *decimal.Decimal
	Entry      *decimal.Decimal
	Fee        *decimal.Decimal
	Brightness []float64
	Tokens     []string
	Opened     time.Time
}

type virtualBook struct {
	mu   sync.Mutex
	open map[string]virtualLot
}

type stepBars struct {
	mu   sync.Mutex
	bars map[string][]float64
	seen map[string][]float64
}

func virtualOutcome(ret float64) (action string, closed bool) {
	if ret >= virtualProfit {
		return "enter", true
	}

	if ret <= virtualLoss {
		return "", true
	}

	return "", false
}

func (training *Training) beginVirtual(lot virtualLot) bool {
	if training == nil || lot.Path == "" || lot.Symbol == "" {
		return false
	}

	training.virtuals.mu.Lock()
	defer training.virtuals.mu.Unlock()

	if training.virtuals.open == nil {
		training.virtuals.open = map[string]virtualLot{}
	}

	if _, open := training.virtuals.open[lot.Path]; open {
		return false
	}

	if lot.Opened.IsZero() {
		lot.Opened = time.Now().UTC()
	}

	training.virtuals.open[lot.Path] = lot
	training.persistVirtual(lot)
	return true
}

func (training *Training) virtualOpen(path string) bool {
	if training == nil {
		return false
	}

	training.virtuals.mu.Lock()
	defer training.virtuals.mu.Unlock()

	_, open := training.virtuals.open[path]
	return open
}

func (training *Training) closeVirtual(path string) {
	if training == nil {
		return
	}

	training.virtuals.mu.Lock()
	defer training.virtuals.mu.Unlock()

	delete(training.virtuals.open, path)
}

func (training *Training) pathAction(path string) string {
	if training == nil || path == "" {
		return ""
	}

	current := training.policy.Load()

	if current == nil || current.actions == nil {
		return ""
	}

	return current.actions[path]
}

func (training *Training) setPathAction(path, action string) {
	if training == nil || path == "" || (action != "enter" && action != "exit") {
		return
	}

	current := training.policy.Load()
	next := &policy{actions: map[string]string{}}

	if current != nil {
		for key, value := range current.actions {
			next.actions[key] = value
		}

		next.keys = append([]string{}, current.keys...)
	}

	for key := range next.actions {
		if key == path || strings.HasSuffix(key, "/"+path) {
			next.actions[key] = action
		}
	}

	next.actions[path] = action
	next.keys = policyKeys(next.keys, path, action)
	training.policy.Store(next)
	training.persistAction(path, action)
}

func policyKeys(keys []string, path, action string) []string {
	file := path + "/" + action + ".bin"
	kept := make([]string, 0, len(keys)+1)
	seen := map[string]struct{}{file: {}}

	for _, key := range keys {
		if key == path+"/enter.bin" || key == path+"/exit.bin" {
			continue
		}

		if _, ok := seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}
		kept = append(kept, key)
	}

	kept = append(kept, file)
	return kept
}

func (training *Training) persistAction(path, action string) {
	if training == nil || training.client == nil || path == "" {
		return
	}

	for _, other := range []string{"enter", "exit"} {
		if other == action {
			continue
		}

		if err := training.client.DeletePrefix(policyObjectKey(path, other)); err != nil {
			training.Error(err)
		}
	}

	value := data.NewValue("policy", action, "all")
	data.AppendVector(value, []float64{0, 0, 1, 0, 1}, "gain", "hold_seconds", "visits", "mean_reward", "confidence", "path:"+path)

	if err := training.client.Put(policyObjectKey(path, action), value.Bytes()); err != nil {
		training.Error(errnie.Err(errnie.IO, "[training] store "+action+" "+path, err))
	}
}

/*
clearPath removes the path's action. The next time that sentence completes,
nothing matches.
*/
func (training *Training) clearPath(path string) {
	if training == nil || path == "" {
		return
	}

	current := training.policy.Load()
	next := &policy{actions: map[string]string{}}

	if current != nil {
		for key, value := range current.actions {
			if key == path || strings.HasSuffix(key, "/"+path) {
				continue
			}

			next.actions[key] = value
		}

		for _, key := range current.keys {
			if key == path+"/enter.bin" || key == path+"/exit.bin" || key == path+"/noop.bin" {
				continue
			}

			next.keys = append(next.keys, key)
		}
	}

	training.policy.Store(next)

	if training.client == nil {
		return
	}

	for _, action := range []string{"enter", "exit", "noop"} {
		if err := training.client.DeletePrefix(policyObjectKey(path, action)); err != nil {
			training.Error(err)
		}
	}
}

/*
applyVirtualReturn settles one open virtual. Fifty percent fee-net profit
makes the path an entry. A loss of twenty-five percent removes the action,
so the path no longer matches. A path that was already enter or exit and
clears fifty percent also lowers the brightness each of its steps requires.
*/
func (training *Training) applyVirtualReturn(path string, ret float64) {
	if training == nil || !training.virtualOpen(path) {
		return
	}

	action, closed := virtualOutcome(ret)

	if !closed {
		return
	}

	lot := training.takeVirtual(path)
	was := training.pathAction(path)

	if action == "" {
		training.clearPath(path)
		training.publishVirtualLife(lot, true, ret, true)
		return
	}

	training.setPathAction(path, action)
	training.publishVirtualLife(lot, true, ret, true)

	if action != "enter" {
		return
	}

	if was == "enter" || was == "exit" {
		training.strengthenPath(path, lot.Brightness)
		return
	}

	training.seedThreshold(path, lot.Brightness)
}

func (training *Training) takeVirtual(path string) virtualLot {
	training.virtuals.mu.Lock()
	defer training.virtuals.mu.Unlock()

	lot := training.virtuals.open[path]
	delete(training.virtuals.open, path)
	training.dropVirtual(path)
	return lot
}

func (training *Training) seedThreshold(path string, brightness []float64) {
	if training == nil || path == "" || len(brightness) == 0 {
		return
	}

	training.bars.mu.Lock()
	defer training.bars.mu.Unlock()

	if training.bars.bars == nil {
		training.bars.bars = map[string][]float64{}
	}

	training.bars.bars[path] = append([]float64{}, brightness...)
	training.persistBars(path, training.bars.bars[path])
}

func (training *Training) strengthenPath(path string, observed []float64) {
	if training == nil || path == "" {
		return
	}

	training.bars.mu.Lock()
	defer training.bars.mu.Unlock()

	if training.bars.bars == nil {
		training.bars.bars = map[string][]float64{}
	}

	bars := training.bars.bars[path]

	if len(bars) == 0 {
		if len(observed) == 0 && training.bars.seen != nil {
			observed = training.bars.seen[path]
		}

		if len(observed) == 0 {
			return
		}

		bars = make([]float64, len(observed))

		for i, brightness := range observed {
			bars[i] = brightness * thresholdEase
		}

		training.bars.bars[path] = bars
		training.persistBars(path, bars)
		return
	}

	for i := range bars {
		bars[i] *= thresholdEase
	}

	training.bars.bars[path] = bars
	training.persistBars(path, bars)
}

/*
noteClosed strengthens a path when a real enter or exit closes in profit.
A loss leaves the path as it is. Virtual trades are what open and clear paths.
*/
func (training *Training) noteClosed(position *broker.Position) error {
	if training == nil || position == nil {
		return nil
	}

	path, pnl, ok := closedPath(position)

	if !ok || pnl <= 0 {
		return nil
	}

	action := training.pathAction(path)

	if action != "enter" && action != "exit" {
		return nil
	}

	training.strengthenPath(path, nil)
	return nil
}

func closedPath(position *broker.Position) (string, float64, bool) {
	if position == nil || !position.Closed() {
		return "", 0, false
	}

	life := position.Lifecycle()

	if life.Outcome == nil || life.Symbol == "" {
		return "", 0, false
	}

	path := ""

	for _, event := range life.Events {
		if event.Kind != "entry_match" {
			continue
		}

		found, isString := event.Fields["path"].(string)

		if isString && found != "" {
			path = found
		}
	}

	if path == "" {
		return "", 0, false
	}

	return path, life.Outcome.VenueRealized, true
}

func (training *Training) noteSteps(path string, brightness []float64) {
	if training == nil || path == "" || len(brightness) == 0 {
		return
	}

	training.bars.mu.Lock()
	defer training.bars.mu.Unlock()

	if training.bars.seen == nil {
		training.bars.seen = map[string][]float64{}
	}

	training.bars.seen[path] = append([]float64{}, brightness...)
}

/*
admitToken reports whether token may be appended after prefix. A token that
extends a path must clear that step's brightness. Any other token must clear
the tick's midpoint.
*/
func (training *Training) admitToken(prefix []string, token string, brightness, mid float64) bool {
	if training == nil || token == "" || brightness <= 0 {
		return false
	}

	bar, extends := training.stepBar(prefix, token)

	if !extends {
		return brightness >= mid
	}

	return brightness >= bar
}

func (training *Training) stepBar(prefix []string, token string) (float64, bool) {
	training.bars.mu.Lock()
	defer training.bars.mu.Unlock()

	if len(training.bars.bars) == 0 {
		return 0, false
	}

	step := len(prefix)
	soFar := strings.Join(prefix, "/")
	bar := 0.0
	found := false

	for path, steps := range training.bars.bars {
		parts := strings.Split(path, "/")

		if step >= len(parts) || parts[step] != token || step >= len(steps) {
			continue
		}

		if step > 0 && strings.Join(parts[:step], "/") != soFar {
			continue
		}

		if !found || steps[step] < bar {
			bar = steps[step]
			found = true
		}
	}

	return bar, found
}

func (training *Training) openVirtual(symbol string, phrase []string, brightness []float64) {
	if training == nil || training.price == nil || symbol == "" || len(phrase) < 2 {
		return
	}

	path := strings.Join(phrase, "/")

	if training.virtualOpen(path) {
		return
	}

	if system.Cfg == nil || system.Cfg.Risk == nil || system.Cfg.Risk.MaxFraction == nil || system.Cfg.Risk.MaxFraction.Sign() <= 0 {
		training.Error(errnie.Err(
			errnie.Validation,
			"[training] risk max fraction required to size a virtual entry",
			nil,
		))

		return
	}

	stake := decimal.NewFromFloat64(virtualStake)
	allocated := stake.SetScale(4).Mul(system.Cfg.Risk.MaxFraction)
	cost, err := training.price.AllocateEntry(symbol, allocated)

	if err != nil || cost == nil || cost.Quantity == nil || cost.EntryPrice == nil || cost.Quantity.Sign() <= 0 {
		return
	}

	training.beginVirtual(virtualLot{
		Path:       path,
		Symbol:     symbol,
		Quantity:   cost.Quantity,
		Entry:      cost.EntryPrice,
		Fee:        cost.EntryFee,
		Brightness: append([]float64{}, brightness...),
		Tokens:     append([]string{}, phrase...),
	})
	training.noteSteps(path, brightness)
}

func (training *Training) markSymbolVirtuals(symbol string) {
	if training == nil || symbol == "" || training.price == nil {
		return
	}

	training.virtuals.mu.Lock()
	lots := make([]virtualLot, 0)

	for _, lot := range training.virtuals.open {
		if lot.Symbol == symbol {
			lots = append(lots, lot)
		}
	}

	training.virtuals.mu.Unlock()

	for _, lot := range lots {
		_, _, ret, ok := training.price.Exit(lot.Symbol, lot.Quantity, lot.Entry, lot.Fee)

		if !ok {
			continue
		}

		training.applyVirtualReturn(lot.Path, ret)
	}
}

func (training *Training) persistBars(path string, bars []float64) {
	if training == nil || training.client == nil || path == "" {
		return
	}

	raw, err := sonic.Marshal(bars)

	if err != nil {
		training.Error(err)
		return
	}

	value := data.NewValue("policy", "threshold", "all")
	value.FormatPrefix()
	value.AppendJSON(raw, value.Prefix+"/"+data.LayerLedger)

	if err := training.client.Put(thresholdKey(path), value.Bytes()); err != nil {
		training.Error(errnie.Err(errnie.IO, "[training] store threshold "+path, err))
	}
}

func thresholdKey(path string) string {
	return "policy/threshold/" + path + ".bin"
}

func virtualKey(path string) string {
	return "policy/virtual/" + path + ".bin"
}

type storedVirtual struct {
	Path       string    `json:"path"`
	Symbol     string    `json:"symbol"`
	Quantity   string    `json:"quantity"`
	Entry      string    `json:"entry"`
	Fee        string    `json:"fee"`
	Brightness []float64 `json:"brightness"`
	Tokens     []string  `json:"tokens,omitempty"`
	Opened     string    `json:"opened,omitempty"`
}

func (training *Training) persistVirtual(lot virtualLot) {
	if training == nil || training.client == nil || lot.Path == "" {
		return
	}

	record := storedVirtual{
		Path:       lot.Path,
		Symbol:     lot.Symbol,
		Brightness: lot.Brightness,
		Tokens:     append([]string{}, lot.Tokens...),
	}

	if !lot.Opened.IsZero() {
		record.Opened = lot.Opened.UTC().Format(time.RFC3339Nano)
	}

	if lot.Quantity != nil {
		record.Quantity = lot.Quantity.String()
	}

	if lot.Entry != nil {
		record.Entry = lot.Entry.String()
	}

	if lot.Fee != nil {
		record.Fee = lot.Fee.String()
	}

	raw, err := sonic.Marshal(record)

	if err != nil {
		training.Error(err)
		return
	}

	value := data.NewValue("policy", "virtual", lot.Symbol)
	value.FormatPrefix()
	value.AppendJSON(raw, value.Prefix+"/"+data.LayerLedger)

	if err := training.client.Put(virtualKey(lot.Path), value.Bytes()); err != nil {
		training.Error(errnie.Err(errnie.IO, "[training] store virtual "+lot.Path, err))
	}
}

func (training *Training) dropVirtual(path string) {
	if training == nil || training.client == nil || path == "" {
		return
	}

	if err := training.client.DeletePrefix(virtualKey(path)); err != nil {
		training.Error(err)
	}
}

func (training *Training) loadVirtuals() {
	if training == nil || training.client == nil {
		return
	}

	for _, key := range training.client.ListKeys("policy/virtual/") {
		value, err := store.ReadValue(training.client, key)

		if err != nil {
			training.Error(err)
			continue
		}

		lot, ok := virtualFrom(value)

		if !ok {
			continue
		}

		training.beginVirtual(lot)
	}
}

func virtualFrom(value *data.Value) (virtualLot, bool) {
	if value == nil {
		return virtualLot{}, false
	}

	for i, layer := range value.Layers {
		if i >= len(value.Index) || !strings.HasSuffix(value.Index[i], "/"+data.LayerLedger) {
			continue
		}

		var record storedVirtual

		if err := sonic.Unmarshal(layer, &record); err != nil || record.Path == "" || record.Symbol == "" {
			return virtualLot{}, false
		}

		lot := virtualLot{
			Path:       record.Path,
			Symbol:     record.Symbol,
			Brightness: record.Brightness,
			Tokens:     append([]string{}, record.Tokens...),
		}
		var err error

		if record.Opened != "" {
			lot.Opened, err = time.Parse(time.RFC3339Nano, record.Opened)

			if err != nil {
				return virtualLot{}, false
			}
		}

		if record.Quantity != "" {
			if lot.Quantity, err = decimal.NewFromString(record.Quantity); err != nil {
				return virtualLot{}, false
			}
		}

		if record.Entry != "" {
			if lot.Entry, err = decimal.NewFromString(record.Entry); err != nil {
				return virtualLot{}, false
			}
		}

		if record.Fee != "" {
			if lot.Fee, err = decimal.NewFromString(record.Fee); err != nil {
				return virtualLot{}, false
			}
		}

		return lot, true
	}

	return virtualLot{}, false
}

func (training *Training) loadBars() {
	if training == nil || training.client == nil {
		return
	}

	for _, key := range training.client.ListKeys("policy/threshold/") {
		value, err := store.ReadValue(training.client, key)

		if err != nil {
			training.Error(err)
			continue
		}

		bars, ok := barsFrom(value)

		if !ok {
			continue
		}

		path := strings.TrimSuffix(strings.TrimPrefix(key, "policy/threshold/"), ".bin")
		training.seedThreshold(path, bars)
	}
}

func barsFrom(value *data.Value) ([]float64, bool) {
	if value == nil {
		return nil, false
	}

	for i, layer := range value.Layers {
		if i >= len(value.Index) || !strings.HasSuffix(value.Index[i], "/"+data.LayerLedger) {
			continue
		}

		var bars []float64

		if err := sonic.Unmarshal(layer, &bars); err != nil || len(bars) == 0 {
			return nil, false
		}

		return bars, true
	}

	return nil, false
}

func virtualScope(lot virtualLot) string {
	return lot.Symbol + " · " + lot.Path
}

func (training *Training) copyVirtuals() []virtualLot {
	if training == nil {
		return nil
	}

	training.virtuals.mu.Lock()
	defer training.virtuals.mu.Unlock()

	lots := make([]virtualLot, 0, len(training.virtuals.open))

	for _, lot := range training.virtuals.open {
		lots = append(lots, lot)
	}

	return lots
}

func (training *Training) publishVirtualMarks(tick *data.Value) {
	if training == nil || tick == nil {
		return
	}

	live := map[string]struct{}{}

	for _, lot := range training.copyVirtuals() {
		scope := virtualScope(lot)
		live[scope] = struct{}{}
		training.writeVirtual(tick, lot, false, 0)
	}

	if training.virtualShown == nil {
		training.virtualShown = map[string]struct{}{}
	}

	for scope := range training.virtualShown {
		if _, ok := live[scope]; ok {
			continue
		}

		training.writeVirtualFlat(tick, scope)
		delete(training.virtualShown, scope)
	}

	for scope := range live {
		training.virtualShown[scope] = struct{}{}
	}
}

func (training *Training) writeVirtual(tick *data.Value, lot virtualLot, closed bool, ret float64) {
	if lot.Symbol == "" || lot.Path == "" || lot.Entry == nil || lot.Quantity == nil {
		return
	}

	entry := lot.Entry.Float64()
	qty := lot.Quantity.Float64()
	bid := entry
	value := entry * qty
	have := false

	if !closed && training.price != nil {
		if sale, proceeds, marked, ok := training.price.Exit(lot.Symbol, lot.Quantity, lot.Entry, lot.Fee); ok {
			bid = sale
			value = proceeds
			ret = marked
			have = true
		}
	}

	if closed {
		have = true
	}

	keys := []string{
		"position_open",
		"position_entry",
		"position_bid",
		"position_qty",
		"position_cost",
		"position_value",
		"position_exiting",
		"position_held_s",
		"position_book",
	}
	open := 1.0

	if closed {
		open = 0
	}

	held := 0.0

	if !lot.Opened.IsZero() {
		held = time.Since(lot.Opened).Seconds()
	}

	values := []float64{
		open,
		entry,
		bid,
		qty,
		entry * qty,
		value,
		0,
		held,
		1,
	}

	if have {
		keys = append(keys, "position_pnl_pct")
		values = append(values, ret)
	}

	for i, token := range lot.Tokens {
		region, err := strconv.Atoi(strings.TrimPrefix(token, "R"))

		if err != nil || !strings.HasPrefix(token, "R") {
			continue
		}

		keys = append(keys, fmt.Sprintf("path_token:%d:0", i))
		values = append(values, float64(region))
	}

	if len(lot.Tokens) > 0 {
		keys = append(keys, "token_depth")
		values = append(values, float64(len(lot.Tokens)))
	}

	out := data.NewValue("training", "position", virtualScope(lot))
	out.Epoch, out.Tick, out.Timestamp = tick.Epoch, tick.Tick, tick.Timestamp
	out.FormatPrefix()
	data.AppendVector(out, values, keys...)

	if err := training.writeFrame("", out); err != nil {
		training.Error(err)
	}

	training.publishVirtualLife(lot, closed, ret, have)
}

func (training *Training) writeVirtualFlat(tick *data.Value, scope string) {
	out := data.NewValue("training", "position", scope)
	out.Epoch, out.Tick, out.Timestamp = tick.Epoch, tick.Tick, tick.Timestamp
	out.FormatPrefix()
	data.AppendVector(out, []float64{0, 1}, "position_open", "position_book")

	if err := training.writeFrame("", out); err != nil {
		training.Error(err)
	}
}

func (training *Training) publishVirtualLife(lot virtualLot, closed bool, ret float64, have bool) {
	if training == nil || lot.Path == "" {
		return
	}

	status := "open"

	if closed {
		status = "closed"
	}

	opened := lot.Opened

	if opened.IsZero() {
		opened = time.Now().UTC()
	}

	life := closedLife{
		ID:       lot.Path,
		Symbol:   lot.Symbol,
		Status:   status,
		OpenedAt: opened.Format(time.RFC3339),
		Events: []broker.LifeEvent{{
			At:     opened.Format(time.RFC3339),
			Kind:   "entry_match",
			Detail: "virtual " + lot.Path,
			Fields: map[string]any{"path": lot.Path, "book": "virtual"},
		}},
		Book: "virtual",
	}

	if closed {
		life.ClosedAt = time.Now().UTC().Format(time.RFC3339)
		cost := 0.0

		if lot.Entry != nil && lot.Quantity != nil {
			cost = lot.Entry.Float64() * lot.Quantity.Float64()
		}

		fee := 0.0

		if lot.Fee != nil {
			fee = lot.Fee.Float64()
		}

		realized := 0.0

		if have {
			realized = ret * (cost + fee)
		}

		net := 0.0

		if cost+fee > 0 {
			net = realized / (cost + fee)
		}

		life.Outcome = &closedOutcome{
			Cost:      cost,
			Fees:      fee,
			Realized:  realized,
			NetReturn: net,
			Triggers:  []string{"virtual"},
		}
	}

	raw, err := sonic.Marshal(life)

	if err != nil {
		training.Error(err)
		return
	}

	if err := training.writeFrame("virtual/life/"+lot.Path, snapshotValue(lot.Symbol, raw)); err != nil {
		training.Error(err)
	}
}
