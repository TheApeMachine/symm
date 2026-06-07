package execution

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestAffordableSlots(t *testing.T) {
	Convey("Given a €200 base and 20% deploy", t, func() {
		slots := AffordableSlots(200, 0.2, 0.0026, 200)

		Convey("It should fit five concurrent entries", func() {
			So(slots, ShouldEqual, 5)
		})
	})

	Convey("Given max concurrent above cash slots", t, func() {
		original := viper.Get("trading.max_concurrent_positions")
		viper.Set("trading.max_concurrent_positions", 16)
		defer viper.Set("trading.max_concurrent_positions", original)

		Convey("EntrySlotAvailable should respect both limits", func() {
			So(EntrySlotAvailable(4, 0.2, 200, 40, 0.0026), ShouldBeTrue)
			So(EntrySlotAvailable(5, 0.2, 200, 40, 0.0026), ShouldBeFalse)
			So(EntrySlotAvailable(4, 0.2, 200, 0, 0.0026), ShouldBeFalse)
			So(EntrySlotAvailable(16, 0.05, 200, 10_000, 0.0026), ShouldBeFalse)
		})
	})
}

func BenchmarkAffordableSlots(b *testing.B) {
	for b.Loop() {
		_ = AffordableSlots(200, 0.2, 0.0026, 200)
	}
}
