package tests

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	tes "github.com/theapemachine/symm/tests/types"
)

func TestMarketWithMarket(t *testing.T) {
	Convey("Given the default market wrapper", t, WithMarket(t, []*tes.Symbol{
		tes.NewSymbol("SIM1/USD", 100, 42),
	}, func(market *Market) {
		So(market != nil, ShouldBeTrue)
		market.Tick()
	}))
}

func TestMarketWithAutoFill(t *testing.T) {
	symbols := []*tes.Symbol{
		tes.NewSymbol("SIM1/USD", 105, 42),
	}

	Convey("Given two conflicting execution configurations", t, func() {
		market := NewMarket(t.Context(), symbols)
		defer market.Close()
		config := tes.DefaultExecutionConfig()

		Convey("WithAutoFill should reject the ambiguous call", func() {
			So(func() {
				market.WithAutoFill(config, config)
			}, ShouldPanic)
		})
	})

	runAutoFillStackTest(t, symbols)
}
