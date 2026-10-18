package data

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestAppendAndLayerRoundTrip(t *testing.T) {
	value := NewValue("kraken", "trade", "BTC/USD")

	book := []byte("book-bytes")
	value.Append(book, "book")

	if value.Err != nil {
		t.Fatalf("append book: %v", value.Err)
	}

	var gotBook []byte

	for _, layer := range Layer[[]byte](value, "book") {
		gotBook = layer
	}

	if value.Err != nil {
		t.Fatalf("layer book: %v", value.Err)
	}

	if string(gotBook) != string(book) {
		t.Fatalf("book %q", gotBook)
	}

	prices := []map[string]int{{"n": 1}, {"n": 2}}
	value.Append(prices, "price")

	var gotPrices []map[string]int

	for _, layer := range Layer[[]map[string]int](value, value.Prefix+"/price") {
		gotPrices = layer
	}

	if value.Err != nil {
		t.Fatalf("layer price: %v", value.Err)
	}

	if len(gotPrices) != 2 || gotPrices[0]["n"] != 1 || gotPrices[1]["n"] != 2 {
		t.Fatalf("prices %#v", gotPrices)
	}

	seen := 0

	for _, layer := range Layer[[]byte](value, "book") {
		seen++

		if string(layer) != string(book) {
			t.Fatalf("layers book %q", layer)
		}
	}

	if seen != 1 {
		t.Fatalf("seen %d", seen)
	}

	missing := NewValue("kraken", "trade", "BTC/USD")

	for range Layer[[]byte](missing, "book") {
	}

	if missing.Err == nil {
		t.Fatal("missing layer returned no error")
	}
}

func TestLayerReadsPackedFloat64(t *testing.T) {
	values := []float64{0.02764, 0.02772, 0.00289}
	packed := make([]byte, len(values)*8)

	for i, value := range values {
		binary.LittleEndian.PutUint64(packed[i*8:], math.Float64bits(value))
	}

	stored := New()
	stored.Layers = [][]byte{packed}
	stored.Index = []string{"signal/liquidity/best_price:bid/best_price:ask/relative_spread/symbol:CPOOL/USD"}

	var got []float64

	for _, layer := range Layer[[]float64](stored, "signal") {
		got = layer
	}

	if stored.Err != nil {
		t.Fatal(stored.Err)
	}

	if len(got) != len(values) || got[0] != values[0] || got[2] != values[2] {
		t.Fatalf("packed %#v", got)
	}
}
