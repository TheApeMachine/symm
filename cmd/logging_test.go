package cmd

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
)

func TestLogWriterTargetFor(t *testing.T) {
	Convey("Given stdout logging is enabled", t, func() {
		cfg := config.NewConfig()
		cfg.LogStdoutActive = true
		cfg.LogFileActive = false

		target := logWriterTargetFor(cfg, "")

		Convey("It should leave logging on stdout", func() {
			So(target, ShouldEqual, logWriterTargetStdout)
		})
	})

	Convey("Given file logging is enabled without stdout", t, func() {
		cfg := config.NewConfig()
		cfg.LogStdoutActive = false
		cfg.LogFileActive = true

		target := logWriterTargetFor(cfg, "runs/symm.log")

		Convey("It should send logging to the file writer", func() {
			So(target, ShouldEqual, logWriterTargetFile)
		})
	})

	Convey("Given eval disables stdout and file logging", t, func() {
		cfg := config.NewConfig()
		cfg.LogStdoutActive = false
		cfg.LogFileActive = false

		target := logWriterTargetFor(cfg, "")

		Convey("It should discard logs instead of contaminating stdout", func() {
			So(target, ShouldEqual, logWriterTargetDiscard)
		})
	})
}
