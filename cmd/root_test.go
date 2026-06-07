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
	Convey("Given embedded split configs", t, func() {
		infra, infraErr := embedded.ReadFile("cfg/infra.yml")
		strategy, strategyErr := embedded.ReadFile("cfg/strategy.yml")

		Convey("It should ship infra.yml and strategy.yml in the binary", func() {
			So(infraErr, ShouldBeNil)
			So(strategyErr, ShouldBeNil)
			So(string(infra), ShouldContainSubstring, "quote_currency")
			So(string(strategy), ShouldContainSubstring, "position_fraction")
		})
	})
}
