package strategy

import (
	"strings"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
)

func TestClosedSnapshotIsALifeTheDashboardCanOpen(t *testing.T) {
	position := broker.NewPosition()
	position.Enter("TEL/USD", decimal.NewFromFloat64(2), "R13/R04")
	position.Apply(kraken.ExecutionData{
		Symbol:      "TEL/USD",
		Side:        "buy",
		ExecType:    "trade",
		CumQty:      decimal.NewFromFloat64(2),
		LastQty:     decimal.NewFromFloat64(2),
		LastPrice:   decimal.NewFromFloat64(10),
		FeeUsdEquiv: decimal.NewFromFloat64(0.05),
	})
	position.Exit("TEL/USD")
	position.Apply(kraken.ExecutionData{
		Symbol:      "TEL/USD",
		Side:        "sell",
		ExecType:    "trade",
		LastQty:     decimal.NewFromFloat64(2),
		LastPrice:   decimal.NewFromFloat64(11),
		FeeUsdEquiv: decimal.NewFromFloat64(0.05),
	})

	raw, ok := closedSnapshot(position)

	if !ok {
		t.Fatal("closed lot produced no snapshot")
	}

	var life struct {
		Status string `json:"status"`
		Symbol string `json:"symbol"`
		Events []struct {
			Kind string `json:"kind"`
		} `json:"events"`
		Outcome struct {
			Realized float64  `json:"realized"`
			Cost     float64  `json:"cost"`
			Triggers []string `json:"triggers"`
		} `json:"outcome"`
	}

	if err := sonic.Unmarshal(raw, &life); err != nil {
		t.Fatal(err)
	}

	if life.Status != "closed" || life.Symbol != "TEL/USD" {
		t.Fatalf("life %+v", life)
	}

	if life.Outcome.Cost != 20 || life.Outcome.Realized != 1.9 {
		t.Fatalf("outcome %+v", life.Outcome)
	}

	if len(life.Events) < 2 || len(life.Outcome.Triggers) == 0 {
		t.Fatalf("events %d triggers %v", len(life.Events), life.Outcome.Triggers)
	}

	if data.ClosedPositionKey(position.ID()) != "position/closed/"+position.ID()+".bin" {
		t.Fatalf("key %s", data.ClosedPositionKey(position.ID()))
	}
}

func TestPositionLayerRoundTrip(t *testing.T) {
	position := broker.NewPosition()
	position.Enter("RLC/USD", decimal.NewFromFloat64(12.5), "R01/R04")

	raw, err := position.MarshalJSON()

	if err != nil {
		t.Fatal(err)
	}

	value := data.NewValue(data.PositionOrigin, "open", "RLC/USD")
	value.FormatPrefix()
	value.AppendJSON(raw, data.LayerPosition)

	if data.OpenPositionKey("RLC/USD") != "position/open/RLC/USD.bin" {
		t.Fatalf("key %s", data.OpenPositionKey("RLC/USD"))
	}

	got := positionLayer(value)

	if string(got) != string(raw) {
		t.Fatalf("layer %s", got)
	}
}

func TestPositionReadingFeedsTheOpenCard(t *testing.T) {
	training := &Training{
		symbols: map[string]*symbolGrammar{
			"BTC/USD": {
				tokenPath:  []string{"R13", "R04"},
				brightness: map[int]float64{13: 0.8, 4: 0.2, 0: 0},
				strength:   0.8,
				threshold:  0.5,
			},
		},
	}
	training.policy.Store(&policy{
		actions: map[string]string{
			"R13/R04":     "enter",
			"R13/R04/R02": "exit",
			"R09/R12":     "noop",
			"BTC/USD/R13": "enter",
		},
	})

	position := broker.NewPosition()
	position.Enter("BTC/USD", decimal.NewFromFloat64(2), "R13/R04")

	keys, values := training.appendPositionReading("BTC/USD", position, nil, nil)
	got := map[string]float64{}

	for i, key := range keys {
		got[key] = values[i]
	}

	if got["region_brightness:R13"] != 0.8 || got["region_brightness:R00"] != 0 {
		t.Fatalf("brightness %#v", got)
	}

	if got["path_token:0:0"] != 13 || got["path_token:1:0"] != 4 {
		t.Fatalf("tokens %#v", got)
	}

	if got["token_depth"] != 2 || got["token_capacity"] != 2 {
		t.Fatalf("depth %#v", got)
	}

	if got["token_strength"] != 0.8 || got["token_threshold"] != 0.5 {
		t.Fatalf("strength %#v", got)
	}

	if got["stored_paths"] != 2 {
		t.Fatalf("stored paths = %v", got["stored_paths"])
	}

	if got["match_candidates:01"] != 2 || got["match_enter:01"] != 1 || got["match_exit:01"] != 1 {
		t.Fatalf("stage 1 %#v", got)
	}

	if got["match_candidates:02"] != 2 || got["match_enter:02"] != 1 || got["match_exit:02"] != 1 {
		t.Fatalf("stage 2 %#v", got)
	}
}

func TestWalletResetDropsStoredLots(t *testing.T) {
	stored := map[string]struct{}{
		"position/open/MANA/USD.bin":   {},
		"position/closed/old.bin":      {},
		"policy/current/R01/enter.bin": {},
	}

	err := dropListedLots(
		[]string{data.OpenPositionPrefix, data.ClosedPositionPrefix},
		func(prefix string) []string {
			keys := make([]string, 0)

			for key := range stored {
				if strings.HasPrefix(key, prefix) {
					keys = append(keys, key)
				}
			}

			return keys
		},
		func(key string) error {
			delete(stored, key)
			return nil
		},
	)

	if err != nil {
		t.Fatal(err)
	}

	if _, ok := stored["position/open/MANA/USD.bin"]; ok {
		t.Fatal("open lot survived the wallet reset")
	}

	if _, ok := stored["position/closed/old.bin"]; ok {
		t.Fatal("closed lot survived the wallet reset")
	}

	if _, ok := stored["policy/current/R01/enter.bin"]; !ok {
		t.Fatal("the reset removed a policy")
	}
}
