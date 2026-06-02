package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestLayoutDocument(t *testing.T) {
	Convey("Given market config", t, func() {
		viper.Set("market.default_symbols", []string{"ETH/EUR"})

		doc := LayoutDocument()

		Convey("It should emit a layout payload for the dashboard", func() {
			So(doc["event"], ShouldEqual, "layout")
			So(doc["anchor_symbol"], ShouldEqual, "ETH/EUR")
			So(doc["ts"], ShouldNotBeBlank)

			panels, ok := doc["panels"].([]map[string]any)

			So(ok, ShouldBeTrue)
			So(len(panels), ShouldEqual, 6)
			So(panels[1]["type"], ShouldEqual, "gauge_grid")

			sources, ok := panels[1]["sources"].([]string)

			So(ok, ShouldBeTrue)
			So(len(sources), ShouldEqual, 8)
		})
	})
}

func BenchmarkLayoutDocument(b *testing.B) {
	viper.Set("market.default_symbols", []string{"BTC/EUR"})

	for b.Loop() {
		_ = LayoutDocument()
	}
}
