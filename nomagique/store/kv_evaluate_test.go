package store

import (
	"errors"
	"reflect"
	"testing"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestKVNextEvaluateIsolatesSymbolsAndEpochs(t *testing.T) {
	created := 0
	op := NewKV(func() core.Primitive {
		created++
		return nomagique.NewNumber(arithmetic.NewSubtract(), statistic.NewSum())
	})
	// Each three-item row is an address and two scalar operands, not a packed
	// arithmetic input. The structs are test fixtures only.
	observations := []struct {
		key             string
		buy, sell, want float64
	}{
		{"BTC/USD:1", 10, 0, 10},
		{"ETH/USD:1", 100, 0, 100},
		{"BTC/USD:1", 0, 3, 7},
		{"ETH/USD:1", 0, 25, 75},
		{"BTC/USD:2", 2, 0, 2},
		{"BTC/USD:1", 5, 0, 12},
		{"BTC/USD:2", 0, 2, 0},
	}
	for _, observation := range observations {
		msg := data.NewMessage(data.EVALUATE, "symbolstore", observation.key, data.NewValue(observation.buy, observation.sell))
		got := []float64{}
		for ptr := range op.Next(msg.Next(nil)) {
			got = append(got, *(*float64)(ptr))
		}
		if !reflect.DeepEqual(got, []float64{observation.want}) {
			t.Fatalf("%s: got %v, want %v", observation.key, got, observation.want)
		}
		if err := op.Error(); err != nil {
			t.Fatal(err)
		}
	}
	if created != 3 {
		t.Fatalf("factories called %d times, want once per distinct scope (3)", created)
	}
}

func TestKVNextReadWriteKeepStorageSemantics(t *testing.T) {
	op := NewKV()
	payload := data.NewValue(4.0, 9.0)
	write := data.NewMessage(data.WRITE, "state", "key", payload)
	got := []float64{}
	for ptr := range op.Next(write.Next(nil)) {
		got = append(got, *(*float64)(ptr))
	}
	if !reflect.DeepEqual(got, []float64{4, 9}) {
		t.Fatalf("WRITE = %v", got)
	}
	read := data.NewMessage(data.READ, "state", "key", nil)
	held := data.Read[core.Primitive](op.Next(read.Next(nil)))
	if held != payload {
		t.Fatal("READ did not return the stored primitive")
	}
}

func TestKVNextEvaluateRequiresFactory(t *testing.T) {
	op := NewKV()
	msg := data.NewMessage(data.EVALUATE, "state", "new", data.NewValue(1.0))
	for range op.Next(msg.Next(nil)) {
		t.Fatal("missing state yielded data")
	}
	if !errors.Is(op.Error(), core.ErrNotHeld) {
		t.Fatalf("error = %v", op.Error())
	}
}

func TestKVNextEvaluatePropagatesPipelineErrors(t *testing.T) {
	op := NewKV(func() core.Primitive { return nomagique.NewNumber(statistic.NewSum()) })
	bad := &data.Value[float64]{PrimitiveError: core.NewPrimitiveError(), Values: []unsafe.Pointer{nil}}
	msg := data.NewMessage(data.EVALUATE, "state", "new", bad)
	for range op.Next(msg.Next(nil)) {
		t.Fatal("invalid observation produced output")
	}
	if !errors.Is(op.Error(), core.ErrShape) {
		t.Fatalf("nested error disappeared: %v", op.Error())
	}
}

func TestConstantNextRetainsObservedOrigin(t *testing.T) {
	op := NewConstant[int64]()
	for _, at := range []int64{100, 150, 220} {
		got := data.Read[int64](op.Next(data.NewValue(at).Next(nil)))
		if got != 100 {
			t.Fatalf("origin = %d at %d", got, at)
		}
	}
}

func TestConstantNextConfiguredConstantUnchanged(t *testing.T) {
	op := NewConstant(7.0)
	for _, value := range []float64{1, 2, 3} {
		got := data.Read[float64](op.Next(data.NewValue(value).Next(nil)))
		if got != 7 {
			t.Fatalf("constant = %g", got)
		}
	}
}

func TestKVNextNoopIsStillIgnored(t *testing.T) {
	op := NewKV()
	for range op.Next(data.NewMessage(data.NOOP, "state", "key", nil).Next(nil)) {
		t.Fatal("NOOP emitted a value")
	}
	if err := op.Error(); err != nil {
		t.Fatalf("NOOP changed existing behavior: %v", err)
	}
}

func TestKVNextNilFactoryResultIsAnError(t *testing.T) {
	op := NewKV(func() core.Primitive { return nil })
	msg := data.NewMessage(data.EVALUATE, "state", "key", data.NewValue(1.0))
	for range op.Next(msg.Next(nil)) {
		t.Fatal("nil factory result emitted a value")
	}
	if !errors.Is(op.Error(), core.ErrNotHeld) {
		t.Fatalf("nil factory result: %v", op.Error())
	}
}
