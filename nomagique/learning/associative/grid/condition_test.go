package grid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/* tokenOf drives the Condition primitive for one input tuple via Adapter. */
func tokenOf(t testing.TB, quantity uint64, level, change float64) uint64 {
	t.Helper()
	op := NewCondition()
	state := data.NewState(
		data.NewMap("quantity", "quantity", "level", "level", "change", "change"),
	)
	adapter := data.NewAdapter(nil, state)
	inputValues := data.NewOutputMap()
	inputValues.Values["quantity"] = float64(quantity)
	inputValues.Values["level"] = level
	inputValues.Values["change"] = change

	for range adapter.Next(data.NewValue(inputValues)) {
	}

	data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))

	if op.Error() != nil {
		t.Fatal(op.Error())
	}

	return uint64(op.output.Values["token"])
}

func TestCondition(t *testing.T) {
	Convey("The same hot quantity distinguishes buildup, stagnation and reversal", t, func() {
		seen := map[uint64]bool{}

		for _, level := range []float64{-1, 0, 1} {
			for _, change := range []float64{-1, 0, 1} {
				token := tokenOf(t, 1, level, change)
				So(seen[token], ShouldBeFalse)
				seen[token] = true
				So(token, ShouldNotEqual, tokenOf(t, 2, level, change))
				So(token, ShouldEqual, tokenOf(t, 1, level*10, change*10))
			}
		}
	})

	Convey("Invalid quantity returns domain error", t, func() {
		op := NewCondition()
		state := data.NewState(
			data.NewMap("quantity", "quantity", "level", "level", "change", "change"),
		)
		adapter := data.NewAdapter(nil, state)
		inputValues := data.NewOutputMap()
		inputValues.Values["quantity"] = 0
		inputValues.Values["level"] = 1
		inputValues.Values["change"] = 1

		for range adapter.Next(data.NewValue(inputValues)) {
		}

		data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))
		So(op.Error(), ShouldNotBeNil)
	})
}

func BenchmarkCondition(b *testing.B) {
	op := NewCondition()
	state := data.NewState(
		data.NewMap("quantity", "quantity", "level", "level", "change", "change"),
	)
	adapter := data.NewAdapter(nil, state)
	inputValues := data.NewOutputMap()
	inputValues.Values["quantity"] = 1
	inputValues.Values["level"] = -1
	inputValues.Values["change"] = 1

	for range adapter.Next(data.NewValue(inputValues)) {
	}

	for b.Loop() {
		data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))

		if op.output.Values["token"] == 0 {
			b.Fatal("condition produced zero token")
		}
	}
}
