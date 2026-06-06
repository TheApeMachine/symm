package integration

import (
	"sort"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
	signalpool "github.com/theapemachine/symm/signal"
)

/*
FillEvent is one trader fill frame from the ui bus.
*/
type FillEvent struct {
	Symbol string
	Side   string
	Qty    float64
	Price  float64
}

/*
Tape collects measurements, actions, and wallet frames during a scenario.
*/
type Tape struct {
	mu sync.Mutex

	measurements []types.Measurement
	actions      []reasoning.Action
	wallets      []map[string]any
	fills        []FillEvent
	rawFrames    int
}

func NewTape() *Tape {
	return &Tape{
		measurements: make([]types.Measurement, 0, 256),
		actions:      make([]reasoning.Action, 0, 16),
		wallets:      make([]map[string]any, 0, 16),
		fills:        make([]FillEvent, 0, 8),
	}
}

func (tape *Tape) Subscribe(pool *qpool.Q) {
	measurements := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	measurementSub := measurements.Subscribe("integration:tape:measurements", 4096)

	raw := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
	rawSub := raw.Subscribe("integration:tape:raw", 4096)

	ui := pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	uiSub := ui.Subscribe("integration:tape:ui", 256)

	go tape.drainMeasurements(measurementSub)
	go tape.drainRaw(rawSub)
	go tape.drainUI(uiSub)
}

func (tape *Tape) drainMeasurements(subscriber *qpool.Subscriber) {
	if subscriber == nil {
		return
	}

	for message := range subscriber.Incoming {
		if message == nil || message.Value == nil {
			continue
		}

		reading, ok := message.Value.(types.Measurement)

		if !ok {
			continue
		}

		tape.mu.Lock()
		tape.measurements = append(tape.measurements, reading)
		tape.mu.Unlock()
	}
}

func (tape *Tape) drainRaw(subscriber *qpool.Subscriber) {
	if subscriber == nil {
		return
	}

	for message := range subscriber.Incoming {
		if message == nil || message.Value == nil {
			continue
		}

		tape.mu.Lock()
		tape.rawFrames++

		if action, ok := message.Value.(reasoning.Action); ok {
			tape.actions = append(tape.actions, action)
		}

		envelope, envelopeOK := message.Value.(*public.SocketMessage)

		if envelopeOK && envelope.Channel == public.ExecutionsChannel {
			executions := signalpool.GetExecutions(envelope)

			for _, execution := range executions {
				if execution.Symbol == "" || execution.LastQty <= 0 {
					continue
				}

				tape.fills = append(tape.fills, FillEvent{
					Symbol: execution.Symbol,
					Side:   execution.Side,
					Qty:    execution.LastQty,
					Price:  execution.LastPrice,
				})
			}
		}

		if envelope, ok := message.Value.(map[string]any); ok {
			fill := FillEvent{
				Symbol: stringFromAny(envelope["symbol"]),
				Side:   stringFromAny(envelope["side"]),
				Qty:    floatFromAny(envelope["qty"]),
				Price:  floatFromAny(envelope["price"]),
			}

			if envelope["channel"] == public.ExecutionsChannel &&
				fill.Symbol != "" &&
				fill.Qty > 0 {
				tape.fills = append(tape.fills, fill)
			}
		}

		tape.mu.Unlock()
	}
}

func (tape *Tape) drainUI(subscriber *qpool.Subscriber) {
	if subscriber == nil {
		return
	}

	for message := range subscriber.Incoming {
		if message == nil || message.Value == nil {
			continue
		}

		payload, ok := message.Value.(map[string]any)

		if !ok {
			continue
		}

		tape.mu.Lock()

		if payload["event"] == "wallet" {
			tape.wallets = append(tape.wallets, payload)

			tape.mu.Unlock()

			continue
		}

		fill := FillEvent{Side: stringFromAny(payload["Side"])}
		fill.Symbol = stringFromAny(payload["Symbol"])
		fill.Qty = floatFromAny(payload["Qty"])
		fill.Price = floatFromAny(payload["Price"])

		if fill.Symbol != "" && fill.Qty > 0 {
			tape.fills = append(tape.fills, fill)
		}

		tape.mu.Unlock()
	}
}

func stringFromAny(value any) string {
	text, _ := value.(string)

	return text
}

func floatFromAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func (tape *Tape) Snapshot(auditPath string) TapeSnapshot {
	tape.mu.Lock()
	defer tape.mu.Unlock()

	measurements := append([]types.Measurement(nil), tape.measurements...)
	actions := append([]reasoning.Action(nil), tape.actions...)
	wallets := append([]map[string]any(nil), tape.wallets...)
	fills := append([]FillEvent(nil), tape.fills...)

	auditRows, _ := readAuditRows(auditPath)

	return TapeSnapshot{
		Measurements: measurements,
		Actions:      actions,
		Wallets:      wallets,
		Fills:        fills,
		AuditRows:    auditRows,
		RawFrames:    tape.rawFrames,
		DeskReady:    len(wallets) > 0,
	}
}

/*
TapeSnapshot is a point-in-time view of collected scenario telemetry.
*/
type TapeSnapshot struct {
	Measurements []types.Measurement
	Actions      []reasoning.Action
	Wallets      []map[string]any
	Fills        []FillEvent
	AuditRows    []AuditRow
	RawFrames    int
	DeskReady    bool
}

func (snapshot TapeSnapshot) latestBySource(source types.SourceType) types.Measurement {
	var latest types.Measurement

	for _, reading := range snapshot.Measurements {
		if reading.Source != source {
			continue
		}

		latest = reading
	}

	return latest
}

func (snapshot TapeSnapshot) latestBySourceSymbol(
	source types.SourceType,
	symbol string,
) types.Measurement {
	var latest types.Measurement

	for _, reading := range snapshot.Measurements {
		if reading.Source != source || reading.Symbol != symbol {
			continue
		}

		latest = reading
	}

	return latest
}

func (snapshot TapeSnapshot) countBySource(source types.SourceType) int {
	count := 0

	for _, reading := range snapshot.Measurements {
		if reading.Source == source {
			count++
		}
	}

	return count
}

func (snapshot TapeSnapshot) countsBySource() map[string]int {
	counts := make(map[string]int)

	for _, reading := range snapshot.Measurements {
		name := reading.Source.String()

		if name == "" {
			continue
		}

		counts[name]++
	}

	return counts
}

func (snapshot TapeSnapshot) categoriesForSource(source types.SourceType) []string {
	seen := make(map[types.CategoryType]struct{})
	out := make([]string, 0)

	for _, reading := range snapshot.Measurements {
		if reading.Source != source {
			continue
		}

		if _, ok := seen[reading.Category]; ok {
			continue
		}

		seen[reading.Category] = struct{}{}
		out = append(out, string(reading.Category))
	}

	sort.Strings(out)

	return out
}

func (snapshot TapeSnapshot) hasCategory(
	source types.SourceType,
	category types.CategoryType,
) bool {
	for _, reading := range snapshot.Measurements {
		if reading.Source == source && reading.Category == category {
			return true
		}
	}

	return false
}

func (snapshot TapeSnapshot) initialWalletBalance() float64 {
	if len(snapshot.Wallets) == 0 {
		return 0
	}

	balance := walletBalance(snapshot.Wallets[0])

	return balance
}

func (snapshot TapeSnapshot) lastWalletBalance() float64 {
	if len(snapshot.Wallets) == 0 {
		return 0
	}

	last := snapshot.Wallets[len(snapshot.Wallets)-1]
	balance := walletBalance(last)

	return balance
}

func (snapshot TapeSnapshot) lastInventoryMap() map[string]float64 {
	if len(snapshot.Wallets) == 0 {
		return nil
	}

	last := snapshot.Wallets[len(snapshot.Wallets)-1]
	inventory, _ := last["Inventory"].(map[string]float64)

	return inventory
}

func walletBalance(frame map[string]any) float64 {
	balance, ok := frame["Balance"].(float64)

	if ok {
		return balance
	}

	return floatFromAny(frame["balance"])
}

func (snapshot TapeSnapshot) lastInventory(baseAsset string) float64 {
	inventory := snapshot.lastInventoryMap()

	if inventory == nil {
		return 0
	}

	return inventory[baseAsset]
}
