package cmd

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
)

func TestRootCommand(t *testing.T) {
	testconfig.Load(t)

	Convey("Given the root command", t, func() {
		Convey("It should expose the symm use string", func() {
			So(rootCmd.Use, ShouldEqual, "symm")
		})

		Convey("It should register the tune subcommand", func() {
			found := false

			for _, command := range rootCmd.Commands() {
				if command.Use == "tune" {
					found = true
				}
			}

			So(found, ShouldBeTrue)
		})
	})
}

func TestEmbeddedConfig(t *testing.T) {
	Convey("Given embedded config", t, func() {
		configBytes, configErr := embedded.ReadFile("cfg/config.yml")

		Convey("It should ship config.yml in the binary", func() {
			So(configErr, ShouldBeNil)
			So(string(configBytes), ShouldContainSubstring, "quote_currency")
			So(string(configBytes), ShouldContainSubstring, "position_fraction")
		})
	})
}
