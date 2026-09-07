//go:build !darwin || !cgo

package sensorium

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestNewEngine(t *testing.T) {
	Convey("An unsupported platform rejects initialization explicitly", t, func() {
		engine, err := NewEngine("unused.metallib", 8, 8, 8, 0.125)
		So(engine, ShouldBeNil)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "requires Darwin with CGo")
	})
}
