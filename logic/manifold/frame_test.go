package manifold

import (
	"math"
	"testing"
)

func TestPrimitiveResidentFrame(t *testing.T) {
	frame := newFrames(3)
	// Warm the frame with varied prices exceeding core.MinimumPrior so
	// that PriorScale can compute a valid prior dispersion.
	for _, price := range []float64{100, 102, 98, 101, 99, 101.5, 98.5, 100.5, 99.5, 100} {
		if _, _, _, _, err := frame.place("A", math.Log(price), 0); err != nil {
			t.Fatal(err)
		}
	}

	var previous float64 = 1.0
	for _, price := range []float64{99, 98} {
		x, y, _, _, err := frame.place("A", math.Log(price), 0)
		if err != nil {
			t.Fatal(err)
		}
		if x < 0 || x > 1 || y != 0.5 {
			t.Fatalf("invalid frame coordinate %v %v", x, y)
		}
		if x >= previous {
			t.Fatalf("price order lost: %v >= %v", x, previous)
		}
		previous = x
	}
	before := frame.symbols["A"].lastPrice[0]
	for range 5 {
		if _, _, err := frame.placePrice("A", math.Log(101)); err != nil {
			t.Fatal(err)
		}
	}
	after := frame.symbols["A"].lastPrice[0]
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
