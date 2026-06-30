package resonance

import (
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestSymbolRingRefreshCapacityConvertsNanosecondCadence(t *testing.T) {
	previous := viper.GetInt("signals.feed_ring_capacity")
	viper.Set("signals.feed_ring_capacity", 64)
	t.Cleanup(func() {
		viper.Set("signals.feed_ring_capacity", previous)
	})

	ring := &symbolRing{capacity: feedRingCapacity()}
	start := time.Unix(0, 0).UTC()

	ring.push([]byte(`{"symbol":"BTC/USD","price":1}`), 1, 0, start)
	ring.push([]byte(`{"symbol":"BTC/USD","price":2}`), 2, 0, start.Add(time.Second))
	ring.push([]byte(`{"symbol":"BTC/USD","price":3}`), 3, 0, start.Add(2*time.Second))

	if ring.capacity != 2 {
		t.Fatalf("capacity=%d, want cadence-derived two-sample ring", ring.capacity)
	}

	if len(ring.prices) != 2 || len(ring.stamps) != 2 {
		t.Fatalf("ring retained prices=%d stamps=%d, want 2/2", len(ring.prices), len(ring.stamps))
	}
}

func TestSymbolRingCapacityIsBoundedByConfiguredCeiling(t *testing.T) {
	previous := viper.GetInt("signals.feed_ring_capacity")
	viper.Set("signals.feed_ring_capacity", 8)
	t.Cleanup(func() {
		viper.Set("signals.feed_ring_capacity", previous)
	})

	ring := &symbolRing{capacity: feedRingCapacity()}
	start := time.Unix(0, 0).UTC()

	for index := range 32 {
		ring.push(
			[]byte(`{"symbol":"BTC/USD","price":1}`),
			1,
			0,
			start.Add(time.Duration(index)*time.Millisecond),
		)
	}

	if ring.capacity > 8 {
		t.Fatalf("capacity=%d, want configured ceiling <= 8", ring.capacity)
	}

	if len(ring.elements) > 8 || len(ring.prices) > 8 || len(ring.stamps) > 8 {
		t.Fatalf(
			"ring retained elements=%d prices=%d stamps=%d, want each <= 8",
			len(ring.elements),
			len(ring.prices),
			len(ring.stamps),
		)
	}
}
