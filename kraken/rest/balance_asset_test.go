package rest

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAssetBalanceFindsPrefixedCode(t *testing.T) {
	balance := &Balance{
		Result: map[string]string{
			"XXBT": "0.125",
		},
	}

	amount, ok := balance.AssetBalance("XXBT")

	if !ok {
		t.Fatal("expected balance lookup")
	}

	if amount != 0.125 {
		t.Fatalf("expected 0.125, got %v", amount)
	}
}

func TestAssetBalanceCandidates(t *testing.T) {
	Convey("Given Kraken balance key variants", t, func() {
		balance := &Balance{Result: map[string]string{"XETH": "1.5"}}

		amount, ok := balance.AssetBalance("eth")

		Convey("It should resolve prefixed asset codes", func() {
			So(ok, ShouldBeTrue)
			So(amount, ShouldAlmostEqual, 1.5, 1e-12)
		})
	})

	Convey("Given invalid balance input", t, func() {
		Convey("It should reject nil and empty lookups", func() {
			var nilBalance *Balance
			_, ok := nilBalance.AssetBalance("BTC")
			So(ok, ShouldBeFalse)
			_, ok = (&Balance{}).AssetBalance("")
			So(ok, ShouldBeFalse)
		})
	})
}

func BenchmarkAssetBalance(b *testing.B) {
	balance := &Balance{Result: map[string]string{"XXBT": "0.125"}}

	for b.Loop() {
		_, _ = balance.AssetBalance("BTC")
	}
}

