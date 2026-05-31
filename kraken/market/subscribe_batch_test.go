package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
)

func TestSymbolBatches(t *testing.T) {
	Convey("Given symbols larger than SubscribeBatch", t, func() {
		prev := config.System.SubscribeBatch
		config.System.SubscribeBatch = 2
		defer func() { config.System.SubscribeBatch = prev }()

		batches := symbolBatches([]string{"A", "B", "C", "D", "E"})

		Convey("It should split into fixed-size batches", func() {
			So(batches, ShouldResemble, [][]string{
				{"A", "B"},
				{"C", "D"},
				{"E"},
			})
		})
	})
}

func TestLimitSymbols(t *testing.T) {
	Convey("Given a discovered universe and MaxScanSymbols", t, func() {
		universe := []string{"A", "B", "C", "D"}

		Convey("It should cap when max is positive and smaller", func() {
			So(LimitSymbols(universe, 2), ShouldResemble, []string{"A", "B"})
		})

		Convey("It should return the full list when max is zero", func() {
			So(LimitSymbols(universe, 0), ShouldResemble, universe)
		})
	})
}
