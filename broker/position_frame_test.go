package broker

import (
	"strings"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
)

func TestClosedSnapshotIsALifeTheDashboardCanOpen(t *testing.T) {
	position := NewPosition()
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

	raw, ok := ClosedSnapshot(position)

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

	if life.Outcome.Cost != 0 || life.Outcome.Realized != 0 {
		t.Fatalf("fills became a profit: %+v", life.Outcome)
	}

	if len(life.Events) < 2 || len(life.Outcome.Triggers) == 0 {
		t.Fatalf("events %d triggers %v", len(life.Events), life.Outcome.Triggers)
	}

	if data.ClosedPositionKey(position.ID()) != "position/closed/"+position.ID()+".bin" {
		t.Fatalf("key %s", data.ClosedPositionKey(position.ID()))
	}
}

func TestPositionLayerRoundTrip(t *testing.T) {
	position := NewPosition()
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

	got := PositionLayer(value)

	if string(got) != string(raw) {
		t.Fatalf("layer %s", got)
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
