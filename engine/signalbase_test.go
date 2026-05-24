package engine

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestSignalBaseScanSymbolsEnqueuesMeasurement(t *testing.T) {
	config.System.MaxScanSymbols = 0

	watch := NewSymbolWatch([]string{"PUMP/EUR"})
	watch.NoteTrade("PUMP/EUR", 1)

	base, err := NewSignalBase(
		context.Background(),
		"test",
		nil,
		nil,
		nil,
		map[string]asset.Pair{"PUMP/EUR": {Wsname: "PUMP/EUR"}},
		[]string{"PUMP/EUR"},
		watch,
	)

	if err != nil {
		t.Fatalf("new signal base: %v", err)
	}

	err = base.ScanSymbols(time.Now(), func(_ string, _ Snapshot) (Measurement, bool, error) {
		return Measurement{
			Type:       Pump,
			Regime:     "pump",
			Confidence: 0.5,
		}, true, nil
	})

	if err != nil {
		t.Fatalf("scan symbols: %v", err)
	}

	count := 0

	for range base.Measure(context.Background()) {
		count++
	}

	if count != 1 {
		t.Fatalf("expected one queued measurement, got %d", count)
	}
}
