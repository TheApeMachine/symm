package manifold

import (
	"math"
	"testing"
)

func TestPrimitiveResidentFrame(t *testing.T) {
	frame := newFrames(3)
	var previous float64
	for index, price := range []float64{100, 99, 98} {
		x, y, z, _, err := frame.place("A", math.Log(price), 0)
		if err != nil {
			t.Fatal(err)
		}
		if math.IsNaN(x) || x < 0 || x > 1 || y != .5 {
			t.Fatalf("invalid frame coordinate %v %v", x, y)
		}
		if index == 2 && x >= previous {
			t.Fatalf("price order lost: %v >= %v", x, previous)
		}
		if math.IsNaN(z) {
			t.Fatal("nonfinite deviation")
		}
		previous = x
	}
	before := frame.symbols["A"].lastPrice.Count
	for range 5 {
		if _, _, err := frame.placePrice("A", math.Log(101)); err != nil {
			t.Fatal(err)
		}
	}
	after := frame.symbols["A"].lastPrice.Count
	if before != after {
		t.Fatal("probe trained observed frame")
	}
	if _, _, err := frame.placePrice("B", 1); err == nil {
		t.Fatal("unobserved symbol fabricated a frame")
	}
	x, _, _, _, err := frame.place("B", math.Log(1000), 0)
	if err != nil || x != .5 {
		t.Fatalf("symbol isolation %v %v", x, err)
	}
}
func TestOrderIdentityNamespace(t *testing.T) {
	left := orderContentID(orderIdentity{"AB", "C"})
	right := orderContentID(orderIdentity{"A", "BC"})
	if left == right {
		t.Fatal("concatenated identities collide")
	}
	if left != orderContentID(orderIdentity{"AB", "C"}) || left < 1<<62 {
		t.Fatal("unstable order/probe namespace")
	}
}
