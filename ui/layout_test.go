package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestLayoutDocument(t *testing.T) {
	t.Cleanup(viper.Reset)
	perspectives.ResetTelemetryRegistryForTest()
	t.Cleanup(perspectives.ResetTelemetryRegistryForTest)

	Convey("Given market config", t, func() {
		viper.Set("market.anchor_symbol", "")
		viper.Set("market.default_symbols", []string{"ETH/EUR"})
		perspectives.BootstrapTelemetryManifest()

		doc := LayoutDocument()

		Convey("It should emit a layout payload for the dashboard", func() {
			So(doc["event"], ShouldEqual, "layout")
			So(doc["anchor_symbol"], ShouldEqual, "ETH/EUR")
			So(doc["ts"], ShouldNotBeBlank)

			panels, ok := doc["panels"].([]map[string]any)

			So(ok, ShouldBeTrue)
			So(len(panels), ShouldEqual, 7)
			So(panels[1]["type"], ShouldEqual, "gauge_grid")
			So(panels[2]["type"], ShouldEqual, "gauge_strip")

			gridSources, ok := panels[1]["sources"].([]string)

			So(ok, ShouldBeTrue)
			So(len(gridSources), ShouldEqual, 8)

			stripSources, ok := panels[2]["sources"].([]string)

			So(ok, ShouldBeTrue)
			So(len(stripSources), ShouldEqual, 5)
		})
	})
}

func BenchmarkLayoutDocument(b *testing.B) {
	viper.Set("market.anchor_symbol", "")
	viper.Set("market.default_symbols", []string{"BTC/EUR"})
	b.Cleanup(viper.Reset)

	for b.Loop() {
		_ = LayoutDocument()
	}
}
