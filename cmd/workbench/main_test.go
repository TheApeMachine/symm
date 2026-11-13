package main

import (
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLoadConfig(t *testing.T) {
	Convey("Setup", t, func() {
		viper.Reset()
		root, err := filepath.Abs("../..")
		So(err, ShouldBeNil)

		Convey("When loadConfig is given an explicit config path", func() {
			path := filepath.Join(root, "cmd/cfg/config.yml")
			err := loadConfig(path)

			Convey("It loads learning.threshold_start", func() {
				So(err, ShouldBeNil)
				So(viper.GetFloat64("learning.threshold_start"), ShouldEqual, 0.5)
			})
		})

		Convey("When loadConfig is given a missing path", func() {
			path := filepath.Join(root, "no-such-config.yml")
			err := loadConfig(path)

			Convey("It returns the viper read error for that path", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, path)
			})
		})
	})
}
