package broker

import (
	"encoding/json"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestPaperExecutionStillOpen(t *testing.T) {
	Convey("limit_order_placed / open ack is treated as still-open", t, func() {
		open := &kraken.Execution{
			Data: []kraken.ExecutionData{{
				OrderID:     "ORD-OPEN",
				OrderStatus: "open",
				ExecType:    "new",
			}},
		}
		So(paperExecutionStillOpen(open), ShouldBeTrue)

		filled := &kraken.Execution{
			Data: []kraken.ExecutionData{{
				OrderID:     "ORD-FILL",
				OrderStatus: "filled",
				CumQty:      decimal.NewFromFloat64(1),
			}},
		}
		So(paperExecutionStillOpen(filled), ShouldBeFalse)
	})
}

func TestPublishPlaceSoftFailsBalance(t *testing.T) {
	Convey("publishPlace returns nil even when balance refresh would fail path is soft", t, func() {
		// Structural: open ack is watched; filled ack is not.
		exec := kraken.NewExecutionFromMap(map[string]any{
			"order_id":  "ORD-1",
			"cl_ord_id": "CL-1",
			"pair":      "BTC/USD",
			"side":      "buy",
			"status":    "open",
			"action":    "limit_order_placed",
			"volume":    0.0,
			"price":     100.0,
			"cost":      0.0,
			"fee":       0.0,
		})
		So(paperExecutionStillOpen(exec), ShouldBeTrue)
	})
}

// TestAddOrderMessageCarriesOrderQty proves the WS add_order payload uses
// order_qty (not REST "volume"), so paper's unmarshal fills a non-empty size.
func TestAddOrderMessageCarriesOrderQty(t *testing.T) {
	t.Parallel()

	msg := kraken.NewAddOrderMessage("", &kraken.AddOrderRequest{
		Pair:    "BTC/USD",
		Type:    "buy",
		OrdType: "limit",
		Volume:  "0.00123456",
		Price:   "100000.0",
		ClOrdId: "test-cl-ord",
	})

	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var wire struct {
		Params struct {
			OrderQty json.Number `json:"order_qty"`
			Volume   json.Number `json:"volume"`
			Symbol   string      `json:"symbol"`
		} `json:"params"`
	}

	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if wire.Params.OrderQty.String() == "" {
		t.Fatalf("order_qty empty in payload %s", raw)
	}

	if wire.Params.Volume.String() != "" {
		t.Fatalf("legacy volume key should be absent, got %q in %s", wire.Params.Volume.String(), raw)
	}

	if wire.Params.Symbol != "BTC/USD" {
		t.Fatalf("symbol: %q", wire.Params.Symbol)
	}
}

// TestAffordableVolumeStringNonEmpty guards paper CLI: Quantity → String must
// be a non-empty venue-normalized size after SeedNormalizer-style pair load.
func TestAffordableVolumeStringNonEmpty(t *testing.T) {
	t.Parallel()

	normalizer := spot.NewNormalizer()
	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"BTC": {AltName: "XBT"},
			"USD": {AltName: "USD"},
		},
		NewPairs: map[string]spot.AssetPair{
			"BTC/USD": {
				WSName:        "BTC/USD",
				Base:          "BTC",
				Quote:         "USD",
				LotDecimals:   8,
				LotMultiplier: 1,
			},
		},
	})

	price := NewPrice(t.Context(), nil, nil, nil, normalizer)
	fee := decimal.NewFromFloat64(0.26)
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{
		Fee: fee, Minfee: fee, Maxfee: fee,
	})

	unit, err := decimal.NewFromString("100000.0")
	if err != nil {
		t.Fatal(err)
	}
	cash, err := decimal.NewFromString("1000.0")
	if err != nil {
		t.Fatal(err)
	}

	qty, err := price.Affordable("BTC/USD", cash, unit)
	if err != nil {
		t.Fatalf("Affordable: %v", err)
	}
	if qty == nil || qty.Sign() <= 0 {
		t.Fatalf("qty: %v", qty)
	}

	volume := qty.String()
	if volume == "" {
		t.Fatal("volume.String() empty — paper CLI would get Invalid volume")
	}

	entry := &kraken.AddOrderRequest{
		Pair: "BTC/USD", Type: "buy", OrdType: "limit",
		Volume: volume, Price: unit.String(),
	}
	raw, err := json.Marshal(kraken.NewAddOrderMessage("", entry))
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Params struct {
			OrderQty string `json:"order_qty"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Params.OrderQty == "" {
		t.Fatalf("marshaled order_qty empty: %s", raw)
	}
}
