package config_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
)

func TestTunablesApplySaveLoad(t *testing.T) {
	convey.Convey("Given tunable overrides", t, func() {
		cfg := config.NewConfig()
		edge := 2.5
		ttl := 45 * time.Second
		overlay := config.Tunables{
			EntryEdgeMultiple: &edge,
			PerspectiveTTL:    &ttl,
		}

		overlay.Apply(cfg)

		convey.Convey("Apply should mutate cfg", func() {
			convey.So(cfg.EntryEdgeMultiple, convey.ShouldEqual, 2.5)
			convey.So(cfg.PerspectiveTTL, convey.ShouldEqual, 45*time.Second)
		})

		path := filepath.Join(t.TempDir(), "tuned.json")

		convey.Convey("Save and load should round-trip", func() {
			convey.So(config.SaveTunablesFile(path, cfg), convey.ShouldBeNil)

			loaded := config.NewConfig()
			convey.So(config.LoadTunablesFile(path, loaded), convey.ShouldBeNil)
			convey.So(loaded.EntryEdgeMultiple, convey.ShouldEqual, 2.5)
			convey.So(loaded.PerspectiveTTL, convey.ShouldEqual, 45*time.Second)
		})
	})
}

func TestMutateTunables(t *testing.T) {
	convey.Convey("Given specs", t, func() {
		overlay := config.MutateTunables(config.NewConfig(), nil)

		convey.Convey("It should populate bounded fields", func() {
			convey.So(overlay.EntryEdgeMultiple, convey.ShouldNotBeNil)
			convey.So(*overlay.EntryEdgeMultiple, convey.ShouldBeGreaterThanOrEqualTo, 1.0)
			convey.So(*overlay.EntryEdgeMultiple, convey.ShouldBeLessThanOrEqualTo, 4.0)
		})
	})
}

func TestDefaultTunedPath(t *testing.T) {
	convey.Convey("DefaultTunedPath", t, func() {
		convey.So(config.DefaultTunedPath(), convey.ShouldEqual, "runs/tuned.json")
	})
}

func TestDefaultTunedInstallPath(t *testing.T) {
	convey.Convey("DefaultTunedInstallPath", t, func() {
		convey.So(config.DefaultTunedInstallPath(), convey.ShouldEqual, "config/tuned.json")
	})
}

func TestInitLoadsTunedFileWhenPresent(t *testing.T) {
	convey.Convey("Given runs/tuned.json exists", t, func() {
		path := filepath.Join(t.TempDir(), "tuned.json")
		edge := 3.25
		overlay := config.Tunables{EntryEdgeMultiple: &edge}
		cfg := config.NewConfig()
		overlay.Apply(cfg)
		convey.So(config.SaveTunablesFile(path, cfg), convey.ShouldBeNil)

		loaded := config.NewConfig()
		convey.So(config.LoadTunablesFile(path, loaded), convey.ShouldBeNil)
		convey.So(loaded.EntryEdgeMultiple, convey.ShouldEqual, 3.25)
	})
}
