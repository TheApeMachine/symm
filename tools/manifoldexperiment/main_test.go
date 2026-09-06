package main

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestExperimentCommandRun(t *testing.T) {
	Convey("Experiment parameters must be explicit before opening a field", t, func() {
		command := &ExperimentCommand{}
		So(command.Run(), ShouldNotBeNil)
	})
}
