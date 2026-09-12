package grid

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* tokenOf drives the token primitive for one condition. */
func tokenOf(t testing.TB, quantity uint64, level, change float64) uint64 {
	t.Helper()
	token := NewToken()
	command := TokenCommand{Condition: &TokenCondition{Quantity: quantity, Level: level, Change: change}}

	for out := range token.Next(transport.NewOne(unsafe.Pointer(&command)).Next(nil)) {
		return (*TokenResult)(out).Token
	}

	t.Fatal("token primitive held no value")

	return 0
}

func TestConditionToken(t *testing.T) {
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
}

func TestRemapCondition(t *testing.T) {
	Convey("Warmup preserves conditions while quantity registration order changes", t, func() {
		token := NewToken()
		remap := func(tokenVal, quantity uint64) uint64 {
			command := TokenCommand{Remap: &TokenRemap{Token: tokenVal, Quantity: quantity}}

			for out := range token.Next(transport.NewOne(unsafe.Pointer(&command)).Next(nil)) {
				return (*TokenResult)(out).Token
			}

			t.Fatal("token primitive held no value")

			return 0
		}
		So(remap(1, 9), ShouldEqual, 9)
		So(remap(tokenOf(t, 1, -1, 1), 9), ShouldEqual, tokenOf(t, 9, -1, 1))
	})
}

func BenchmarkConditionToken(b *testing.B) {
	for b.Loop() {
		if _, err := conditionToken(1, -1, 1); err != nil {
			b.Fatal(err)
		}
	}
}

func TestConditionQuantity(t *testing.T) {
	Convey("Conditioned quantities preserve their identity in every directional regime", t, func() {
		for _, quantity := range []uint64{1, 793, (1 << 48) - 1} {
			So(conditionQuantity(quantity), ShouldEqual, quantity)

			for _, level := range []float64{-1, 0, 1} {
				for _, change := range []float64{-1, 0, 1} {
					token, err := conditionToken(quantity, level, change)
					So(err, ShouldBeNil)
					So(conditionQuantity(token), ShouldEqual, quantity)
				}
			}
		}
	})
}

func BenchmarkConditionQuantity(b *testing.B) {
	token, err := conditionToken(793, -1, 1)

	if err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		if conditionQuantity(token) != 793 {
			b.Fatal("quantity identity changed")
		}
	}
}
