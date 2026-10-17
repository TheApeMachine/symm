package strategy

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
)

/*
publishHeld writes one training frame per lot the desk holds, and a closed
frame for a lot that was held on the previous tick. The open-positions panel
reads these metrics; a missing frame leaves the last reading in place.
*/
func (training *Training) publishHeld(tick *data.Value) {
	if training == nil || training.desk == nil || tick == nil {
		return
	}

	open := map[string]*broker.Position{}

	for _, position := range training.desk.OpenPositions() {
		if position == nil || position.Symbol() == "" || position.Closed() {
			continue
		}

		open[position.Symbol()] = position
	}

	if training.published == nil {
		training.published = map[string]string{}
	}

	for symbol := range training.published {
		if _, still := open[symbol]; still {
			continue
		}

		training.writePosition(tick, symbol, nil)
		training.forgetOpen(symbol)
		delete(training.published, symbol)
	}

	for symbol, position := range open {
		training.writePosition(tick, symbol, position)

		if fingerprint := positionFingerprint(position); training.published[symbol] != fingerprint {
			training.persistOpen(symbol, position)
			training.published[symbol] = fingerprint
		}
	}

	training.publishClosed()
	training.publishVirtualMarks(tick)
}

/*
restoreOpenPositions installs every lot stored under position/open/ so a
restart can exit what the previous process entered.
*/
func (training *Training) restoreOpenPositions() {
	if training == nil || training.client == nil || training.desk == nil {
		return
	}

	for _, key := range training.client.ListKeys(data.OpenPositionPrefix) {
		value, err := store.ReadValue(training.client, key)

		if err != nil {
			training.Error(err)
			continue
		}

		raw := positionLayer(value)

		if len(raw) == 0 {
			continue
		}

		symbol, err := training.desk.RestoreStored(raw)

		if err != nil {
			training.Error(err)
			continue
		}

		if symbol == "" {
			continue
		}

		if training.published == nil {
			training.published = map[string]string{}
		}

		for _, position := range training.desk.OpenPositions() {
			if position != nil && position.Symbol() == symbol {
				training.published[symbol] = positionFingerprint(position)
			}
		}
	}
}

func (training *Training) persistOpen(symbol string, position *broker.Position) {
	if training == nil || training.client == nil || position == nil || symbol == "" {
		return
	}

	raw, err := position.MarshalJSON()

	if err != nil {
		training.Error(errnie.Err(errnie.Internal, "[training] position of "+symbol, err))
		return
	}

	out := data.NewValue(data.PositionOrigin, "open", symbol)
	out.FormatPrefix()
	out.AppendJSON(raw, data.LayerPosition)

	if err := training.client.Put(data.OpenPositionKey(symbol), out.Bytes()); err != nil {
		training.Error(errnie.Err(errnie.IO, "[training] store open position "+symbol, err))
	}
}

func (training *Training) forgetOpen(symbol string) {
	if training == nil || training.client == nil || symbol == "" {
		return
	}

	if err := training.client.DeletePrefix(data.OpenPositionKey(symbol)); err != nil {
		training.Error(errnie.Err(errnie.IO, "[training] remove open position "+symbol, err))
	}
}

/*
dropWalletLots removes the open and closed lots of the wallet that just
reset. A later start would otherwise put those lots back on the empty account.
*/
func (training *Training) dropWalletLots() {
	if training == nil || training.client == nil {
		return
	}

	err := dropListedLots(
		[]string{data.OpenPositionPrefix, data.ClosedPositionPrefix},
		func(prefix string) []string {
			return training.client.ListKeys(prefix)
		},
		training.client.DeletePrefix,
	)

	if err != nil {
		training.Error(errnie.Err(errnie.IO, "[training] drop stored lots after wallet reset", err))
	}

	training.archived = map[string]struct{}{}
}

func dropListedLots(prefixes []string, list func(string) []string, drop func(string) error) error {
	for _, prefix := range prefixes {
		for _, key := range list(prefix) {
			if err := drop(key); err != nil {
				return err
			}
		}
	}

	return nil
}

func positionFingerprint(position *broker.Position) string {
	qty, entry := "0", "0"

	if position.Qty() != nil {
		qty = position.Qty().String()
	}

	if position.EntryPrice() != nil {
		entry = position.EntryPrice().String()
	}

	return fmt.Sprintf("%s|%s|%t", qty, entry, position.Exiting())
}

func positionLayer(value *data.Value) []byte {
	if value == nil {
		return nil
	}

	for i, layer := range value.Layers {
		if len(layer) == 0 {
			continue
		}

		if i < len(value.Index) && strings.HasSuffix(value.Index[i], data.LayerPosition) {
			return layer
		}
	}

	for _, layer := range value.Layers {
		if len(layer) > 0 {
			return layer
		}
	}

	return nil
}

/*
writePosition publishes one training frame for a lot. The sale price, the
proceeds and the return are Price.Exit. A lot the book cannot sell leaves
the percent off.
*/
func (training *Training) writePosition(tick *data.Value, symbol string, position *broker.Position) {
	var (
		quote   exitQuote
		quoted  bool
		pnl     float64
		havePnL bool
	)

	values := []float64{0, 0, 0, 0, 0, 0, 0, 0}

	if position != nil {
		entry := decFloat(position.EntryPrice())
		qty := decFloat(position.Qty())
		quote, quoted = training.exitQuote(symbol, position.Qty())
		bid := entry
		value := entry * qty

		if sale, proceeds, ret, ok := training.price.Exit(symbol, position.Qty(), position.EntryPrice(), position.EntryFee()); ok {
			bid = sale
			value = proceeds
			pnl = ret
			havePnL = true
		}

		exiting := 0.0

		if position.Exiting() {
			exiting = 1
		}

		held := 0.0

		if opened := position.OpenedAt(); !opened.IsZero() {
			held = time.Since(opened).Seconds()
		}

		values = []float64{
			1,
			entry,
			bid,
			qty,
			entry * qty,
			value,
			exiting,
			held,
		}

		if havePnL {
			values = append(values, pnl)
		}
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
	}

	if len(values) > len(keys) {
		keys = append(keys, "position_pnl_pct")
	}

	if quoted {
		keys = append(keys, "position_exit_room")
		values = append(values, quote.room)

		if quote.haveVWAP {
			keys = append(keys, "position_exit_vwap")
			values = append(values, quote.vwap)
		}
	}

	if position != nil {
		keys, values = training.appendPositionReading(symbol, position, keys, values)
	}

	out := data.NewValue("training", "position", symbol)
	out.Epoch, out.Tick, out.Timestamp = tick.Epoch, tick.Tick, tick.Timestamp
	out.FormatPrefix()
	data.AppendVector(out, values, keys...)

	if err := training.writeFrame("", out); err != nil {
		training.Error(err)
	}
}

/*
appendPositionReading adds the readings the open-position card draws beside
the lot: bid depth versus size, this symbol's region brightness, the token
sequence it has walked, and how many stored paths still fit that sequence.
*/
func (training *Training) appendPositionReading(symbol string, position *broker.Position, keys []string, values []float64) ([]string, []float64) {
	var state *symbolGrammar

	if training != nil && training.symbols != nil {
		state = training.symbols[symbol]
	}

	walked := []string{}
	strength, threshold := 0.0, 0.0

	if state != nil {
		walked = append([]string{}, state.tokenPath...)
		strength, threshold = state.strength, state.threshold

		ids := make([]int, 0, len(state.brightness))

		for id := range state.brightness {
			ids = append(ids, id)
		}

		slices.Sort(ids)

		for _, id := range ids {
			keys = append(keys, fmt.Sprintf("region_brightness:R%02d", id))
			values = append(values, state.brightness[id])
		}

		for i, token := range walked {
			region, err := strconv.Atoi(strings.TrimPrefix(token, "R"))

			if err != nil || !strings.HasPrefix(token, "R") {
				continue
			}

			keys = append(keys, fmt.Sprintf("path_token:%d:0", i))
			values = append(values, float64(region))
		}
	}

	keys = append(keys, "token_depth", "token_capacity")
	values = append(values, float64(len(walked)), float64(pathCapacity()))

	if threshold > 0 {
		keys = append(keys, "token_strength", "token_threshold")
		values = append(values, strength, threshold)
	}

	return appendPathFit(keys, values, training.policy.Load(), walked)
}

type exitQuote struct {
	room     float64
	vwap     float64
	haveVWAP bool
}

func (training *Training) exitQuote(symbol string, qty *decimal.Decimal) (exitQuote, bool) {
	if training == nil || training.price == nil || qty == nil || qty.Sign() <= 0 || symbol == "" {
		return exitQuote{}, false
	}

	surface, err := training.price.SellQuote(symbol, qty)

	if err != nil || surface == nil || !surface.BookComplete || surface.ExecutableQty == nil || surface.ExecutableQty.Sign() <= 0 {
		return exitQuote{}, false
	}

	quote := exitQuote{room: surface.ExecutableQty.Div(qty).Float64()}

	if surface.FullyExecutable && surface.ExecutableVWAP != nil {
		quote.vwap = surface.ExecutableVWAP.Float64()
		quote.haveVWAP = true
	}

	return quote, true
}

func pathCapacity() int {
	if system.Cfg != nil && system.Cfg.Learning.MinimumPathConfidence >= 2 {
		return system.Cfg.Learning.MinimumPathConfidence
	}

	return 2
}

func appendPathFit(keys []string, values []float64, pol *policy, walked []string) ([]string, []float64) {
	type stored struct {
		tokens []string
		action string
	}

	paths := make([]stored, 0)

	if pol != nil {
		for key, action := range pol.actions {
			tokens := regionPath(key)

			if tokens == nil {
				continue
			}

			paths = append(paths, stored{tokens: tokens, action: action})
		}
	}

	count := func(prefix []string) (n, enter, exit int) {
		for _, path := range paths {
			if len(path.tokens) < len(prefix) {
				continue
			}

			if !slices.Equal(path.tokens[:len(prefix)], prefix) {
				continue
			}

			n++

			switch path.action {
			case "enter":
				enter++
			case "exit":
				exit++
			}
		}

		return n, enter, exit
	}

	storedCount, _, _ := count(walked)
	keys = append(keys, "stored_paths")
	values = append(values, float64(storedCount))

	for i := range walked {
		n, enter, exit := count(walked[:i+1])
		stage := fmt.Sprintf("%02d", i+1)
		keys = append(keys,
			"match_candidates:"+stage,
			"match_enter:"+stage,
			"match_exit:"+stage,
		)
		values = append(values, float64(n), float64(enter), float64(exit))
	}

	return keys, values
}

func regionPath(path string) []string {
	if path == "" {
		return nil
	}

	parts := strings.Split(path, "/")

	for _, part := range parts {
		if len(part) != 3 || part[0] != 'R' {
			return nil
		}

		if _, err := strconv.Atoi(part[1:]); err != nil {
			return nil
		}
	}

	return parts
}

/*
publishClosed writes each newly closed lot once: to the store, so a restart
can show it, and to the hub, so the life-cycle list can open it.
*/
func (training *Training) publishClosed() {
	if training == nil || training.desk == nil {
		return
	}

	if training.archived == nil {
		training.archived = map[string]struct{}{}
	}

	for _, position := range training.desk.Closed() {
		id := position.ID()

		if id == "" {
			continue
		}

		if _, seen := training.archived[id]; seen {
			continue
		}

		if err := training.noteClosed(position); err != nil {
			training.Error(errnie.Err(errnie.IO, "[training] note closed "+id, err))
			continue
		}

		raw, ok := closedSnapshot(position)

		if !ok {
			continue
		}

		if training.client != nil {
			stored := snapshotValue(position.Symbol(), raw)

			if err := training.client.Put(data.ClosedPositionKey(id), stored.Bytes()); err != nil {
				training.Error(errnie.Err(errnie.IO, "[training] store closed position "+id, err))
				continue
			}
		}

		training.publishSnapshot(id, position.Symbol(), raw)
		training.archived[id] = struct{}{}
	}
}

/*
replayClosed installs every stored closed life on the hub. The hub keeps
each one and sends it when a dashboard connects.
*/
func (training *Training) replayClosed() {
	if training == nil || training.client == nil || training.hub == nil {
		return
	}

	if training.archived == nil {
		training.archived = map[string]struct{}{}
	}

	for _, key := range training.client.ListKeys(data.ClosedPositionPrefix) {
		value, err := store.ReadValue(training.client, key)

		if err != nil {
			training.Error(err)
			continue
		}

		if err := training.writeFrame(key, value); err != nil {
			training.Error(err)
			continue
		}

		id := strings.TrimSuffix(strings.TrimPrefix(key, data.ClosedPositionPrefix), ".bin")
		training.archived[id] = struct{}{}
	}
}

func (training *Training) publishSnapshot(id, symbol string, raw []byte) {
	if err := training.writeFrame(data.ClosedPositionKey(id), snapshotValue(symbol, raw)); err != nil {
		training.Error(err)
	}
}

func snapshotValue(symbol string, raw []byte) *data.Value {
	out := data.NewValue(data.PositionOrigin, "closed", symbol)
	out.FormatPrefix()
	out.AppendJSON(raw, out.Prefix+"/"+data.LayerPosition)
	return out
}

type closedLife struct {
	ID       string             `json:"id"`
	Symbol   string             `json:"symbol"`
	Status   string             `json:"status"`
	OpenedAt string             `json:"opened_at"`
	ClosedAt string             `json:"closed_at"`
	Events   []broker.LifeEvent `json:"events"`
	Outcome  *closedOutcome     `json:"outcome"`
	Book     string             `json:"book,omitempty"`
}

type closedOutcome struct {
	Cost           float64  `json:"cost"`
	Proceeds       float64  `json:"proceeds"`
	Fees           float64  `json:"fees"`
	Realized       float64  `json:"realized"`
	NetReturn      float64  `json:"net_return"`
	HoldNs         int64    `json:"hold_ns"`
	ExpectedHoldNs *int64   `json:"expected_hold_ns"`
	Triggers       []string `json:"triggers"`
}

func closedSnapshot(position *broker.Position) ([]byte, bool) {
	if position == nil || !position.Closed() {
		return nil, false
	}

	life := position.Lifecycle()
	events := life.Events

	if events == nil {
		events = []broker.LifeEvent{}
	}

	outcome := &closedOutcome{Triggers: []string{}}

	if life.Outcome != nil {
		outcome.Cost = life.Outcome.VenueCost
		outcome.Proceeds = life.Outcome.VenueProceeds
		outcome.Fees = life.Outcome.VenueFees
		outcome.Realized = life.Outcome.VenueRealized
		outcome.HoldNs = life.Outcome.HoldNs

		if life.Outcome.ExpectedHoldNs != 0 {
			expected := life.Outcome.ExpectedHoldNs
			outcome.ExpectedHoldNs = &expected
		}

		if len(life.Outcome.Triggers) > 0 {
			outcome.Triggers = life.Outcome.Triggers
		}
	}

	if outcome.Cost > 0 {
		outcome.NetReturn = outcome.Realized / outcome.Cost
	}

	if len(outcome.Triggers) == 0 {
		for _, event := range events {
			if event.Kind == "exit_order" && event.Detail != "" {
				outcome.Triggers = append(outcome.Triggers, event.Detail)
			}
		}
	}

	closedAt := life.ClosedAt

	if closedAt == "" {
		closedAt = time.Now().UTC().Format(time.RFC3339)
	}

	raw, err := sonic.Marshal(closedLife{
		ID:       life.Id,
		Symbol:   life.Symbol,
		Status:   "closed",
		OpenedAt: life.OpenedAt,
		ClosedAt: closedAt,
		Events:   events,
		Outcome:  outcome,
	})

	if err != nil {
		return nil, false
	}

	return raw, true
}

func decFloat(value *decimal.Decimal) float64 {
	if value == nil {
		return 0
	}

	return value.Float64()
}
