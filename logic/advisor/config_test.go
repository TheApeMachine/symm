package advisor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAdvisorConfig(t *testing.T) {
	Convey("Given the default advisor configuration", t, func() {
		defaultConfig := DefaultConfig()
		So(defaultConfig, ShouldNotBeNil)
		So(len(defaultConfig.Advisors), ShouldEqual, 7)

		Convey("It serializes to and deserializes from JSON losslessly", func() {
			encoded, err := json.MarshalIndent(defaultConfig, "", "  ")
			So(err, ShouldBeNil)
			So(len(encoded), ShouldBeGreaterThan, 0)

			var decoded AdvisorsConfig
			err = json.Unmarshal(encoded, &decoded)
			So(err, ShouldBeNil)
			So(len(decoded.Advisors), ShouldEqual, 7)

			momentumCfg, found := decoded.Advisors[MomentumName]
			So(found, ShouldBeTrue)
			So(momentumCfg.Clock, ShouldEqual, momentumClock)
			So(len(momentumCfg.Features), ShouldEqual, 4)

			features := momentumCfg.ToFeatures()
			So(len(features), ShouldEqual, 4)
			So(features[0].Class.Label, ShouldEqual, "Building")
			So(len(features[0].Keys), ShouldBeGreaterThan, 0)
			So(len(features[0].Class.Predictions), ShouldEqual, 1)
			So(features[0].Class.Predictions[0].Support.Move, ShouldEqual, INCREASE)
			So(features[0].Class.Predictions[0].Contradict.Move, ShouldEqual, DECREASE)
		})

		Convey("FeaturesForAdvisor returns configured features when present", func() {
			features := FeaturesForAdvisor(MomentumName, defaultConfig)
			So(len(features), ShouldEqual, 4)
			So(features[0].Class.Label, ShouldEqual, "Building")
		})

		Convey("Enabled preserves existing configs and honors an explicit roster", func() {
			So(defaultConfig.Enabled(MomentumName), ShouldBeTrue)

			disabled := false
			momentum := defaultConfig.Advisors[MomentumName]
			momentum.Enabled = &disabled
			defaultConfig.Advisors[MomentumName] = momentum

			So(defaultConfig.Enabled(MomentumName), ShouldBeFalse)
			So(defaultConfig.Enabled(AuctionName), ShouldBeTrue)
		})

		Convey("FeaturesForAdvisor falls back to compiled defaults when config is nil or missing", func() {
			features := FeaturesForAdvisor(MomentumName, nil)
			So(len(features), ShouldEqual, 4)
			So(features[0].Class.Label, ShouldEqual, "Building")

			emptyConfig := &AdvisorsConfig{Advisors: make(map[string]AdvisorConfig)}
			fallbackFeatures := FeaturesForAdvisor(AuctionName, emptyConfig)
			So(len(fallbackFeatures), ShouldEqual, 5)
		})

		Convey("LoadConfig reads from a JSON file on disk", func() {
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "advisors.json")

			encoded, err := json.MarshalIndent(defaultConfig, "", "  ")
			So(err, ShouldBeNil)

			err = os.WriteFile(configPath, encoded, 0644)
			So(err, ShouldBeNil)

			loaded, err := LoadConfig(configPath)
			So(err, ShouldBeNil)
			So(loaded, ShouldNotBeNil)
			So(len(loaded.Advisors), ShouldEqual, 7)

			profitRunCfg := loaded.Advisors[ProfitRunName]
			So(len(profitRunCfg.Features), ShouldEqual, 4)
		})

		Convey("Optimized configuration from Python script parses and converts to features properly", func() {
			testConfigPath := filepath.Join("..", "..", "config", "test_advisors.json")

			if _, statErr := os.Stat(testConfigPath); statErr == nil {
				loaded, err := LoadConfig(testConfigPath)
				So(err, ShouldBeNil)
				So(loaded, ShouldNotBeNil)

				momentumFeatures := FeaturesForAdvisor(MomentumName, loaded)
				So(len(momentumFeatures), ShouldEqual, 4)
				So(momentumFeatures[0].Class.Label, ShouldEqual, "Building")
				So(len(momentumFeatures[0].Keys), ShouldBeGreaterThanOrEqualTo, 3)
				So(len(momentumFeatures[0].Class.Predictions), ShouldBeGreaterThanOrEqualTo, 1)
			}
		})
	})
}
