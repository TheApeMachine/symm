package hindsight

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
)

type memStore struct {
	mu   sync.Mutex
	keys map[string][]byte
}

func (store *memStore) Write(p []byte) (int, error) {
	return 0, errnie.Err(errnie.Validation, "write without a prefix", nil)
}

func (store *memStore) WriteAtPrefix(prefix string, p []byte) (int, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	if store.keys == nil {
		store.keys = map[string][]byte{}
	}

	store.keys[prefix] = append([]byte(nil), p...)
	return len(p), nil
}

func TestCommitWritesTheClosedLot(t *testing.T) {
	opened := time.Date(2026, 10, 23, 20, 35, 45, 0, time.UTC)
	closed := opened.Add(20 * time.Minute)
	desk := &broker.Desk{}
	desk.ArchiveVirtual(&broker.Position{
		ID:          "lot-1",
		Symbol:      "BTC/USD",
		Virtual:     true,
		PolicyEpoch: 1792786614875161800,
		EntryTick:   8556,
		ExitTick:    16288,
		EntryPrice:  decimal.NewFromInt64(100),
		Proceeds:    decimal.NewFromInt64(281),
		Qty:         decimal.NewFromInt64(2),
		OpenedAt:    opened,
		ClosedAt:    &closed,
	})

	store := &memStore{}
	body := []byte(`{"run":"1792786614875161800","symbol":"BTC/USD","class":"up","a":"8555","b":"8556","c":"16288","d":"16288"}`)

	payload, err := Commit(desk, store, body)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(payload, []byte(`"a":"8555"`)) {
		t.Fatalf("response = %s", payload)
	}

	const key = "detections/up/BTC/USD/1792786614875161800/0000000000000016288"
	raw, ok := store.keys[key]
	if !ok {
		t.Fatalf("stored keys = %v, want %s", store.keys, key)
	}

	value := data.From(raw)
	fields := metrics(value)
	if fields["a_tick"] != 8555 || fields["b_tick"] != 8556 || fields["c_tick"] != 16288 {
		t.Fatalf("ticks = a %v b %v c %v", fields["a_tick"], fields["b_tick"], fields["c_tick"])
	}

	if fields["b_price"] != 100 || fields["c_price"] != 140.5 {
		t.Fatalf("prices = entry %v exit %v, want 100 and 140.5", fields["b_price"], fields["c_price"])
	}
}

func TestCommitRejectsAMissingLot(t *testing.T) {
	desk := &broker.Desk{}
	_, err := Commit(desk, &memStore{}, []byte(`{"run":"1","symbol":"BTC/USD","class":"up","a":"1","b":"2","c":"3","d":"4"}`))
	if err == nil || !errnie.IsNotFound(err) {
		t.Fatalf("err = %v, want not found", err)
	}
}
