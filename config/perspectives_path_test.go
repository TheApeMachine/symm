package config

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDefaultPerspectivePath(t *testing.T) {
	Convey("DefaultPerspectivePath", t, func() {
		So(DefaultPerspectivePath(), ShouldEqual, "runs/perspectives.yaml")
	})
}

func TestPerspectiveLoadPathFor(t *testing.T) {
	Convey("Given only the builtin perspectives file exists", t, func() {
		dir := t.TempDir()
		builtin := filepath.Join(dir, defaultPerspectiveBuiltinFile)
		So(os.MkdirAll(filepath.Dir(builtin), 0o755), ShouldBeNil)
		So(os.WriteFile(builtin, []byte("version: 1\nplaybooks: []\n"), 0o644), ShouldBeNil)
		previous, err := os.Getwd()
		So(err, ShouldBeNil)
		So(os.Chdir(dir), ShouldBeNil)
		defer func() {
			_ = os.Chdir(previous)
		}()

		Convey("It should fall back from the default run path to the builtin file", func() {
			So(PerspectiveLoadPathFor(DefaultPerspectivePath()), ShouldEqual, defaultPerspectiveBuiltinFile)
		})
	})

	Convey("Given an explicit override path that is missing", t, func() {
		Convey("It should not fall back to the builtin file", func() {
			So(PerspectiveLoadPathFor("runs/custom.yaml"), ShouldEqual, "runs/custom.yaml")
		})
	})
}
