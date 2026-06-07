package cmd

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestLoadDefaultConfigs(t *testing.T) {
	Convey("Given split infra and strategy files", t, func() {
		err := loadDefaultConfigs()

		Convey("It should load trading strategy keys", func() {
			So(err, ShouldBeNil)
			So(viper.GetFloat64("trading.position_fraction"), ShouldBeGreaterThan, 0)
			So(viper.GetString("market.quote_currency"), ShouldEqual, "EUR")
		})
	})
}

func TestMergeConfigFilesRejectsMissingPath(t *testing.T) {
	Convey("Given a missing config path", t, func() {
		err := mergeConfigFiles(filepath.Join(t.TempDir(), "missing.yml"))

		Convey("It should return an error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestLoadEmbeddedConfigs(t *testing.T) {
	Convey("Given embedded split configs", t, func() {
		originalDir, wdErr := os.Getwd()

		So(wdErr, ShouldBeNil)

		repoRoot := filepath.Clean(filepath.Join(originalDir, ".."))
		chdirErr := os.Chdir(repoRoot)

		So(chdirErr, ShouldBeNil)

		defer func() {
			_ = os.Chdir(originalDir)
		}()

		err := loadEmbeddedConfigs()

		Convey("It should merge infra and strategy", func() {
			So(err, ShouldBeNil)
			So(viper.IsSet("trading.max_concurrent_positions"), ShouldBeTrue)
			So(viper.IsSet("market.ws_ping_interval"), ShouldBeTrue)
		})
	})
}
