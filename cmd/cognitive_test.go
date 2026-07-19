package cmd

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestRotateCognitive(t *testing.T) {
	previous := viper.Get("cognitive.reset_on_boot")
	t.Cleanup(func() { viper.Set("cognitive.reset_on_boot", previous) })

	Convey("Given an existing cognitive persist directory", t, func() {
		persistDir := filepath.Join(t.TempDir(), "cognitive")
		So(os.MkdirAll(persistDir, 0o700), ShouldBeNil)
		So(os.WriteFile(filepath.Join(persistDir, "wal.log"), []byte("x"), 0o644), ShouldBeNil)
		viper.Set("cognitive.reset_on_boot", true)

		Convey("When rotateCognitive runs", func() {
			So(rotateCognitive(persistDir), ShouldBeNil)

			Convey("Then the original path is free and a stamped sibling exists", func() {
				_, err := os.Stat(persistDir)
				So(os.IsNotExist(err), ShouldBeTrue)
				parent := filepath.Dir(persistDir)
				entries, err := os.ReadDir(parent)
				So(err, ShouldBeNil)
				So(len(entries), ShouldBeGreaterThan, 0)
			})
		})
	})

	Convey("Given reset_on_boot disabled", t, func() {
		persistDir := filepath.Join(t.TempDir(), "cognitive")
		So(os.MkdirAll(persistDir, 0o700), ShouldBeNil)
		viper.Set("cognitive.reset_on_boot", false)
		So(rotateCognitive(persistDir), ShouldBeNil)
		_, err := os.Stat(persistDir)
		So(err, ShouldBeNil)
	})

	Convey("Given in-memory cognition", t, func() {
		previousMemory := viper.Get("cognitive.in_memory")
		t.Cleanup(func() { viper.Set("cognitive.in_memory", previousMemory) })
		persistDir := filepath.Join(t.TempDir(), "cognitive")
		So(os.MkdirAll(persistDir, 0o700), ShouldBeNil)
		viper.Set("cognitive.in_memory", true)
		viper.Set("cognitive.reset_on_boot", true)
		So(rotateCognitive(persistDir), ShouldBeNil)
		_, err := os.Stat(persistDir)
		So(err, ShouldBeNil)
	})
}
