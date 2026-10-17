package strategy

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/theapemachine/symm/data"
	nmstore "github.com/theapemachine/symm/nomagique/store"
)

func TestPublishSequiturFrameShape(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	training := &Training{hub: &buf}
	training.policy.Store(&policy{
		keys:    []string{"R02/R04/enter.bin"},
		actions: map[string]string{"R02/R04": "enter"},
	})

	tick := data.NewValue("workspace", "tick", "aggregate")
	tick.Epoch = 7
	tick.Tick = 41
	tick.Timestamp = 99

	err := training.publishSequitur(
		tick,
		"BTC/USD",
		[]*nmstore.Region{{Key: 2, Brightness: 0.8}, {Key: 4, Brightness: 0.4}},
		0.6,
		[]string{"R02", "R04"},
		0,
		"R02/R04",
		nil,
		"enter",
		"",
	)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	got := data.From(buf.Bytes())
	if got.Origin != data.SequiturOrigin || got.Role != data.SequiturRole || got.Scope != "BTC/USD" {
		t.Fatalf("value %s/%s/%s", got.Origin, got.Role, got.Scope)
	}

	var raw []byte
	for i, index := range got.Index {
		if index == data.LayerGrammar && i < len(got.Layers) {
			raw = got.Layers[i]
		}
	}
	if len(raw) == 0 {
		t.Fatal("grammar layer missing")
	}

	var frame sequiturWire
	if err := json.Unmarshal(raw, &frame); err != nil {
		t.Fatalf("layer: %v", err)
	}
	if frame.Symbol != "BTC/USD" || frame.Epoch != "7" || frame.Tick != 41 || frame.Blocked {
		t.Fatalf("frame %+v", frame)
	}
	if len(frame.Fired) != 2 || frame.Policy.Action != "enter" || !frame.Policy.Trading {
		t.Fatalf("chord %+v policy %+v", frame.Fired, frame.Policy)
	}
	if frame.Events == nil || frame.Rules == nil || frame.Violations == nil {
		t.Fatal("dashboard drops a frame whose arrays are null")
	}
}
