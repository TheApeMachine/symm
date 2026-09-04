package cvd

import (
	"testing"
	"time"
)

func TestDbg(t *testing.T) {
	entity := NewTrade()
	m := entity.Step(cvdTrade("BTC/USD", "buy", 100, 2, time.Unix(1000, 0)))
	if m == nil {
		t.Fatal("nil measurement")
	}
	t.Logf("err=%v metrics=%d", m.Err, len(m.Metrics))
	for k, v := range m.Metrics {
		t.Logf("  %s = %v", k, v.Raw)
	}
}
