package transport

import (
	"errors"
	"testing"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestAddressableNextPropagatesStoreError(t *testing.T) {
	root := nomagique.NewNumber(NewAddressable("symbolstore", store.NewKV(func() core.Primitive {
		return nomagique.NewNumber(statistic.NewSum())
	})))
	bad := &data.Value[float64]{PrimitiveError: core.NewPrimitiveError(), Values: []unsafe.Pointer{nil}}
	msg := data.NewMessage(data.EVALUATE, "symbolstore", "BTC/USD:1", bad)
	for range root.Next(msg.Next(nil)) {
		t.Fatal("invalid input yielded a value")
	}
	if !errors.Is(root.Error(), core.ErrShape) {
		t.Fatalf("root lost primitive error: %v", root.Error())
	}
}
