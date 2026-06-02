package integration

import (
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
Tape collects measurements, actions, and wallet frames during a scenario.
*/
type Tape struct {
	mu sync.Mutex

	measurements []perspectives.Measurement
	actions      []perspectives.Action
	wallets      []map[string]any
	rawFrames    int
}

func NewTape() *Tape {
	return &Tape{
		measurements: make([]perspectives.Measurement, 0, 64),
		actions:      make([]perspectives.Action, 0, 8),
		wallets:      make([]map[string]any, 0, 4),
	}
}

func (tape *Tape) Subscribe(pool *qpool.Q) {
	measurements := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	measurementSub := measurements.Subscribe("integration:tape:measurements", 4096)

	raw := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
	rawSub := raw.Subscribe("integration:tape:raw", 4096)

	ui := pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	uiSub := ui.Subscribe("integration:tape:ui", 64)

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

		reading, ok := message.Value.(perspectives.Measurement)

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

		if action, ok := message.Value.(perspectives.Action); ok {
			tape.actions = append(tape.actions, action)
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

		if !ok || payload["event"] != "wallet" {
			continue
		}

		tape.mu.Lock()
		tape.wallets = append(tape.wallets, payload)
		tape.mu.Unlock()
	}
}

func (tape *Tape) Snapshot() TapeSnapshot {
	tape.mu.Lock()
	defer tape.mu.Unlock()

	measurements := append([]perspectives.Measurement(nil), tape.measurements...)
	actions := append([]perspectives.Action(nil), tape.actions...)
	wallets := append([]map[string]any(nil), tape.wallets...)

	return TapeSnapshot{
		Measurements: measurements,
		Actions:      actions,
		Wallets:      wallets,
		RawFrames:    tape.rawFrames,
		DeskReady:    trading.DeskReady(),
	}
}

/*
TapeSnapshot is a point-in-time view of collected scenario telemetry.
*/
type TapeSnapshot struct {
	Measurements []perspectives.Measurement
	Actions      []perspectives.Action
	Wallets      []map[string]any
	RawFrames    int
	DeskReady    bool
}

func (snapshot TapeSnapshot) latestBySource(source perspectives.SourceType) perspectives.Measurement {
	var latest perspectives.Measurement

	for _, reading := range snapshot.Measurements {
		if reading.Source != source {
			continue
		}

		latest = reading
	}

	return latest
}

func (snapshot TapeSnapshot) categoriesForSource(source perspectives.SourceType) []string {
	seen := make(map[perspectives.CategoryType]struct{})
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

	return out
}

func (snapshot TapeSnapshot) hasCategory(
	source perspectives.SourceType,
	category perspectives.CategoryType,
) bool {
	for _, reading := range snapshot.Measurements {
		if reading.Source == source && reading.Category == category {
			return true
		}
	}

	return false
}

func (snapshot TapeSnapshot) lastWalletBalance() float64 {
	if len(snapshot.Wallets) == 0 {
		return 0
	}

	last := snapshot.Wallets[len(snapshot.Wallets)-1]
	balance, _ := last["Balance"].(float64)

	return balance
}
