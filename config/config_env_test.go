package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
)

func TestApplyEnvironmentRejectsInvalidDuration(t *testing.T) {
	Convey("Given an invalid replay pace", t, func() {
		t.Setenv("SYMM_REPLAY_PACE", "not-a-duration")
		cfg := &config.Config{}

		Convey("It should fail closed", func() {
			err := config.ApplyEnvironment(cfg)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "SYMM_REPLAY_PACE")
		})
	})
}

func TestApplyEnvironmentParsesValidDuration(t *testing.T) {
	Convey("Given a valid replay pace", t, func() {
		t.Setenv("SYMM_REPLAY_PACE", "250ms")
		cfg := &config.Config{}

		Convey("It should apply the override", func() {
			So(config.ApplyEnvironment(cfg), ShouldBeNil)
			So(cfg.ReplayPace, ShouldEqual, 250*time.Millisecond)
		})
	})
}

func TestLoadTunablesFileRejectsInvalidJSON(t *testing.T) {
	Convey("Given an invalid tunables file", t, func() {
		path := filepath.Join(t.TempDir(), "bad.json")
		So(os.WriteFile(path, []byte("{"), 0o644), ShouldBeNil)

		Convey("LoadTunablesFile should return an error", func() {
			err := config.LoadTunablesFile(path, config.NewConfig())

			So(err, ShouldNotBeNil)
		})
	})
}
