package logic

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
)

func TestEntrySlotAdmission(t *testing.T) {
	Convey("Given a four-base two-opportunity envelope", t, func() {
		tradingConfig := config.TradingConfig{
			MaxConcurrentPositions: 4,
			OpportunitySlotCount:   2,
		}

		occupancy := EntrySlotOccupancyFromHoldings(NewHoldings())

		Convey("It should allow a base slot while capacity remains", func() {
			allowed, opportunity := EntrySlotAdmission(occupancy, tradingConfig, false)

			So(allowed, ShouldBeTrue)
			So(opportunity, ShouldBeFalse)
		})

		Convey("It should reject a regular entry once base slots are full", func() {
			holdings := NewHoldings()

			for index := range 4 {
				holdings.SetPosition(
					fmt.Sprintf("SYM%d/USD", index),
					1,
					0.7,
					false,
				)
			}

			allowed, opportunity := EntrySlotAdmission(
				EntrySlotOccupancyFromHoldings(holdings),
				tradingConfig,
				false,
			)

			So(allowed, ShouldBeFalse)
			So(opportunity, ShouldBeFalse)
		})

		Convey("It should allow a qualified opportunity entry after base slots fill", func() {
			holdings := NewHoldings()

			for index := range 4 {
				holdings.SetPosition(
					fmt.Sprintf("SYM%d/USD", index),
					1,
					0.7,
					false,
				)
			}

			allowed, opportunity := EntrySlotAdmission(
				EntrySlotOccupancyFromHoldings(holdings),
				tradingConfig,
				true,
			)

			So(allowed, ShouldBeTrue)
			So(opportunity, ShouldBeTrue)
		})

		Convey("It should reject any entry once all six slots are occupied", func() {
			holdings := NewHoldings()
			holdings.SetPosition("BASE0/USD", 1, 0.7, false)
			holdings.SetPosition("BASE1/USD", 1, 0.7, false)
			holdings.SetPosition("BASE2/USD", 1, 0.7, false)
			holdings.SetPosition("BASE3/USD", 1, 0.7, false)

			holdings.SetPosition("OPP1/USD", 1, 0.95, true)
			holdings.SetPosition("OPP2/USD", 1, 0.96, true)

			allowed, opportunity := EntrySlotAdmission(
				EntrySlotOccupancyFromHoldings(holdings),
				tradingConfig,
				true,
			)

			So(allowed, ShouldBeFalse)
			So(opportunity, ShouldBeFalse)
		})

		Convey("It should count in-flight base entries against capacity", func() {
			occupancy := EntrySlotOccupancy{
				BasePending: 4,
			}

			allowed, opportunity := EntrySlotAdmission(occupancy, tradingConfig, false)

			So(allowed, ShouldBeFalse)
			So(opportunity, ShouldBeFalse)
		})

		Convey("It should reject a seventh entry when six are already committed", func() {
			occupancy := EntrySlotOccupancy{
				BaseHeld:        4,
				OpportunityHeld: 1,
				BasePending:     1,
			}

			allowed, opportunity := EntrySlotAdmission(occupancy, tradingConfig, true)

			So(allowed, ShouldBeFalse)
			So(opportunity, ShouldBeFalse)
		})
	})
}

func TestOpportunitySlotCounts(t *testing.T) {
	Convey("Given mixed base and opportunity holdings", t, func() {
		holdings := NewHoldings()
		holdings.SetPosition("AAA/USD", 1, 0.8, false)
		holdings.SetPosition("BBB/USD", 1, 0.9, true)

		Convey("It should count each slot class separately", func() {
			So(holdings.BaseSlotCount(), ShouldEqual, 1)
			So(holdings.OpportunitySlotCount(), ShouldEqual, 1)
			So(holdings.OpenCount(), ShouldEqual, 2)
		})
	})
}

func BenchmarkEntrySlotAdmission(b *testing.B) {
	tradingConfig := config.TradingConfig{
		MaxConcurrentPositions: 4,
		OpportunitySlotCount:   2,
	}
	occupancy := EntrySlotOccupancy{
		BaseHeld:        3,
		OpportunityHeld: 1,
		BasePending:     1,
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = EntrySlotAdmission(occupancy, tradingConfig, true)
	}
}
