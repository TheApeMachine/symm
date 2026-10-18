package broker

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
)

/*
PositionReading augments open-position hub frames with strategy-specific
metrics (token path, policy fit, and similar).
*/
type PositionReading func(symbol string, keys []string, values []float64) ([]string, []float64)

/*
ConfigurePositions binds object storage and an optional hub for open/closed
lot persistence and dashboard replay.
*/
func (desk *Desk) ConfigurePositions(client *store.Client, hub io.Writer) {
	if desk == nil {
		return
	}

	desk.positionStore = client
	desk.positionHub = hub

	if desk.openPublished == nil {
		desk.openPublished = map[string]string{}
	}

	if desk.closedArchived == nil {
		desk.closedArchived = map[string]struct{}{}
	}
}

func (desk *Desk) SetPositionHub(hub io.Writer) {
	if desk == nil {
		return
	}

	desk.positionHub = hub
}

func (desk *Desk) SetClosedHook(hook func(*Position) error) {
	if desk == nil {
		return
	}

	desk.onClosed = hook
}

/*
RestoreOpenLots installs every lot stored under position/open/ so a restart
can exit what the previous process entered.
*/
func (desk *Desk) RestoreOpenLots() {
	if desk == nil || desk.positionStore == nil {
		return
	}

	for _, key := range desk.positionStore.ListKeys(data.OpenPositionPrefix) {
		value, err := store.ReadValue(desk.positionStore, key)

		if err != nil {
			desk.Error(err)
			continue
		}

		raw := PositionLayer(value)

		if len(raw) == 0 {
			continue
		}

		symbol, err := desk.RestoreStored(raw)

		if err != nil {
			desk.Error(err)
			continue
		}

		if symbol == "" {
			continue
		}

		if desk.openPublished == nil {
			desk.openPublished = map[string]string{}
		}

		for _, position := range desk.OpenPositions() {
			if position != nil && position.Symbol() == symbol {
				desk.openPublished[symbol] = positionFingerprint(position)
			}
		}
	}
}

/*
DropStoredLots removes open and closed lots from the store after a wallet
reset so a later start does not restore them on an empty account.
*/
func (desk *Desk) DropStoredLots() error {
	if desk == nil || desk.positionStore == nil {
		return nil
	}

	err := dropListedLots(
		[]string{data.OpenPositionPrefix, data.ClosedPositionPrefix},
		func(prefix string) []string {
			return desk.positionStore.ListKeys(prefix)
		},
		desk.positionStore.DeletePrefix,
	)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "[desk] drop stored lots after wallet reset", err))
	}

	desk.closedArchived = map[string]struct{}{}

	return nil
}

/*
PublishHeld writes one hub frame per open lot, clears frames for symbols that
flatlined, persists open-lot changes, and archives newly closed lots.
*/
func (desk *Desk) PublishHeld(tick *data.Value, reading PositionReading) {
	if desk == nil || tick == nil {
		return
	}

	open := map[string]*Position{}

	for _, position := range desk.OpenPositions() {
		if position == nil || position.Symbol() == "" || position.Closed() {
			continue
		}

		open[position.Symbol()] = position
	}

	if desk.openPublished == nil {
		desk.openPublished = map[string]string{}
	}

	for symbol := range desk.openPublished {
		if _, still := open[symbol]; still {
			continue
		}

		desk.writePositionFrame(tick, symbol, nil, reading)
		desk.forgetOpen(symbol)
		delete(desk.openPublished, symbol)
	}

	for symbol, position := range open {
		desk.writePositionFrame(tick, symbol, position, reading)

		if fingerprint := positionFingerprint(position); desk.openPublished[symbol] != fingerprint {
			desk.persistOpen(symbol, position)
			desk.openPublished[symbol] = fingerprint
		}
	}

	desk.publishClosedLots()
}

func (desk *Desk) persistOpen(symbol string, position *Position) {
	if desk.positionStore == nil || position == nil || symbol == "" {
		return
	}

	raw, err := position.MarshalJSON()

	if err != nil {
		desk.Error(errnie.Err(errnie.Internal, "[desk] position of "+symbol, err))
		return
	}

	out := data.NewValue(data.PositionOrigin, "open", symbol)
	out.FormatPrefix()
	out.AppendJSON(raw, data.LayerPosition)

	if err := desk.positionStore.Put(data.OpenPositionKey(symbol), out.Bytes()); err != nil {
		desk.Error(errnie.Err(errnie.IO, "[desk] store open position "+symbol, err))
	}
}

func (desk *Desk) forgetOpen(symbol string) {
	if desk.positionStore == nil || symbol == "" {
		return
	}

	if err := desk.positionStore.DeletePrefix(data.OpenPositionKey(symbol)); err != nil {
		desk.Error(errnie.Err(errnie.IO, "[desk] remove open position "+symbol, err))
	}
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

func positionFingerprint(position *Position) string {
	qty, entry := "0", "0"

	if position.Qty() != nil {
		qty = position.Qty().String()
	}

	if position.EntryPrice() != nil {
		entry = position.EntryPrice().String()
	}

	return fmt.Sprintf("%s|%s|%t", qty, entry, position.Exiting())
}

/*
PositionLayer returns the position JSON layer from a stored Value.
*/
func PositionLayer(value *data.Value) []byte {
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

func (desk *Desk) writePositionFrame(tick *data.Value, symbol string, position *Position, reading PositionReading) {
	var (
		quote   exitQuote
		quoted  bool
		pnl     float64
		havePnL bool
	)

	values := []float64{0, 0, 0, 0, 0, 0, 0, 0}

	if position != nil {
		if desk.price != nil && (position.EntryPrice() == nil || position.EntryPrice().Sign() <= 0) {
			if live, _ := desk.price.Touch(symbol); live != nil && live.Sign() > 0 {
				position.SeedEntry(live, nil)
			}
		}

		entry := decFloat(position.EntryPrice())
		qty := decFloat(position.Qty())
		quote, quoted = desk.exitQuote(symbol, position.Qty())
		bid := entry
		value := entry * qty

		if desk.price != nil {
			if live, _ := desk.price.Touch(symbol); live != nil && live.Sign() > 0 {
				bid = live.Float64()
				value = bid * qty
			}
		}

		if desk.price != nil {
			if sale, proceeds, ret, ok := desk.price.Sale(symbol, position.Qty(), position.EntryPrice(), position.EntryFee()); ok {
				bid = sale
				value = proceeds
				pnl = ret
				havePnL = true
			}
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

	if position != nil && reading != nil {
		keys, values = reading(symbol, keys, values)
	}

	out := data.NewValue("training", "position", symbol)
	out.Epoch, out.Tick, out.Timestamp = tick.Epoch, tick.Tick, tick.Timestamp
	out.FormatPrefix()
	data.AppendVector(out, values, keys...)

	if err := desk.writePositionHub("", out); err != nil {
		desk.Error(err)
	}
}

type exitQuote struct {
	room     float64
	vwap     float64
	haveVWAP bool
}

func (desk *Desk) exitQuote(symbol string, qty *decimal.Decimal) (exitQuote, bool) {
	if desk.price == nil || qty == nil || qty.Sign() <= 0 || symbol == "" {
		return exitQuote{}, false
	}

	surface, err := desk.price.SellQuote(symbol, qty)

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

func (desk *Desk) publishClosedLots() {
	if desk.closedArchived == nil {
		desk.closedArchived = map[string]struct{}{}
	}

	for _, position := range desk.Closed() {
		id := position.ID()

		if id == "" {
			continue
		}

		if _, seen := desk.closedArchived[id]; seen {
			continue
		}

		raw, ok := ClosedSnapshot(position)

		if !ok {
			continue
		}

		if desk.positionStore != nil {
			stored := snapshotValue(position.Symbol(), raw)

			if err := desk.positionStore.Put(data.ClosedPositionKey(id), stored.Bytes()); err != nil {
				desk.Error(errnie.Err(errnie.IO, "[desk] store closed position "+id, err))
				continue
			}
		}

		desk.publishClosedSnapshot(id, position.Symbol(), raw)

		if desk.onClosed != nil {
			if err := desk.onClosed(position); err != nil {
				desk.Error(errnie.Err(errnie.IO, "[desk] note closed "+id, err))
			}
		}

		desk.closedArchived[id] = struct{}{}
	}
}

/*
ReplayClosedLots installs every stored closed life on the hub.
*/
func (desk *Desk) ReplayClosedLots() {
	if desk == nil || desk.positionStore == nil || desk.positionHub == nil {
		return
	}

	if desk.closedArchived == nil {
		desk.closedArchived = map[string]struct{}{}
	}

	for _, key := range desk.positionStore.ListKeys(data.ClosedPositionPrefix) {
		value, err := store.ReadValue(desk.positionStore, key)

		if err != nil {
			desk.Error(err)
			continue
		}

		if err := desk.writePositionHub(key, value); err != nil {
			desk.Error(err)
			continue
		}

		id := strings.TrimSuffix(strings.TrimPrefix(key, data.ClosedPositionPrefix), ".bin")
		desk.closedArchived[id] = struct{}{}
	}
}

func (desk *Desk) publishClosedSnapshot(id, symbol string, raw []byte) {
	if err := desk.writePositionHub(data.ClosedPositionKey(id), snapshotValue(symbol, raw)); err != nil {
		desk.Error(err)
	}
}

func snapshotValue(symbol string, raw []byte) *data.Value {
	out := data.NewValue(data.PositionOrigin, "closed", symbol)
	out.FormatPrefix()
	out.AppendJSON(raw, out.Prefix+"/"+data.LayerPosition)

	return out
}

func (desk *Desk) writePositionHub(key string, value *data.Value) error {
	if desk == nil || desk.positionHub == nil || value == nil {
		return nil
	}

	writer := desk.positionHub

	if key != "" {
		if kept, ok := desk.positionHub.(interface{ Retain(string) io.Writer }); ok {
			writer = kept.Retain(key)
		}
	}

	_, err := io.Copy(writer, value)

	return err
}

type lifeMark struct {
	At               string  `json:"at"`
	Bid              float64 `json:"bid"`
	Quantity         float64 `json:"quantity"`
	EntryPrice       float64 `json:"entry_price"`
	Unrealized       float64 `json:"unrealized"`
	UnrealizedReturn float64 `json:"unrealized_return"`
}

type closedLife struct {
	ID       string     `json:"id"`
	Symbol   string     `json:"symbol"`
	Status   string     `json:"status"`
	OpenedAt string     `json:"opened_at"`
	ClosedAt string     `json:"closed_at"`
	Events   []LifeEvent `json:"events"`
	Mark     *lifeMark  `json:"mark,omitempty"`
	Outcome  *closedOutcome `json:"outcome"`
	Book     string     `json:"book,omitempty"`
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

/*
ClosedSnapshot serializes a closed lot for the store and lifecycle dashboard.
*/
func ClosedSnapshot(position *Position) ([]byte, bool) {
	if position == nil || !position.Closed() {
		return nil, false
	}

	life := position.Lifecycle()
	events := life.Events

	if events == nil {
		events = []LifeEvent{}
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
