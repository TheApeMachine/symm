package parallel

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

func TestLedgerJSONRoundTrip(t *testing.T) {
	ledger := NewLedger()

	qty := decimal.NewFromFloat64(0.01)
	entry := decimal.NewFromFloat64(50000)
	fee := decimal.NewFromFloat64(1.5)

	ledger.Open(BookTreatment, &Lot{
		ID:        "lot-1",
		Symbol:    "BTC/USD",
		Book:      BookTreatment,
		EntryPath: "parallel/B",
		Qty:       qty,
		Entry:     entry,
		Fee:       fee,
	})
	ledger.returns[BookControl] = []float64{0.01, -0.005}

	blob, err := ledger.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	restored := NewLedger()
	if err := restored.Unmarshal(blob); err != nil {
		t.Fatal(err)
	}

	lot := restored.OpenLot(BookTreatment, "BTC/USD")
	if lot == nil || lot.Qty == nil || lot.Qty.Cmp(qty) != 0 {
		t.Fatalf("qty not restored: %+v", lot)
	}

	if len(restored.returns[BookControl]) != 2 {
		t.Fatalf("returns not restored: %+v", restored.returns)
	}
}
