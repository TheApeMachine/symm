package broker

import (
	"sync"
	"testing"

	"github.com/theapemachine/symm/data"
)

func TestTradesReadsGobLayer(t *testing.T) {
	raw := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":"buy","price":"100","qty":1,"ord_type":"market","trade_id":1,"timestamp":"2026-10-18T08:00:00Z"}]}`)
	value := data.NewValue("workspace", "tick", "aggregate")
	value.Append(raw, "spot:trade", "update")

	trades, err := Trades(value)

	if err != nil {
		t.Fatal(err)
	}

	if len(trades) != 1 || trades[0].Symbol != "BTC/USD" || trades[0].Side != "buy" {
		t.Fatalf("trades = %+v", trades)
	}
}

func TestConcurrentSignalAppendKeepsTrade(t *testing.T) {
	raw := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":"buy","price":"100","qty":1,"ord_type":"market","trade_id":1,"timestamp":"2026-10-18T08:00:00Z"}]}`)
	value := data.NewValue("workspace", "tick", "aggregate")
	value.Append(raw, "spot:trade", "update")
	value.Append([]byte(`{"bids":1}`), "book", "BTC/USD")
	value.Append([]byte(`{"balance":1}`), "balance")

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := Trades(value); err != nil {
				t.Error(err)
				return
			}

			value.Append([]float64{1}, "signal", "cvd", "gross_notional", "symbol:BTC/USD")
		}()
	}

	wg.Wait()
}
