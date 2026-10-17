package keyindex

import "testing"

func TestParseAndScopeView(t *testing.T) {
	t.Parallel()

	key := "workspace/tick/aggregate/0000000000000000007/0000000000000000042.bin"
	entry, err := ParseObjectKey(key)

	if err != nil {
		t.Fatal(err)
	}

	if entry.Scope != "aggregate" || entry.Epoch != 7 || entry.Tick != 42 {
		t.Fatalf("entry = %+v", entry)
	}

	idx, err := New().InsertObjectKey(key)

	if err != nil {
		t.Fatal(err)
	}

	var seen int

	idx.WalkScopePrefix("aggregate", func(got *Entry) bool {
		if got.Canonical != key {
			t.Fatalf("canonical = %q", got.Canonical)
		}

		seen++

		return true
	})

	if seen != 1 {
		t.Fatalf("scope walk seen = %d", seen)
	}
}

func TestObjectCache(t *testing.T) {
	t.Parallel()

	key := "draft/up/BTC/USD/0000000000000000007/0000000000000000001.bin"
	idx, err := New().InsertObjectKey(key)

	if err != nil {
		t.Fatal(err)
	}

	idx = idx.StoreObject(key, []byte("payload"))

	buf, ok := idx.Object(key)

	if !ok || string(buf) != "payload" {
		t.Fatalf("cache miss or wrong body: %q ok=%t", buf, ok)
	}
}
